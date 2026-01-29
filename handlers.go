package main

import (
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

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

	hash, err := HashPassword(req.Password)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to hash password"})
		return
	}

	var userUUID string
	err = DB.QueryRow(
		"INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING uuid",
		req.Email, hash,
	).Scan(&userUUID)
	if err != nil {
		c.JSON(400, gin.H{"error": "email already exists"})
		return
	}

	// Mark code as used
	DB.Exec("UPDATE verification_codes SET used = true WHERE id = $1", codeID)

	// Create K8s pod for user
	if err := CreateUserPod(userUUID); err != nil {
		log.Printf("Warning: Failed to create pod for user %s: %v", userUUID, err)
	}

	token, _ := GenerateToken(userUUID, req.Email)
	c.JSON(200, gin.H{"user_id": userUUID, "email": req.Email, "token": token})
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
		"SELECT uuid, email, password_hash FROM users WHERE email = $1",
		req.Email,
	).Scan(&user.UUID, &user.Email, &user.PasswordHash)
	if err != nil {
		c.JSON(401, gin.H{"error": "invalid credentials"})
		return
	}

	if !CheckPassword(req.Password, user.PasswordHash) {
		c.JSON(401, gin.H{"error": "invalid credentials"})
		return
	}

	token, _ := GenerateToken(user.UUID, user.Email)
	c.JSON(200, gin.H{"user_id": user.UUID, "token": token})
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
	userUUID := c.GetString("user_id")
	var req struct {
		Name string `json:"name" binding:"required"`
		Type string `json:"rdb_type" binding:"required"`
		URL  string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	var rdbUUID string
	err := DB.QueryRow(
		`INSERT INTO user_rdbs (user_id, name, rdb_type, url)
		 VALUES ((SELECT id FROM users WHERE uuid = $1), $2, $3, $4)
		 RETURNING uuid`,
		userUUID, req.Name, req.Type, req.URL,
	).Scan(&rdbUUID)
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to create RDB"})
		return
	}

	// Trigger config update
	if err := UpdateUserConfig(userUUID); err != nil {
		log.Printf("Failed to update config for user %s: %v", userUUID, err)
	}

	c.JSON(200, gin.H{"id": rdbUUID, "name": req.Name})
}

// ListRDBs lists all RDB resources for user
func ListRDBs(c *gin.Context) {
	userUUID := c.GetString("user_id")
	rows, err := DB.Query(
		`SELECT uuid, name, rdb_type, url, enabled FROM user_rdbs
		 WHERE user_id = (SELECT id FROM users WHERE uuid = $1)`,
		userUUID,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to query"})
		return
	}
	defer rows.Close()

	var rdbs []RDB
	for rows.Next() {
		var r RDB
		rows.Scan(&r.UUID, &r.Name, &r.Type, &r.URL, &r.Enabled)
		rdbs = append(rdbs, r)
	}
	c.JSON(200, gin.H{"rdbs": rdbs})
}

// CreateKV creates a new KV resource
func CreateKV(c *gin.Context) {
	userUUID := c.GetString("user_id")
	var req struct {
		Name string `json:"name" binding:"required"`
		Type string `json:"kv_type" binding:"required"`
		URL  string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	var kvUUID string
	err := DB.QueryRow(
		`INSERT INTO user_kvs (user_id, name, kv_type, url)
		 VALUES ((SELECT id FROM users WHERE uuid = $1), $2, $3, $4)
		 RETURNING uuid`,
		userUUID, req.Name, req.Type, req.URL,
	).Scan(&kvUUID)
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to create KV"})
		return
	}

	// Trigger config update
	if err := UpdateUserConfig(userUUID); err != nil {
		log.Printf("Failed to update config for user %s: %v", userUUID, err)
	}

	c.JSON(200, gin.H{"id": kvUUID, "name": req.Name})
}

// ListKVs lists all KV resources for user
func ListKVs(c *gin.Context) {
	userUUID := c.GetString("user_id")
	rows, err := DB.Query(
		`SELECT uuid, name, kv_type, url, enabled FROM user_kvs
		 WHERE user_id = (SELECT id FROM users WHERE uuid = $1)`,
		userUUID,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to query"})
		return
	}
	defer rows.Close()

	var kvs []KV
	for rows.Next() {
		var k KV
		rows.Scan(&k.UUID, &k.Name, &k.Type, &k.URL, &k.Enabled)
		kvs = append(kvs, k)
	}
	c.JSON(200, gin.H{"kvs": kvs})
}

// DeleteRDB deletes an RDB resource
func DeleteRDB(c *gin.Context) {
	userUUID := c.GetString("user_id")
	rdbUUID := c.Param("id")

	result, err := DB.Exec(
		`DELETE FROM user_rdbs
		 WHERE uuid = $1 AND user_id = (SELECT id FROM users WHERE uuid = $2)`,
		rdbUUID, userUUID,
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
	if err := UpdateUserConfig(userUUID); err != nil {
		log.Printf("Failed to update config for user %s: %v", userUUID, err)
	}

	c.JSON(200, gin.H{"message": "deleted"})
}

// DeleteKV deletes a KV resource
func DeleteKV(c *gin.Context) {
	userUUID := c.GetString("user_id")
	kvUUID := c.Param("id")

	result, err := DB.Exec(
		`DELETE FROM user_kvs
		 WHERE uuid = $1 AND user_id = (SELECT id FROM users WHERE uuid = $2)`,
		kvUUID, userUUID,
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
	if err := UpdateUserConfig(userUUID); err != nil {
		log.Printf("Failed to update config for user %s: %v", userUUID, err)
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
