package handlers

import (
	"time"

	"jabberwocky238/console/k8s"
	"jabberwocky238/console/dblayer"

	"github.com/gin-gonic/gin"
)

// Health handles health check endpoint
func Health(c *gin.Context) {
	status := gin.H{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
	}

	// Check database connection
	if dblayer.DB != nil {
		if err := dblayer.DB.Ping(); err != nil {
			status["database"] = "unhealthy"
			status["database_error"] = err.Error()
			c.JSON(503, status)
			return
		}
		status["database"] = "healthy"
	} else {
		status["database"] = "not_initialized"
	}

	// Check K8s client
	if k8s.K8sClient != nil {
		status["kubernetes"] = "healthy"
	} else {
		status["kubernetes"] = "not_initialized"
	}

	c.JSON(200, status)
}
