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

	// Get database DSN
	dsn := *dbDSN
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		dsn = "host=localhost port=5432 user=postgres password=postgres dbname=combfather sslmode=disable"
	}

	// Initialize database
	if err := InitDB(dsn); err != nil {
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
		api.POST("/combinator", AddCombinator)
		api.DELETE("/combinator", DeleteCombinator)
	}

	// Start server
	log.Printf("Server listening on %s", *listen)
	r.Run(*listen)
}
