package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/handlers"
	"jabberwocky238/console/k8s"
	"jabberwocky238/console/k8s/controller"

	"github.com/gin-gonic/gin"
	"github.com/resend/resend-go/v3"
)

type App struct {
	closers []io.Closer
}

func (a *App) Register(c io.Closer) {
	a.closers = append(a.closers, c)
}

// closerFunc adapts a func() to io.Closer
type closerFunc func() error

func (f closerFunc) Close() error { return f() }

func (a *App) Shutdown() {
	log.Println("[shutdown] starting...")
	for i := len(a.closers) - 1; i >= 0; i-- {
		if err := a.closers[i].Close(); err != nil {
			log.Printf("[shutdown] error: %v", err)
		}
	}
	log.Println("[shutdown] done")
}

func main() {
	listen := flag.String("l", "localhost:9900", "External listen address")
	internalListen := flag.String("i", "localhost:9901", "Internal listen address")
	dbDSN := flag.String("d", "postgresql://myuser:your_password@localhost:5432/mydb?sslmode=disable", "Database DSN")
	kubeconfig := flag.String("k", "", "Kubeconfig path (empty for in-cluster)")
	flag.Parse()

	if os.Getenv("ENV") != "test" {
		checkEnv()
	}

	app := &App{}

	// 1. Database
	if err := dblayer.InitDB(*dbDSN); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	app.Register(closerFunc(func() error {
		log.Println("[shutdown] closing database")
		return dblayer.DB.Close()
	}))

	// 2. CockroachDB
	if err := k8s.InitRDBManager(); err != nil {
		panic("CockroachDB init failed: " + err.Error())
	}
	app.Register(k8s.RDBManager)

	// 3. K8s + Controller
	if err := k8s.InitK8s(*kubeconfig); err != nil {
		log.Printf("Warning: K8s client init failed: %v", err)
	} else {
		log.Println("K8s client initialized")
		controller.EnsureCRD(k8s.RestConfig)
		controller.EnsureCombinatorCRD(k8s.RestConfig)

		stopCh := make(chan struct{})
		app.Register(closerFunc(func() error {
			log.Println("[shutdown] stopping controller")
			close(stopCh)
			return nil
		}))
		ctrl := controller.NewController(k8s.DynamicClient, k8s.K8sClient)
		go ctrl.Start(stopCh)
	}

	// 4. Processor
	proc := k8s.NewProcessor(256, 4)
	proc.Start()
	app.Register(proc)

	wh := handlers.NewWorkerHandler(proc)
	ah := handlers.NewAuthHandler(proc)
	ch := handlers.NewCombinatorHandler(proc)
	cih := handlers.NewCombinatorInternalHandler(proc)

	// 5. Cron
	cron := k8s.NewCronScheduler(proc)
	cron.RegisterJob(24*time.Hour, &handlers.UserAuditJob{})
	cron.RegisterJob(12*time.Hour, &handlers.DomainCheckJob{})
	proc.Submit(&handlers.UserAuditJob{})
	cron.Start()
	app.Register(cron)

	log.Println("Control plane starting...")

	// Setup External Gin router (public access)
	externalRouter := gin.Default()
	if os.Getenv("ENV") == "test" {
		externalRouter.Use(crossOriginMiddleware())
	}

	// Serve frontend static files from dist/
	externalRouter.Static("/assets", "./dist/assets")
	externalRouter.GET("/", func(c *gin.Context) {
		c.File("./dist/index.html")
	})

	externalAPI := externalRouter.Group("/api")
	// Public routes
	externalAPI.POST("/auth/register", ah.Register)
	externalAPI.POST("/auth/login", handlers.Login)
	externalAPI.POST("/auth/send-code", handlers.SendCode)
	externalAPI.POST("/auth/reset-password", handlers.ResetPassword)

	// Protected routes (auth required)
	protected := externalAPI.Group("")
	protected.Use(handlers.AuthMiddleware())
	{
		protected.GET("/rdb", ch.ListRDBs)
		protected.GET("/rdb/:id", ch.GetRDB)
		protected.POST("/rdb", ch.CreateRDB)
		protected.DELETE("/rdb/:id", ch.DeleteRDB)

		protected.GET("/kv", ch.ListKVs)
		protected.POST("/kv", ch.CreateKV)
		protected.DELETE("/kv/:id", ch.DeleteKV)

		protected.GET("/worker", wh.ListWorkers)
		protected.GET("/worker/:id", wh.GetWorker)
		protected.POST("/worker", wh.CreateWorker)
		protected.DELETE("/worker/:id", wh.DeleteWorker)

		protected.GET("/worker/:id/env", wh.GetWorkerEnv)
		protected.POST("/worker/:id/env", wh.SetWorkerEnv)
		protected.GET("/worker/:id/secret", wh.GetWorkerSecrets)
		protected.POST("/worker/:id/secret", wh.SetWorkerSecrets)

		protected.GET("/domain", handlers.ListCustomDomains)
		protected.GET("/domain/:id", handlers.GetCustomDomain)
		protected.POST("/domain", handlers.AddCustomDomain)
		protected.DELETE("/domain/:id", handlers.DeleteCustomDomain)
	}

	// Sensitive routes (signature required)
	sensitive := externalAPI.Group("")
	sensitive.Use(handlers.SignatureMiddleware())
	{
		sensitive.POST("/worker/deploy", wh.DeployWorker)
	}

	// Fallback: serve static files from dist/ or index.html for SPA
	externalRouter.NoRoute(func(c *gin.Context) {
		filePath := path.Join("./dist", c.Request.URL.Path)
		if _, err := os.Stat(filePath); err == nil {
			c.File(filePath)
			return
		}
		c.File("./dist/index.html")
	})

	// Setup Internal Gin router (internal services access)
	internalRouter := gin.Default()
	internalRouter.GET("/health", handlers.Health)

	internalAPI := internalRouter.Group("/api")
	{
		// Internal routes (no auth required, only accessible from cluster)
		internalAPI.POST("/worker/deploy", wh.DeployWorker)
		internalAPI.GET("/combinator/retrieveSecretByID", cih.RetrieveSecretByID)
	}

	// 6. External HTTP Server
	externalSrv := &http.Server{Addr: *listen, Handler: externalRouter}
	app.Register(closerFunc(func() error {
		log.Println("[shutdown] stopping external http server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return externalSrv.Shutdown(ctx)
	}))

	// 7. Internal HTTP Server
	internalSrv := &http.Server{Addr: *internalListen, Handler: internalRouter}
	app.Register(closerFunc(func() error {
		log.Println("[shutdown] stopping internal http server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return internalSrv.Shutdown(ctx)
	}))

	go func() {
		log.Printf("External server listening on %s", *listen)
		if err := externalSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("external listen error: %v", err)
		}
	}()

	go func() {
		log.Printf("Internal server listening on %s", *internalListen)
		if err := internalSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("internal listen error: %v", err)
		}
	}()

	// Wait for signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	app.Shutdown()
}

func checkEnv() {
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		panic("DOMAIN environment variable is required")
	}
	log.Printf("Using domain: %s", domain)
	k8s.Domain = domain

	resend_api_key := os.Getenv("RESEND_API_KEY")
	if resend_api_key == "" {
		panic("RESEND_API_KEY environment variable is required")
	}
	log.Print("RESEND_API_KEY is set")
	handlers.RESEND_API_KEY = resend_api_key
	handlers.ResendClient = resend.NewClient(resend_api_key)
}

func crossOriginMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
