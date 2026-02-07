package main

import (
	"flag"
	"log"
	"os"
	"path"
	"time"

	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/handlers"
	"jabberwocky238/console/k8s"
	"jabberwocky238/console/k8s/controller"

	"github.com/gin-gonic/gin"
	"github.com/resend/resend-go/v3"
)

func main() {
	// Parse flags
	listen := flag.String("l", "localhost:9900", "Listen address")
	dbDSN := flag.String("d", "postgresql://myuser:your_password@localhost:5432/mydb?sslmode=disable", "Database DSN")
	kubeconfig := flag.String("k", "", "Kubeconfig path (empty for in-cluster)")
	flag.Parse()

	// Check DOMAIN environment variable (required for IngressRoute creation)
	// 检查是否为测试环境
	if os.Getenv("ENV") != "test" {
		checkEnv()
	}

	// Initialize database
	if err := dblayer.InitDB(*dbDSN); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer dblayer.DB.Close()

	// Initialize CockroachDB admin connection
	if err := k8s.InitRDBManager(); err != nil {
		log.Printf("Warning: CockroachDB admin init failed: %v", err)
		log.Println("Running without CockroachDB integration")
	} else {
		log.Println("CockroachDB admin connection initialized")
		defer k8s.RDBManager.Close()
	}

	// Initialize K8s client
	if err := k8s.InitK8s(*kubeconfig); err != nil {
		log.Printf("Warning: K8s client init failed: %v", err)
		log.Println("Running without K8s integration")
	} else {
		log.Println("K8s client initialized")

		// Ensure CRDs exist
		if err := controller.EnsureCRD(k8s.RestConfig); err != nil {
			log.Printf("Warning: WorkerApp CRD ensure failed: %v", err)
		}
		if err := controller.EnsureCombinatorCRD(k8s.RestConfig); err != nil {
			log.Printf("Warning: CombinatorApp CRD ensure failed: %v", err)
		}

		// Start WorkerApp controller (informer)
		stopCh := make(chan struct{})
		defer close(stopCh)
		ctrl := controller.NewController(k8s.DynamicClient, k8s.K8sClient)
		go ctrl.Start(stopCh)
	}

	// Start shared job processor
	proc := k8s.NewProcessor(256, 4)
	proc.Start()

	wh := handlers.NewWorkerHandler(proc)
	ah := handlers.NewAuthHandler(proc)
	ch := handlers.NewCombinatorHandler(proc)

	// Start periodic domain check
	k8s.StartPeriodicCheck()

	// Start cron scheduler for periodic jobs
	cron := k8s.NewCronScheduler(proc)
	cron.RegisterJob(24*time.Hour, &handlers.UserAuditJob{})
	proc.Submit(&handlers.UserAuditJob{}) // Run once at startup
	cron.Start()
	defer cron.Stop()

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

	// Start server
	log.Printf("Server listening on %s", *listen)
	r.Run(*listen)
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
