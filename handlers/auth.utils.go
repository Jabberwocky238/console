package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var JWTSecret []byte

// 50个单词的词表，用于生成用户ID
var wordList = []string{
	"apple", "banana", "cherry", "dragon", "eagle",
	"falcon", "grape", "honey", "ivory", "jungle",
	"koala", "lemon", "mango", "noble", "ocean",
	"panda", "queen", "river", "storm", "tiger",
	"ultra", "vivid", "whale", "xenon", "yacht",
	"zebra", "alpha", "brave", "coral", "delta",
	"ember", "frost", "ghost", "haven", "index",
	"joker", "karma", "lunar", "maple", "nexus",
	"orbit", "pixel", "quest", "radar", "solar",
	"terra", "unity", "venom", "wired", "zesty",
}

// GenerateUID generates a 12-character UID: 4-6 letters from email + random digits
func GenerateUID(email string) string {
	// Extract letters before @
	atIndex := 0
	for i, c := range email {
		if c == '@' {
			atIndex = i
			break
		}
	}

	var letters []rune
	if atIndex > 0 {
		for _, c := range email[:atIndex] {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				if c >= 'A' && c <= 'Z' {
					c = c + 32
				}
				letters = append(letters, c)
			}
		}
	}

	// Get prefix: 4-6 letters or random word
	var prefix string
	if len(letters) >= 4 {
		maxLen := 6
		if len(letters) < maxLen {
			maxLen = len(letters)
		}
		prefix = string(letters[:maxLen])
	} else {
		// Use random word from wordList
		bytes := make([]byte, 1)
		rand.Read(bytes)
		prefix = wordList[int(bytes[0])%len(wordList)]
	}

	// Generate random digits to fill up to 12 characters
	digitCount := 12 - len(prefix)
	bytes := make([]byte, 4)
	rand.Read(bytes)
	randomNum := int(bytes[0])<<24 | int(bytes[1])<<16 | int(bytes[2])<<8 | int(bytes[3])
	if randomNum < 0 {
		randomNum = -randomNum
	}

	// Mod to fit digit count
	mod := 1
	for range digitCount {
		mod *= 10
	}
	randomNum = randomNum % mod

	return fmt.Sprintf("%s%0*d", prefix, digitCount, randomNum)
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

// GenerateSecretKey generates a 32-byte secret key for HMAC
func GenerateSecretKey() string {
	bytes := make([]byte, 24)
	rand.Read(bytes)
	return "sk_" + base64.RawURLEncoding.EncodeToString(bytes)
}

// GenerateHMACSignature generates HMAC-SHA256 signature
func GenerateHMACSignature(secretKey string, data []byte) string {
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write(data)
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// VerifyHMACSignature verifies HMAC-SHA256 signature
func VerifyHMACSignature(secretKey string, data []byte, signature string) error {
	expected := GenerateHMACSignature(secretKey, data)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return errors.New("invalid signature")
	}
	return nil
}
