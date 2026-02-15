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

// VerificationCode model
type VerificationCode struct {
	ID        int       `json:"-"`
	Email     string    `json:"email"`
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
}

// CustomDomain model
type CustomDomain struct {
	ID        int       `json:"id"`
	CDID      string    `json:"cdid"`
	UserUID   string    `json:"user_uid"`
	Domain    string    `json:"domain"`
	Target    string    `json:"target"`
	TXTName   string    `json:"txt_name"`
	TXTValue  string    `json:"txt_value"`
	Status    string    `json:"status"` // pending, success, error
	CreatedAt time.Time `json:"created_at"`
}

// Worker model
type Worker struct {
	ID              int       `json:"id"`
	WID             string    `json:"worker_id"`
	UserUID         string    `json:"user_uid"`
	WorkerName      string    `json:"worker_name"`
	Status          string    `json:"status"` // unloaded, loading, active, error
	ActiveVersionID *int      `json:"active_version_id"`
	EnvJSON         string    `json:"env_json"`     // JSON object: {"KEY": "VALUE", ...}
	SecretsJSON     string    `json:"secrets_json"` // JSON array: ["secret1", "secret2", ...]
	AssignedCPU    string    `json:"assigned_cpu"`    // e.g. "1"
	AssignedMemory string    `json:"assigned_memory"` // e.g. "500Mi"
	AssignedDisk   string    `json:"assigned_disk"`   // e.g. "2Gi"
	MaxReplicas     int       `json:"max_replicas"`
	MainRegion      string    `json:"main_region"`
	CreatedAt       time.Time `json:"created_at"`
}

// WorkerDeployVersion model
type WorkerDeployVersion struct {
	ID        int       `json:"id"`
	WorkerID  int       `json:"worker_id"`
	Image     string    `json:"image"`
	Port      int       `json:"port"`
	Status    string    `json:"status"` // loading, success, error
	Msg       string    `json:"msg"`
	CreatedAt time.Time `json:"created_at"`
}

// CombinatorResource model
type CombinatorResource struct {
	ID           int       `json:"id"`
	UserUID      string    `json:"user_uid"`
	ResourceType string    `json:"resource_type"` // rdb, kv
	ResourceID   string    `json:"resource_id"`
	Status       string    `json:"status"` // loading, error, active
	Msg          string    `json:"msg"`
	CreatedAt    time.Time `json:"created_at"`
}

// CombinatorResourceUsage model
type CombinatorResourceReport struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id" binding:"required"`
	ResourceID    string    `json:"resource_id" binding:"required"`
	ResourceType  string    `json:"resource_type" binding:"required"`
	DataChange    int       `json:"datachange" binding:"required"`
	TimespanStart time.Time `json:"timespan_start" binding:"required"`
	TimespanEnd   time.Time `json:"timespan_end" binding:"required"`
}
