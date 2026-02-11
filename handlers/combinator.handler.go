package handlers

import (
	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/handlers/jobs"
	"jabberwocky238/console/k8s"

	"github.com/gin-gonic/gin"
)

type CombinatorHandler struct {
	proc *k8s.Processor
}

func NewCombinatorHandler(proc *k8s.Processor) *CombinatorHandler {
	return &CombinatorHandler{proc: proc}
}

// CreateRDB creates a new RDB resource record and submits async job
func (h *CombinatorHandler) CreateRDB(c *gin.Context) {
	userUID := c.GetString("user_id")
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	resourceID := GenerateResourceUID()
	if err := dblayer.CreateCombinatorResource(userUID, "rdb", resourceID); err != nil {
		c.JSON(500, gin.H{"error": "failed to create resource: " + err.Error()})
		return
	}

	if err := SendTask(jobs.NewCreateRDBJob(userUID, req.Name, resourceID)); err != nil {
		c.JSON(500, gin.H{"error": "failed to enqueue create task"})
		return
	}

	c.JSON(200, gin.H{"id": resourceID, "status": "loading"})
}

// ListRDBs lists all RDB resources for user from database
func (h *CombinatorHandler) ListRDBs(c *gin.Context) {
	userUID := c.GetString("user_id")

	resources, err := dblayer.ListCombinatorResources(userUID, "rdb")
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to list resources: " + err.Error()})
		return
	}

	var dbSize int64
	if k8s.RDBManager != nil {
		dbSize, _ = k8s.RDBManager.DatabaseSize(userUID)
	}

	c.JSON(200, gin.H{"rdbs": resources, "database_size": dbSize})
}

// GetRDB returns detail of a single RDB resource including schema size
func (h *CombinatorHandler) GetRDB(c *gin.Context) {
	userUID := c.GetString("user_id")
	resourceID := c.Param("id")

	cr, err := dblayer.GetCombinatorResource(userUID, "rdb", resourceID)
	if err != nil {
		c.JSON(404, gin.H{"error": "resource not found"})
		return
	}

	var schemaSize int64
	if k8s.RDBManager != nil {
		schemaSize, _ = k8s.RDBManager.SchemaSize(userUID, cr.ResourceID)
	}

	c.JSON(200, gin.H{
		"id":          cr.ID,
		"resource_id": cr.ResourceID,
		"status":      cr.Status,
		"msg":         cr.Msg,
		"created_at":  cr.CreatedAt,
		"schema_size": schemaSize,
	})
}

// CreateKV creates a new KV resource record and submits async job
func (h *CombinatorHandler) CreateKV(c *gin.Context) {
	userUID := c.GetString("user_id")

	resourceID := GenerateResourceUID()
	if err := dblayer.CreateCombinatorResource(userUID, "kv", resourceID); err != nil {
		c.JSON(500, gin.H{"error": "failed to create resource: " + err.Error()})
		return
	}

	if err := SendTask(jobs.NewCreateKVJob(userUID, resourceID)); err != nil {
		c.JSON(500, gin.H{"error": "failed to enqueue create task"})
		return
	}

	c.JSON(200, gin.H{"id": resourceID, "status": "loading"})
}

// ListKVs lists all KV resources for user from database
func (h *CombinatorHandler) ListKVs(c *gin.Context) {
	userUID := c.GetString("user_id")

	resources, err := dblayer.ListCombinatorResources(userUID, "kv")
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to list resources: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{"kvs": resources})
}

// DeleteRDB deletes an RDB resource record and submits async job
func (h *CombinatorHandler) DeleteRDB(c *gin.Context) {
	userUID := c.GetString("user_id")
	resourceID := c.Param("id")

	cr, err := dblayer.GetCombinatorResource(userUID, "rdb", resourceID)
	if err != nil {
		c.JSON(404, gin.H{"error": "resource not found"})
		return
	}

	if err := dblayer.DeleteCombinatorResource(userUID, "rdb", resourceID); err != nil {
		c.JSON(500, gin.H{"error": "failed to delete resource: " + err.Error()})
		return
	}

	if err := SendTask(jobs.NewDeleteRDBJob(userUID, cr.ResourceID)); err != nil {
		c.JSON(500, gin.H{"error": "failed to enqueue delete task"})
		return
	}

	c.JSON(200, gin.H{"message": "deleted"})
}

// DeleteKV deletes a KV resource record and submits async job
func (h *CombinatorHandler) DeleteKV(c *gin.Context) {
	userUID := c.GetString("user_id")
	resourceID := c.Param("id")

	cr, err := dblayer.GetCombinatorResource(userUID, "kv", resourceID)
	if err != nil {
		c.JSON(404, gin.H{"error": "resource not found"})
		return
	}

	if err := dblayer.DeleteCombinatorResource(userUID, "kv", resourceID); err != nil {
		c.JSON(500, gin.H{"error": "failed to delete resource: " + err.Error()})
		return
	}

	if err := SendTask(jobs.NewDeleteKVJob(userUID, cr.ResourceID)); err != nil {
		c.JSON(500, gin.H{"error": "failed to enqueue delete task"})
		return
	}

	c.JSON(200, gin.H{"message": "deleted"})
}
