package main

import (
	"log"
	"strings"
	"time"

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
		var expiresAt time.Time
		err := DB.QueryRow(
			"SELECT id, expires_at FROM verification_codes WHERE email = $1 AND code = $2 AND used = false",
			req.Email, req.Code,
		).Scan(&codeID, &expiresAt)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid code"})
			return
		}

		if time.Now().After(expiresAt) {
			c.JSON(400, gin.H{"error": "code expired"})
			return
		}
	} else {
		// Special code for dev/testing
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to hash password"})
		return
	}

	var userUID string
	err = DB.QueryRow(
		"INSERT INTO users (uid, email, password_hash) VALUES ($1, $2, $3) RETURNING uid",
		GenerateUID(req.Email), req.Email, hash,
	).Scan(&userUID)
	if err != nil {
		c.JSON(400, gin.H{"error": "email already exists"})
		return
	}

	// Mark code as used
	if req.Code != SPECIAL_CODE {
		DB.Exec("UPDATE verification_codes SET used = true WHERE id = $1", codeID)
	}

	// Create K8s pod for user
	if err := CreateUserPod(userUID); err != nil {
		log.Printf("Warning: Failed to create pod for user %s: %v", userUID, err)
	}

	token, _ := GenerateToken(userUID, req.Email)
	c.JSON(200, gin.H{"user_id": userUID, "email": req.Email, "token": token})
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

	var user User
	err := DB.QueryRow(
		"SELECT uid, email, password_hash FROM users WHERE email = $1",
		req.Email,
	).Scan(&user.UID, &user.Email, &user.PasswordHash)
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

	var rdbUID string
	err := DB.QueryRow(
		`INSERT INTO user_rdbs (user_id, uid, name, rdb_type, url)
		 VALUES ((SELECT id FROM users WHERE uid = $1), $2, $3, $4, $5)
		 RETURNING uid`,
		userUID, GenerateResourceUID(), req.Name, req.Type, req.URL,
	).Scan(&rdbUID)
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to create RDB"})
		return
	}

	// Trigger config update
	if err := UpdateUserConfig(userUID); err != nil {
		log.Printf("Failed to update config for user %s: %v", userUID, err)
	}

	c.JSON(200, gin.H{"id": rdbUID, "name": req.Name})
}

// ListRDBs lists all RDB resources for user
func ListRDBs(c *gin.Context) {
	userUID := c.GetString("user_id")
	rows, err := DB.Query(
		`SELECT uid, name, rdb_type, url, enabled FROM user_rdbs
		 WHERE user_id = (SELECT id FROM users WHERE uid = $1)`,
		userUID,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to query"})
		return
	}
	defer rows.Close()

	var rdbs []RDB
	for rows.Next() {
		var r RDB
		rows.Scan(&r.UID, &r.Name, &r.Type, &r.URL, &r.Enabled)
		rdbs = append(rdbs, r)
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

	var kvUID string
	err := DB.QueryRow(
		`INSERT INTO user_kvs (user_id, uid, name, kv_type, url)
		 VALUES ((SELECT id FROM users WHERE uid = $1), $2, $3, $4, $5)
		 RETURNING uid`,
		userUID, GenerateResourceUID(), req.Name, req.Type, req.URL,
	).Scan(&kvUID)
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to create KV"})
		return
	}

	// Trigger config update
	if err := UpdateUserConfig(userUID); err != nil {
		log.Printf("Failed to update config for user %s: %v", userUID, err)
	}

	c.JSON(200, gin.H{"id": kvUID, "name": req.Name})
}

// ListKVs lists all KV resources for user
func ListKVs(c *gin.Context) {
	userUID := c.GetString("user_id")
	rows, err := DB.Query(
		`SELECT uid, name, kv_type, url, enabled FROM user_kvs
		 WHERE user_id = (SELECT id FROM users WHERE uid = $1)`,
		userUID,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to query"})
		return
	}
	defer rows.Close()

	var kvs []KV
	for rows.Next() {
		var k KV
		rows.Scan(&k.UID, &k.Name, &k.Type, &k.URL, &k.Enabled)
		kvs = append(kvs, k)
	}
	c.JSON(200, gin.H{"kvs": kvs})
}

// DeleteRDB deletes an RDB resource
func DeleteRDB(c *gin.Context) {
	userUID := c.GetString("user_id")
	rdbUID := c.Param("id")

	result, err := DB.Exec(
		`DELETE FROM user_rdbs
		 WHERE uid = $1 AND user_id = (SELECT id FROM users WHERE uid = $2)`,
		rdbUID, userUID,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to delete"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}

	// Trigger config update
	if err := UpdateUserConfig(userUID); err != nil {
		log.Printf("Failed to update config for user %s: %v", userUID, err)
	}

	c.JSON(200, gin.H{"message": "deleted"})
}

// DeleteKV deletes a KV resource
func DeleteKV(c *gin.Context) {
	userUID := c.GetString("user_id")
	kvUID := c.Param("id")

	result, err := DB.Exec(
		`DELETE FROM user_kvs
		 WHERE uid = $1 AND user_id = (SELECT id FROM users WHERE uid = $2)`,
		kvUID, userUID,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to delete"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}

	// Trigger config update
	if err := UpdateUserConfig(userUID); err != nil {
		log.Printf("Failed to update config for user %s: %v", userUID, err)
	}

	c.JSON(200, gin.H{"message": "deleted"})
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

	_, err := DB.Exec(
		"INSERT INTO verification_codes (email, code, expires_at) VALUES ($1, $2, $3)",
		req.Email, code, expiresAt,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to save code"})
		return
	}

	// TODO: Send email with code
	// For now, just return it in response (dev only)
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

	// Verify code
	var codeID int
	var expiresAt time.Time
	err := DB.QueryRow(
		"SELECT id, expires_at FROM verification_codes WHERE email = $1 AND code = $2 AND used = false",
		req.Email, req.Code,
	).Scan(&codeID, &expiresAt)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid code"})
		return
	}

	if time.Now().After(expiresAt) {
		c.JSON(400, gin.H{"error": "code expired"})
		return
	}

	// Hash new password
	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to hash password"})
		return
	}

	// Update password
	_, err = DB.Exec(
		"UPDATE users SET password_hash = $1 WHERE email = $2",
		hash, req.Email,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to update password"})
		return
	}

	// Mark code as used
	DB.Exec("UPDATE verification_codes SET used = true WHERE id = $1", codeID)

	c.JSON(200, gin.H{"message": "password reset successfully"})
}

// Health handles health check endpoint
func Health(c *gin.Context) {
	status := gin.H{
		"status": "ok",
		"timestamp": time.Now().Unix(),
	}

	// Check database connection
	if DB != nil {
		if err := DB.Ping(); err != nil {
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
	if K8sClient != nil {
		status["kubernetes"] = "healthy"
	} else {
		status["kubernetes"] = "not_initialized"
	}

	c.JSON(200, status)
}
