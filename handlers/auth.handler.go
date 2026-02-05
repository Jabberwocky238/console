package handlers

import (
	"log"
	"strings"
	"time"

	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/k8s"

	"github.com/gin-gonic/gin"
)

var SPECIAL_CODE = "701213"

// Register handles user registration
func Register(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required,min=2"`
		Code     string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Verify code
	var codeID int
	if req.Code != SPECIAL_CODE {
		id, expiresAt, err := dblayer.GetVerificationCode(req.Email, req.Code)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid code"})
			return
		}
		codeID = id

		if time.Now().After(expiresAt) {
			c.JSON(400, gin.H{"error": "code expired"})
			return
		}
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to hash password"})
		return
	}

	// Generate secret key for HMAC
	secretKey := GenerateSecretKey()

	userUID, err := dblayer.CreateUser(GenerateUID(req.Email), req.Email, hash, secretKey)
	if err != nil {
		c.JSON(400, gin.H{"error": "email already exists"})
		return
	}

	// Mark code as used
	if req.Code != SPECIAL_CODE {
		dblayer.MarkCodeUsed(codeID)
	}

	// Create K8s pod for user
	if err := k8s.CreateCombinatorPod(userUID); err != nil {
		log.Printf("Warning: Failed to create pod for user %s: %v", userUID, err)
	}

	token, _ := GenerateToken(userUID, req.Email)
	c.JSON(200, gin.H{
		"user_id":    userUID,
		"email":      req.Email,
		"token":      token,
		"secret_key": secretKey,
	})
}

// Login handles user login
func Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	user, err := dblayer.GetUserByEmail(req.Email)
	if err != nil {
		c.JSON(401, gin.H{"error": "invalid credentials"})
		return
	}

	if !CheckPassword(req.Password, user.PasswordHash) {
		c.JSON(401, gin.H{"error": "invalid credentials"})
		return
	}

	token, _ := GenerateToken(user.UID, user.Email)
	c.JSON(200, gin.H{"user_id": user.UID, "token": token})
}

// AuthMiddleware validates JWT token
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(401, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		userID, err := ValidateToken(token)
		if err != nil {
			c.JSON(401, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}

// SignatureMiddleware validates HMAC signature for requests
func SignatureMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		signature := c.GetHeader("X-Combinator-Signature")
		if signature == "" {
			c.JSON(401, gin.H{"error": "signature required"})
			c.Abort()
			return
		}

		userID := c.GetHeader("X-Combinator-User-ID")
		if userID == "" {
			c.JSON(401, gin.H{"error": "user_id required"})
			c.Abort()
			return
		}

		timestamp := c.GetHeader("X-Combinator-Timestamp")
		if timestamp == "" {
			c.JSON(401, gin.H{"error": "timestamp required"})
			c.Abort()
			return
		}

		// Get user's secret key
		secretKey, err := dblayer.GetUserSecretKey(userID)
		if err != nil {
			c.JSON(401, gin.H{"error": "invalid user"})
			c.Abort()
			return
		}

		// Read request body
		body, err := c.GetRawData()
		if err != nil {
			c.JSON(400, gin.H{"error": "failed to read body"})
			c.Abort()
			return
		}

		// Verify signature: HMAC(body + timestamp)
		payload := append(body, []byte(timestamp)...)
		if err := VerifyHMACSignature(secretKey, payload, signature); err != nil {
			c.JSON(401, gin.H{"error": "invalid signature"})
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}

// SendCode sends verification code to email
func SendCode(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	code := GenerateCode()
	expiresAt := time.Now().Add(10 * time.Minute)

	if err := dblayer.SaveVerificationCode(req.Email, code, expiresAt); err != nil {
		c.JSON(500, gin.H{"error": "failed to save code"})
		return
	}

	c.JSON(200, gin.H{"message": "code sent", "code": code})
}

// ResetPassword resets password with verification code
func ResetPassword(c *gin.Context) {
	var req struct {
		Email       string `json:"email" binding:"required,email"`
		Code        string `json:"code" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	codeID, expiresAt, err := dblayer.GetVerificationCode(req.Email, req.Code)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid code"})
		return
	}

	if time.Now().After(expiresAt) {
		c.JSON(400, gin.H{"error": "code expired"})
		return
	}

	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to hash password"})
		return
	}

	if err := dblayer.UpdateUserPassword(req.Email, hash); err != nil {
		c.JSON(500, gin.H{"error": "failed to update password"})
		return
	}

	dblayer.MarkCodeUsed(codeID)
	c.JSON(200, gin.H{"message": "password reset successfully"})
}
