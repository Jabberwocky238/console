package k8s

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq"
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
	UserUID string
}

// sanitize replaces invalid characters for SQL identifiers
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return strings.ToLower(s)
}

// Username returns user_<uid>
func (r *UserRDB) Username() string {
	return fmt.Sprintf("user_%s", sanitize(r.UserUID))
}

// Database returns db_<uid>
func (r *UserRDB) Database() string {
	return fmt.Sprintf("db_%s", sanitize(r.UserUID))
}

// DSN returns full connection string (no password in insecure mode)
func (r *UserRDB) DSN() string {
	return fmt.Sprintf("postgresql://%s@%s:%s/%s?sslmode=disable",
		r.Username(), CockroachDBHost, CockroachDBPort, r.Database())
}

// DSNWithSchema returns connection string with specific schema
func (r *UserRDB) DSNWithSchema(schemaID string) string {
	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	return fmt.Sprintf("postgresql://%s@%s:%s/%s?sslmode=disable&search_path=%s",
		r.Username(), CockroachDBHost, CockroachDBPort, r.Database(), schName)
}

// getDB returns connection to user's database
func (r *UserRDB) getDB() (*sql.DB, error) {
	dsn := fmt.Sprintf("postgresql://root@%s:%s/%s?sslmode=disable", CockroachDBHost, CockroachDBPort, r.Database())
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

	r := &UserRDB{UserUID: userUID}

	// Create database
	if _, err := db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", r.Database())); err != nil {
		return nil, err
	}

	// Create user (no password in insecure mode)
	if _, err := db.Exec(fmt.Sprintf("CREATE USER IF NOT EXISTS %s", r.Username())); err != nil {
		return nil, err
	}

	// Grant privileges
	if _, err := db.Exec(fmt.Sprintf("GRANT ALL ON DATABASE %s TO %s", r.Database(), r.Username())); err != nil {
		return nil, err
	}

	return r, nil
}

// DeleteUserRDB deletes user's database and user
func DeleteUserRDB(userUID string) error {
	db, err := sql.Open("postgres", CockroachDBAdminDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	r := &UserRDB{UserUID: userUID}
	db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", r.Database()))
	db.Exec(fmt.Sprintf("DROP USER IF EXISTS %s", r.Username()))
	return nil
}
