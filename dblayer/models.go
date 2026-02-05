package dblayer

import "time"

// User model
type User struct {
	ID           int       `json:"-"`
	UID          string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	SecretKey    string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// RDB model
type RDB struct {
	ID       int    `json:"-"`
	UID      string `json:"id"`
	UserID   int    `json:"-"`
	Name     string `json:"name"`
	Type     string `json:"rdb_type"`
	URL      string `json:"url"`
	Enabled  bool   `json:"enabled"`
	Status   string `json:"status"`
	ErrorMsg string `json:"error_msg,omitempty"`
}

// KV model
type KV struct {
	ID       int    `json:"-"`
	UID      string `json:"id"`
	UserID   int    `json:"-"`
	Name     string `json:"name"`
	Type     string `json:"kv_type"`
	URL      string `json:"url"`
	Enabled  bool   `json:"enabled"`
	Status   string `json:"status"`
	ErrorMsg string `json:"error_msg,omitempty"`
}

// VerificationCode model
type VerificationCode struct {
	ID        int       `json:"-"`
	Email     string    `json:"email"`
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
}

// Worker model
type Worker struct {
	ID        int       `json:"-"`
	WorkerID  string    `json:"worker_id"`
	OwnerID   string    `json:"owner_id"`
	Image     string    `json:"image"`
	Port      int       `json:"port"`
	Status    string    `json:"status"`
	ErrorMsg  string    `json:"error_msg,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
