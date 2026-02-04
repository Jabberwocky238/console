package main

import (
	"flag"
	"log"
	"os"

	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/handlers"
	"jabberwocky238/console/k8s"

	"github.com/gin-gonic/gin"
)

func main() {
	// Parse flags
	listen := flag.String("l", "localhost:9900", "Listen address")
	dbDSN := flag.String("d", "", "Database DSN")
	kubeconfig := flag.String("k", "", "Kubeconfig path (empty for in-cluster)")
	flag.Parse()

	// Check DOMAIN environment variable (required for IngressRoute creation)
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		log.Fatal("DOMAIN environment variable is required")
	}
	log.Printf("Using domain: %s", domain)
	k8s.Domain = domain

	// Initialize database
	if err := dblayer.InitDB(*dbDSN); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer dblayer.DB.Close()

	// Initialize K8s client
	if err := k8s.InitK8s(*kubeconfig); err != nil {
		log.Printf("Warning: K8s client init failed: %v", err)
		log.Println("Running without K8s integration")
	} else {
		log.Println("K8s client initialized")
	}

	log.Println("Control plane starting...")

	// Setup Gin router
	r := gin.Default()

	// Health check endpoint
	r.GET("/health", handlers.Health)

	// Serve static files
	r.StaticFile("/", "./index.html")
	r.StaticFile("/index.js", "./index.js")

	// Public routes
	r.POST("/auth/register", handlers.Register)
	r.POST("/auth/login", handlers.Login)
	r.POST("/auth/send-code", handlers.SendCode)
	r.POST("/auth/reset-password", handlers.ResetPassword)

	// Protected routes
	api := r.Group("/api")
	api.Use(handlers.AuthMiddleware())
	{
		api.POST("/rdb", handlers.CreateRDB)
		api.GET("/rdb", handlers.ListRDBs)
		api.DELETE("/rdb/:id", handlers.DeleteRDB)
		api.POST("/kv", handlers.CreateKV)
		api.GET("/kv", handlers.ListKVs)
		api.DELETE("/kv/:id", handlers.DeleteKV)

		// Worker routes
		api.POST("/worker", handlers.RegisterWorker)
		api.GET("/worker", handlers.ListWorkers)
		api.GET("/worker/:id", handlers.GetWorker)
		api.DELETE("/worker/:id", handlers.DeleteWorker)
	}

	// Start server
	log.Printf("Server listening on %s", *listen)
	r.Run(*listen)
}
