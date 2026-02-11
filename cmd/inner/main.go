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
)

func main() {
	listen := flag.String("l", "0.0.0.0:9901", "Internal listen address")
	dbDSN := flag.String("d", "postgresql://myuser:your_password@localhost:5432/mydb?sslmode=disable", "Database DSN")
	flag.Parse()

	// 1. Database
	if err := dblayer.InitDB(*dbDSN); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer dblayer.DB.Close()

	// 2. CockroachDB
	if err := k8s.InitRDBManager(); err != nil {
		panic("CockroachDB init failed: " + err.Error())
	}
	defer k8s.RDBManager.Close()

	// 3. Processor
	proc := k8s.NewProcessor(256, 4)
	proc.Start()
	defer proc.Close()

	wh := handlers.NewWorkerHandler(proc)
	cih := handlers.NewCombinatorInternalHandler(proc)

	log.Println("Inner gateway starting...")

	// Setup Internal Gin router (internal services access)
	router := gin.Default()
	router.GET("/health", handlers.Health)

	api := router.Group("/api")
	{
		// Internal routes (no auth required, only accessible from cluster)
		api.POST("/worker/deploy", wh.DeployWorker)
		api.GET("/combinator/retrieveSecretByID", cih.RetrieveSecretByID)
		api.POST("/combinator/reportUsage", cih.ReportUsage)
	}

	// HTTP Server
	srv := &http.Server{Addr: *listen, Handler: router}

	go func() {
		log.Printf("Inner gateway listening on %s", *listen)
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
