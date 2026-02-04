package handlers

import (
	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/k8s"

	"github.com/gin-gonic/gin"
)

// RegisterWorker registers a new worker
func RegisterWorker(c *gin.Context) {
	userUID := c.GetString("user_id")

	var req struct {
		WorkerID string `json:"worker_id" binding:"required"`
		Image    string `json:"image" binding:"required"`
		Port     int    `json:"port" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Save to database
	if err := dblayer.CreateWorker(req.WorkerID, userUID, req.Image, req.Port); err != nil {
		c.JSON(500, gin.H{"error": "failed to create worker record"})
		return
	}

	// Deploy to K8s
	worker := &k8s.Worker{
		WorkerID: req.WorkerID,
		OwnerID:  userUID,
		Image:    req.Image,
		Port:     req.Port,
	}

	if err := k8s.DeployWorker(worker); err != nil {
		dblayer.SetWorkerStatus(req.WorkerID, userUID, "error", err.Error())
		c.JSON(200, gin.H{
			"worker_id": req.WorkerID,
			"status":    "error",
			"error":     err.Error(),
		})
		return
	}

	dblayer.SetWorkerStatus(req.WorkerID, userUID, "active", "")
	c.JSON(200, gin.H{
		"worker_id": req.WorkerID,
		"status":    "active",
		"domain":    worker.Name() + ".worker." + k8s.Domain,
	})
}

// DeleteWorker deletes a worker
func DeleteWorker(c *gin.Context) {
	userUID := c.GetString("user_id")
	workerID := c.Param("id")

	// Get worker from database
	dbWorker, err := dblayer.GetWorker(workerID, userUID)
	if err != nil {
		c.JSON(404, gin.H{"error": "worker not found"})
		return
	}

	// Delete from K8s
	worker := &k8s.Worker{
		WorkerID: dbWorker.WorkerID,
		OwnerID:  dbWorker.OwnerID,
		Image:    dbWorker.Image,
		Port:     dbWorker.Port,
	}

	if err := k8s.DeleteWorker(worker); err != nil {
		c.JSON(500, gin.H{"error": "failed to delete worker from k8s"})
		return
	}

	// Delete from database
	if err := dblayer.DeleteWorker(workerID, userUID); err != nil {
		c.JSON(500, gin.H{"error": "failed to delete worker record"})
		return
	}

	c.JSON(200, gin.H{"message": "worker deleted"})
}

// ListWorkers lists all workers for the user
func ListWorkers(c *gin.Context) {
	userUID := c.GetString("user_id")

	workers, err := dblayer.ListWorkersByOwner(userUID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to list workers"})
		return
	}

	c.JSON(200, workers)
}

// GetWorker gets a single worker
func GetWorker(c *gin.Context) {
	userUID := c.GetString("user_id")
	workerID := c.Param("id")

	worker, err := dblayer.GetWorker(workerID, userUID)
	if err != nil {
		c.JSON(404, gin.H{"error": "worker not found"})
		return
	}

	c.JSON(200, worker)
}
