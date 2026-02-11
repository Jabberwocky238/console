package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/handlers"
	"jabberwocky238/console/k8s"

	"github.com/gin-gonic/gin"
	"github.com/resend/resend-go/v3"
)

func main() {
	listen := flag.String("l", "0.0.0.0:9900", "External listen address")
	dbDSN := flag.String("d", "postgresql://myuser:your_password@localhost:5432/mydb?sslmode=disable", "Database DSN")

	flag.Parse()
	debug := os.Getenv("ENV") == "test"
	if !debug {
		checkEnvOuter()
	}

	// 1. Database
	if err := dblayer.InitDB(*dbDSN); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
		panic("Failed to connect to database:" + err.Error())
	}
	defer dblayer.DB.Close()

	// 2. CockroachDB
	if err := k8s.InitRDBManager(); err != nil {
		log.Fatalf("CockroachDB init failed: %v", err)
		panic("CockroachDB init failed: " + err.Error())
	}
	defer k8s.RDBManager.Close()

	wh := handlers.NewWorkerHandler()
	ch := handlers.NewCombinatorHandler()

	log.Println("Outer gateway starting...")

	// Setup External Gin router (public access)
	router := gin.Default()
	router.GET("/health", handlers.HealthOuter)
	if debug {
		router.Use(crossOriginMiddleware())
	}

	// Serve frontend static files from dist/
	router.Static("/assets", "./dist/assets")
	router.GET("/", func(c *gin.Context) {
		c.File("./dist/index.html")
	})

	api := router.Group("/api")
	// Public routes
	api.POST("/auth/register", handlers.Register)
	api.POST("/auth/login", handlers.Login)
	api.POST("/auth/send-code", handlers.SendCode)
	api.POST("/auth/reset-password", handlers.ResetPassword)

	// Protected routes (auth required)
	protected := api.Group("")
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
	sensitive := api.Group("")
	sensitive.Use(handlers.SignatureMiddleware())
	{
		sensitive.POST("/worker/deploy", wh.DeployWorker)
	}

	// HTTP Server
	srv := &http.Server{Addr: *listen, Handler: router}
	go func() {
		log.Printf("Outer gateway listening on %s", *listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	// Wait for signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	srv.Shutdown(context.Background())
}

func checkEnvOuter() {
	var shouldPanic bool = false
	requiredEnvs := []string{"DOMAIN", "RESEND_API_KEY"}
	for _, env := range requiredEnvs {
		thisVar := os.Getenv(env)
		if thisVar == "" {
			log.Printf("Environment variable %s is required but not set", env)
			shouldPanic = true
			continue
		} else {
			log.Printf("Environment variable %s is set", env)
			switch env {
			case "DOMAIN":
				k8s.Domain = thisVar
			case "RESEND_API_KEY":
				handlers.RESEND_API_KEY = thisVar
				handlers.ResendClient = resend.NewClient(handlers.RESEND_API_KEY)
			}
		}
	}
	if shouldPanic {
		log.Fatalf("ENV not set, panic")
		panic("One or more required environment variables are not set")
	}
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
