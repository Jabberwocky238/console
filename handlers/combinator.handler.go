package handlers

import (
	"jabberwocky238/console/dblayer"
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

	id := GenerateResourceUID()
	resourceID := GenerateResourceUID()
	if err := dblayer.CreateCombinatorResource(id, userUID, "rdb", resourceID); err != nil {
		c.JSON(500, gin.H{"error": "failed to create resource: " + err.Error()})
		return
	}

	h.proc.Submit(NewCreateRDBJob(id, userUID, req.Name, resourceID))

	c.JSON(200, gin.H{"id": id, "status": "loading"})
}

// ListRDBs lists all RDB resources for user from database
func (h *CombinatorHandler) ListRDBs(c *gin.Context) {
	userUID := c.GetString("user_id")

	resources, err := dblayer.ListCombinatorResources(userUID, "rdb")
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to list resources: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{"rdbs": resources})
}

// CreateKV creates a new KV resource record and submits async job
func (h *CombinatorHandler) CreateKV(c *gin.Context) {
	userUID := c.GetString("user_id")
	var req struct {
		Type string `json:"kv_type" binding:"required"`
		URL  string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	id := GenerateResourceUID()
	resourceID := GenerateResourceUID()
	if err := dblayer.CreateCombinatorResource(id, userUID, "kv", resourceID); err != nil {
		c.JSON(500, gin.H{"error": "failed to create resource: " + err.Error()})
		return
	}

	h.proc.Submit(NewCreateKVJob(id, userUID, resourceID, req.Type, req.URL))

	c.JSON(200, gin.H{"id": id, "status": "loading"})
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
	id := c.Param("id")

	cr, err := dblayer.GetCombinatorResource(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "resource not found"})
		return
	}
	if cr.UserUID != userUID {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}

	if err := dblayer.DeleteCombinatorResource(id); err != nil {
		c.JSON(500, gin.H{"error": "failed to delete resource: " + err.Error()})
		return
	}

	h.proc.Submit(NewDeleteRDBJob(userUID, cr.ResourceID))

	c.JSON(200, gin.H{"message": "deleted"})
}

// DeleteKV deletes a KV resource record and submits async job
func (h *CombinatorHandler) DeleteKV(c *gin.Context) {
	userUID := c.GetString("user_id")
	id := c.Param("id")

	cr, err := dblayer.GetCombinatorResource(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "resource not found"})
		return
	}
	if cr.UserUID != userUID {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}

	if err := dblayer.DeleteCombinatorResource(id); err != nil {
		c.JSON(500, gin.H{"error": "failed to delete resource: " + err.Error()})
		return
	}

	h.proc.Submit(NewDeleteKVJob(userUID, cr.ResourceID))

	c.JSON(200, gin.H{"message": "deleted"})
}
