package handlers

import (
	"jabberwocky238/console/k8s"

	"github.com/gin-gonic/gin"
)

// AddCustomDomain handles adding a new custom domain
func AddCustomDomain(c *gin.Context) {
	userUID := c.GetString("user_id")
	var req struct {
		Domain string `json:"domain" binding:"required"`
		Target string `json:"target" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	cd, err := k8s.NewCustomDomain(userUID, req.Domain, req.Target)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	cd.StartVerification()

	c.JSON(200, gin.H{
		"id":        cd.ID,
		"domain":    cd.Domain,
		"target":    cd.Target,
		"txt_name":  cd.TXTName,
		"txt_value": cd.TXTValue,
		"status":    cd.Status,
	})
}

// ListCustomDomains lists all custom domains for user
func ListCustomDomains(c *gin.Context) {
	userUID := c.GetString("user_id")
	domains := k8s.ListCustomDomains(userUID)
	c.JSON(200, gin.H{"domains": domains})
}

// GetCustomDomain gets a custom domain by ID
func GetCustomDomain(c *gin.Context) {
	cdid := c.Param("id")
	cd, err := k8s.GetCustomDomain(cdid)
	if err != nil {
		c.JSON(404, gin.H{"error": "domain not found: " + err.Error()})
		return
	}
	c.JSON(200, cd)
}

// DeleteCustomDomain deletes a custom domain
func DeleteCustomDomain(c *gin.Context) {
	cdid := c.Param("id")
	if err := k8s.DeleteCustomDomain(cdid); err != nil {
		c.JSON(404, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "deleted"})
}
