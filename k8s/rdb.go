package k8s

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	RDBNamespace        = "cockroachdb"
	CockroachDBHost     = "cockroachdb-public.cockroachdb.svc.cluster.local"
	CockroachDBPort     = "26257"
	CockroachDBAdminDSN = "postgresql://root@cockroachdb-public.cockroachdb.svc.cluster.local:26257?sslmode=disable"
)

func init() {
	if v := os.Getenv("COCKROACHDB_ADMIN_DSN"); v != "" {
		CockroachDBAdminDSN = v
	}
	if v := os.Getenv("COCKROACHDB_HOST"); v != "" {
		CockroachDBHost = v
	}
	if v := os.Getenv("COCKROACHDB_PORT"); v != "" {
		CockroachDBPort = v
	}
}

// UserRDB represents user's database info
type UserRDB struct {
	UserUID  string
	Password string
}

// sanitize replaces invalid characters for SQL identifiers
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return strings.ToLower(s)
}

// generatePassword generates a random password
func generatePassword() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 24)
	for i := range b {
		b[i] = chars[i%len(chars)]
	}
	return string(b)
}

// Username returns user_<uid>
func (r *UserRDB) Username() string {
	return fmt.Sprintf("user_%s", sanitize(r.UserUID))
}

// Database returns db_<uid>
func (r *UserRDB) Database() string {
	return fmt.Sprintf("db_%s", sanitize(r.UserUID))
}

// DSN returns full connection string
func (r *UserRDB) DSN() string {
	return fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		r.Username(), r.Password, CockroachDBHost, CockroachDBPort, r.Database())
}

// secretName returns rdb-secret-<uid>
func (r *UserRDB) secretName() string {
	return fmt.Sprintf("rdb-secret-%s", r.UserUID)
}

// getDB returns connection to user's database
func (r *UserRDB) getDB() (*sql.DB, error) {
	dsn := CockroachDBAdminDSN + "/" + r.Database()
	return sql.Open("postgres", dsn)
}

// CreateSchema creates a new schema in user's database
func (r *UserRDB) CreateSchema(schemaID string) error {
	db, err := r.getDB()
	if err != nil {
		return err
	}
	defer db.Close()

	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	if _, err := db.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schName)); err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf("GRANT ALL ON SCHEMA %s TO %s", schName, r.Username()))
	return err
}

// DeleteSchema deletes a schema from user's database
func (r *UserRDB) DeleteSchema(schemaID string) error {
	db, err := r.getDB()
	if err != nil {
		return err
	}
	defer db.Close()

	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	_, err = db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schName))
	return err
}

// ListSchemas lists all schemas in user's database
func (r *UserRDB) ListSchemas() ([]string, error) {
	db, err := r.getDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'schema_%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			schemas = append(schemas, strings.TrimPrefix(name, "schema_"))
		}
	}
	return schemas, nil
}

// SchemaExists checks if schema exists
func (r *UserRDB) SchemaExists(schemaID string) (bool, error) {
	db, err := r.getDB()
	if err != nil {
		return false, err
	}
	defer db.Close()

	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = $1`, schName).Scan(&count)
	return count > 0, err
}

// InitUserRDB creates user and database for new user
func InitUserRDB(userUID string) (*UserRDB, error) {
	db, err := sql.Open("postgres", CockroachDBAdminDSN)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	r := &UserRDB{UserUID: userUID, Password: generatePassword()}

	// Create database
	if _, err := db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", r.Database())); err != nil {
		return nil, err
	}

	// Create user
	if _, err := db.Exec(fmt.Sprintf("CREATE USER IF NOT EXISTS %s WITH PASSWORD '%s'", r.Username(), r.Password)); err != nil {
		return nil, err
	}

	// Grant privileges
	if _, err := db.Exec(fmt.Sprintf("GRANT ALL ON DATABASE %s TO %s", r.Database(), r.Username())); err != nil {
		return nil, err
	}

	// Store secret
	if err := r.saveSecret(); err != nil {
		return nil, err
	}

	return r, nil
}

// saveSecret stores password in K8s Secret
func (r *UserRDB) saveSecret() error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	ctx := context.Background()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.secretName(),
			Namespace: RDBNamespace,
			Labels:    map[string]string{"app": "user-rdb", "user-uid": r.UserUID},
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{"password": r.Password},
	}
	_, err := K8sClient.CoreV1().Secrets(RDBNamespace).Create(ctx, secret, metav1.CreateOptions{})
	return err
}

// GetUserRDB retrieves user's RDB from K8s Secret
func GetUserRDB(userUID string) (*UserRDB, error) {
	if K8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	ctx := context.Background()
	r := &UserRDB{UserUID: userUID}
	secret, err := K8sClient.CoreV1().Secrets(RDBNamespace).Get(ctx, r.secretName(), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	r.Password = string(secret.Data["password"])
	return r, nil
}

// DeleteUserRDB deletes user's database, user and secret
func DeleteUserRDB(userUID string) error {
	db, err := sql.Open("postgres", CockroachDBAdminDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	r := &UserRDB{UserUID: userUID}
	db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", r.Database()))
	db.Exec(fmt.Sprintf("DROP USER IF EXISTS %s", r.Username()))

	if K8sClient != nil {
		ctx := context.Background()
		K8sClient.CoreV1().Secrets(RDBNamespace).Delete(ctx, r.secretName(), metav1.DeleteOptions{})
	}
	return nil
}
