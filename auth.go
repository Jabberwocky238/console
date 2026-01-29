package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var JWTSecret []byte

// GenerateUID generates a UID from email (lowercase letters before @) + 4 random digits
func GenerateUID(email string) string {
	// Extract part before @
	atIndex := 0
	for i, c := range email {
		if c == '@' {
			atIndex = i
			break
		}
	}

	prefix := ""
	if atIndex > 0 {
		prefix = email[:atIndex]
	}

	// Convert to lowercase and keep only letters
	var letters []rune
	for _, c := range prefix {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			if c >= 'A' && c <= 'Z' {
				c = c + 32 // Convert to lowercase
			}
			letters = append(letters, c)
		}
	}

	// Generate 4 random digits
	bytes := make([]byte, 2)
	rand.Read(bytes)
	randomNum := (int(bytes[0])<<8 | int(bytes[1])) % 10000

	return fmt.Sprintf("%s%04d", string(letters), randomNum)
}

// GenerateResourceUID generates a random UID for resources (RDB/KV)
func GenerateResourceUID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	// Generate a 16-character hex string
	return fmt.Sprintf("%x", bytes)
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(bytes), err
}

// CheckPassword checks if password matches hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateToken generates a JWT token for user
func GenerateToken(userID, email string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(JWTSecret)
}

// ValidateToken validates JWT token and returns user_id
func ValidateToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return JWTSecret, nil
	})
	if err != nil {
		return "", err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID, ok := claims["user_id"].(string)
		if !ok {
			return "", errors.New("invalid token claims")
		}
		return userID, nil
	}
	return "", errors.New("invalid token")
}

// GenerateCode generates a 6-digit verification code
func GenerateCode() string {
	return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
}
