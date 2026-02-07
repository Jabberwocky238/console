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
	listen := flag.String("l", "localhost:9900", "Listen address")
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

	// 5. Cron
	cron := k8s.NewCronScheduler(proc)
	cron.RegisterJob(24*time.Hour, &handlers.UserAuditJob{})
	cron.RegisterJob(12*time.Hour, &handlers.DomainCheckJob{})
	proc.Submit(&handlers.UserAuditJob{})
	cron.Start()
	app.Register(cron)

	log.Println("Control plane starting...")

	// Setup Gin router
	r := gin.Default()
	if os.Getenv("ENV") == "test" {
		r.Use(crossOriginMiddleware())
	}
	// Health check endpoint
	r.GET("/health", handlers.Health)

	// Serve frontend static files from dist/
	r.Static("/assets", "./dist/assets")
	r.GET("/", func(c *gin.Context) {
		c.File("./dist/index.html")
	})

	api := r.Group("/api")
	// Public routes
	api.POST("/auth/register", ah.Register)
	api.POST("/auth/login", handlers.Login)
	api.POST("/auth/send-code", handlers.SendCode)
	api.POST("/auth/reset-password", handlers.ResetPassword)

	// Protected routes
	api.Use(handlers.AuthMiddleware())
	{
		api.GET("/rdb", ch.ListRDBs)
		api.GET("/rdb/:id", ch.GetRDB)
		api.POST("/rdb", ch.CreateRDB)
		api.DELETE("/rdb/:id", ch.DeleteRDB)

		api.GET("/kv", ch.ListKVs)
		api.POST("/kv", ch.CreateKV)
		api.DELETE("/kv/:id", ch.DeleteKV)

		api.GET("/worker", wh.ListWorkers)
		api.GET("/worker/:id", wh.GetWorker)
		api.POST("/worker", wh.CreateWorker)
		api.DELETE("/worker/:id", wh.DeleteWorker)

		api.GET("/worker/:id/env", wh.GetWorkerEnv)
		api.POST("/worker/:id/env", wh.SetWorkerEnv)
		api.GET("/worker/:id/secret", wh.GetWorkerSecrets)
		api.POST("/worker/:id/secret", wh.SetWorkerSecrets)

		api.GET("/domain", handlers.ListCustomDomains)
		api.GET("/domain/:id", handlers.GetCustomDomain)
		api.POST("/domain", handlers.AddCustomDomain)
		api.DELETE("/domain/:id", handlers.DeleteCustomDomain)
	}

	// Sensitive routes (signature required)
	sensitive := r.Group("/api")
	sensitive.Use(handlers.SignatureMiddleware())
	{
		sensitive.POST("/worker/deploy", wh.DeployWorker)
	}

	// Fallback: serve static files from dist/ or index.html for SPA
	r.NoRoute(func(c *gin.Context) {
		filePath := path.Join("./dist", c.Request.URL.Path)
		if _, err := os.Stat(filePath); err == nil {
			c.File(filePath)
			return
		}
		c.File("./dist/index.html")
	})

	// 6. HTTP Server
	srv := &http.Server{Addr: *listen, Handler: r}
	app.Register(closerFunc(func() error {
		log.Println("[shutdown] stopping http server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}))

	go func() {
		log.Printf("Server listening on %s", *listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
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
