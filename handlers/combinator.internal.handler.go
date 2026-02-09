package handlers

import (
	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/k8s"

	"github.com/gin-gonic/gin"
)

// CombinatorInternalHandler handles internal combinator API requests
type CombinatorInternalHandler struct {
	proc *k8s.Processor
}

func NewCombinatorInternalHandler(proc *k8s.Processor) *CombinatorInternalHandler {
	return &CombinatorInternalHandler{proc: proc}
}

// RetrieveSecretByID retrieves all active combinator resources and their secrets for a user
func (h *CombinatorInternalHandler) RetrieveSecretByID(c *gin.Context) {
	userUID := c.GetString("user_id")

	// Get user secret key
	secretKey, err := dblayer.GetUserSecretKey(userUID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to get user secret: " + err.Error()})
		return
	}

	// Get all active resources
	resources, err := dblayer.ListActiveCombinatorResources(userUID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to list resources: " + err.Error()})
		return
	}

	// Build response with secrets
	type ResourceWithSecret struct {
		ResourceType string `json:"resource_type"`
		ResourceID   string `json:"resource_id"`
	}

	var result []ResourceWithSecret
	for _, res := range resources {
		item := ResourceWithSecret{
			ResourceType: res.ResourceType,
			ResourceID:   res.ResourceID,
		}

		result = append(result, item)
	}

	c.JSON(200, gin.H{"resources": result, "secret_key": secretKey})
}
