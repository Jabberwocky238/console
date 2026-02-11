package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"jabberwocky238/console/k8s"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type AcceptTaskRequest struct {
	TaskType  string  `json:"task_type" binding:"required"`
	Timestamp int64   `json:"timestamp" binding:"required"`
	Data      k8s.Job `json:"data" binding:"required"`
}

type JobsHandler struct {
	processor *k8s.Processor
	cron      *k8s.CronScheduler
}

func NewTaskHandler(proc *k8s.Processor, cron *k8s.CronScheduler) *JobsHandler {
	return &JobsHandler{
		processor: proc,
		cron:      cron,
	}
}

func (h *JobsHandler) AcceptTask(c *gin.Context) {
	var req AcceptTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate timestamp
	if req.Timestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid timestamp"})
		return
	}

	// TODO: Process the task based on task_type
	// You can add your business logic here

	c.JSON(http.StatusOK, gin.H{
		"message":     "task accepted",
		"task_type":   req.TaskType,
		"timestamp":   req.Timestamp,
		"received_at": time.Now().Unix(),
	})
}

// SendTask sends a task to the inner control plane endpoint
// Uses Kubernetes internal service: control-plane-inner.console.svc.cluster.local
func SendTask(job k8s.Job) error {
	endpoint := fmt.Sprintf("%s/api/acceptTask", k8s.ControlPlaneInnerEndpoint)

	req := AcceptTaskRequest{
		TaskType:  job.Type(),
		Timestamp: time.Now().Unix(),
		Data:      job,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send task: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("task rejected with status: %d", resp.StatusCode)
	}

	return nil
}
