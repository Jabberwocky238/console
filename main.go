package main

import (
	"flag"
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	// Parse flags
	listen := flag.String("l", "localhost:9900", "Listen address")
	dbDSN := flag.String("d", "", "Database DSN")
	kubeconfig := flag.String("k", "", "Kubeconfig path (empty for in-cluster)")
	flag.Parse()

	// Get JWT secret from env
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "defaultsecret"
	}
	JWTSecret = []byte(jwtSecret)

	// Initialize database
	if err := InitDB(*dbDSN); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer DB.Close()

	// Initialize K8s client
	if err := InitK8s(*kubeconfig); err != nil {
		log.Printf("Warning: K8s client init failed: %v", err)
		log.Println("Running without K8s integration")
	} else {
		log.Println("K8s client initialized")
	}

	log.Println("Control plane starting...")

	// Setup Gin router
	r := gin.Default()

	// Health check endpoint
	r.GET("/health", Health)

	// Serve index.html at root
	r.StaticFile("/", "./index.html")

	// Public routes
	r.POST("/auth/register", Register)
	r.POST("/auth/login", Login)
	r.POST("/auth/send-code", SendCode)
	r.POST("/auth/reset-password", ResetPassword)

	// Protected routes
	api := r.Group("/api")
	api.Use(AuthMiddleware())
	{
		api.POST("/rdb", CreateRDB)
		api.GET("/rdb", ListRDBs)
		api.DELETE("/rdb/:id", DeleteRDB)
		api.POST("/kv", CreateKV)
		api.GET("/kv", ListKVs)
		api.DELETE("/kv/:id", DeleteKV)
	}

	// Start server
	log.Printf("Server listening on %s", *listen)
	r.Run(*listen)
}
