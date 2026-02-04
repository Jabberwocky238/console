package handlers

import (
	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/k8s"

	"github.com/gin-gonic/gin"
)

// CreateRDB creates a new RDB resource
func CreateRDB(c *gin.Context) {
	userUID := c.GetString("user_id")
	var req struct {
		Name string `json:"name" binding:"required"`
		Type string `json:"rdb_type" binding:"required"`
		URL  string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	rdbUID, err := dblayer.CreateRDB(userUID, GenerateResourceUID(), req.Name, req.Type, req.URL)
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to create RDB"})
		return
	}

	// Reload combinator config
	if err := k8s.UpdateCombinatorConfig(userUID); err != nil {
		dblayer.SetRDBStatus(rdbUID, "error", err.Error())
		c.JSON(200, gin.H{"id": rdbUID, "error": "RDB created but failed to update config, err: " + err.Error()})
	} else {
		dblayer.SetRDBStatus(rdbUID, "active", "")
		c.JSON(200, gin.H{"id": rdbUID, "message": "RDB created successfully"})
	}
}

// ListRDBs lists all RDB resources for user
func ListRDBs(c *gin.Context) {
	userUID := c.GetString("user_id")
	rdbs, err := dblayer.ListRDBsByUser(userUID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to query"})
		return
	}
	c.JSON(200, gin.H{"rdbs": rdbs})
}

// CreateKV creates a new KV resource
func CreateKV(c *gin.Context) {
	userUID := c.GetString("user_id")
	var req struct {
		Name string `json:"name" binding:"required"`
		Type string `json:"kv_type" binding:"required"`
		URL  string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	kvUID, err := dblayer.CreateKV(userUID, GenerateResourceUID(), req.Name, req.Type, req.URL)
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to create KV"})
		return
	}

	// Reload combinator config
	if err := k8s.UpdateCombinatorConfig(userUID); err != nil {
		dblayer.SetKVStatus(kvUID, "error", err.Error())
		c.JSON(200, gin.H{"id": kvUID, "error": "KV created but failed to update config, err: " + err.Error()})
	} else {
		dblayer.SetKVStatus(kvUID, "active", "")
		c.JSON(200, gin.H{"id": kvUID, "message": "KV created successfully"})
	}
}

// ListKVs lists all KV resources for user
func ListKVs(c *gin.Context) {
	userUID := c.GetString("user_id")
	kvs, err := dblayer.ListKVsByUser(userUID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to query"})
		return
	}
	c.JSON(200, gin.H{"kvs": kvs})
}

// DeleteRDB deletes an RDB resource
func DeleteRDB(c *gin.Context) {
	userUID := c.GetString("user_id")
	rdbUID := c.Param("id")

	rows, err := dblayer.DeleteRDB(rdbUID, userUID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to delete"})
		return
	}
	if rows == 0 {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}

	c.JSON(200, gin.H{"message": "deleted"})
}

// DeleteKV deletes a KV resource
func DeleteKV(c *gin.Context) {
	userUID := c.GetString("user_id")
	kvUID := c.Param("id")

	rows, err := dblayer.DeleteKV(kvUID, userUID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to delete"})
		return
	}
	if rows == 0 {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}

	c.JSON(200, gin.H{"message": "deleted"})
}
