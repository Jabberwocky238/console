package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/handlers"
	"jabberwocky238/console/handlers/jobs"
	"jabberwocky238/console/k8s"
	"jabberwocky238/console/k8s/controller"

	"github.com/gin-gonic/gin"
)

func main() {
	listen := flag.String("l", "0.0.0.0:9901", "Internal listen address")
	dbDSN := flag.String("d", "postgresql://myuser:your_password@localhost:5432/mydb?sslmode=disable", "Database DSN")
	kubeconfig := flag.String("k", "", "Kubeconfig path (empty for in-cluster)")
	flag.Parse()
	debug := os.Getenv("ENV") == "test"
	if !debug {
		checkEnvInner()
	}

	// 1. Database
	log.Printf("try to connect to database: " + *dbDSN)
	if err := dblayer.InitDB(*dbDSN); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
		panic("Failed to connect to database:" + err.Error())
	}
	defer dblayer.DB.Close()
	log.Printf("Database connected successfully")

	// 2. CockroachDB
	log.Printf("try to InitRDBManager")
	if err := k8s.InitRDBManager(); err != nil {
		log.Printf("Warning: CockroachDB init failed: %v", err)
		log.Println("Inner gateway will continue without RDB support")
	} else {
		defer k8s.RDBManager.Close()
		log.Println("CockroachDB initialized")
	}

	// 3. K8s + Controller
	log.Printf("try to InitK8s")
	if err := k8s.InitK8s(*kubeconfig); err != nil {
		log.Printf("Warning: K8s client init failed: %v", err)
		panic("K8s client init failed: " + err.Error())
	} else {
		log.Println("K8s client initialized, EnsureCRD and start controller")
		controller.EnsureCRD(k8s.RestConfig)
		stopCh := make(chan struct{})
		defer close(stopCh)
		ctrl := controller.NewController(k8s.DynamicClient, k8s.K8sClient)
		go ctrl.Start(stopCh)
	}

	// 4. Processor and Cron
	proc := k8s.NewProcessor(256, 4)
	cron := k8s.NewCronScheduler(proc)
	proc.Start()
	cron.Start()
	defer proc.Close()
	defer cron.Close()

	cron.RegisterJob(24*time.Hour, jobs.NewUserAuditJob())
	cron.RegisterJob(12*time.Hour, jobs.NewDomainCheckJob())
	proc.Submit(jobs.NewUserAuditJob())

	wh := handlers.NewWorkerHandler()
	cih := handlers.NewCombinatorInternalHandler(proc)
	th := handlers.NewTaskHandler(proc, cron)

	log.Println("Inner gateway starting...")

	// Setup Internal Gin router (internal services access)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/health", handlers.HealthInner)
	// 过滤 /health 请求的日志
	router.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/health"},
	}))
	api := router.Group("/api")
	{
		// Internal routes (no auth required, only accessible from cluster)
		api.POST("/worker/deploy", wh.DeployWorker)
		api.GET("/combinator/retrieveSecretByID", cih.RetrieveSecretByID)
		api.POST("/combinator/reportUsage", cih.ReportUsage)
		api.POST("/acceptTask", th.AcceptTask)
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

func checkEnvInner() {
	var shouldPanic bool = false
	requiredEnvs := []string{"DOMAIN"}
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
			}
		}
	}
	if shouldPanic {
		log.Fatalf("ENV not set, panic")
		panic("One or more required environment variables are not set")
	}
}
