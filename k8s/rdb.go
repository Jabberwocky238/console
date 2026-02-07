package k8s

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"

	_ "github.com/lib/pq"
)

// RootRDBManager holds a persistent admin connection to CockroachDB
type RootRDBManager struct {
	mu sync.Mutex
	db *sql.DB
}

// InitRDBManager creates a RootRDBManager with a persistent admin connection
func InitRDBManager() error {
	db, err := sql.Open("postgres", CockroachDBAdminDSN)
	if err != nil {
		return err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return err
	}
	RDBManager = &RootRDBManager{db: db}
	return nil
}

// Close closes the persistent admin connection
func (m *RootRDBManager) Close() error {
	return m.db.Close()
}

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

// sanitize replaces invalid characters for SQL identifiers
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return strings.ToLower(s)
}

// userRDB represents user's database info (internal only)
type userRDB struct {
	userUID string
}

func newUserRDB(userUID string) *userRDB {
	return &userRDB{userUID: userUID}
}

func (r *userRDB) username() string {
	return fmt.Sprintf("user_%s", sanitize(r.userUID))
}

func (r *userRDB) database() string {
	return fmt.Sprintf("db_%s", sanitize(r.userUID))
}

func (r *userRDB) dsnWithSchema(schemaID string) string {
	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	return fmt.Sprintf("postgresql://%s@%s:%s/%s?sslmode=disable&search_path=%s",
		r.username(), CockroachDBHost, CockroachDBPort, r.database(), schName)
}

// DSNWithSchema returns connection string with specific schema (exported for external use)
func (m *RootRDBManager) DSNWithSchema(userUID, schemaID string) string {
	return newUserRDB(userUID).dsnWithSchema(schemaID)
}

// DatabaseName returns db_<uid> (exported for external use)
func (m *RootRDBManager) DatabaseName(userUID string) string {
	return newUserRDB(userUID).database()
}

func (m *RootRDBManager) useDB(userUID string) string {
	return fmt.Sprintf("SET DATABASE = %s", newUserRDB(userUID).database())
}

// CreateSchema creates a new schema in user's database
func (m *RootRDBManager) CreateSchema(userUID, schemaID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := newUserRDB(userUID)
	if _, err := m.db.Exec(m.useDB(userUID)); err != nil {
		return err
	}
	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	if _, err := m.db.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schName)); err != nil {
		return err
	}
	_, err := m.db.Exec(fmt.Sprintf("GRANT ALL ON SCHEMA %s TO %s", schName, r.username()))
	return err
}

// DeleteSchema deletes a schema from user's database
func (m *RootRDBManager) DeleteSchema(userUID, schemaID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.db.Exec(m.useDB(userUID)); err != nil {
		return err
	}
	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	_, err := m.db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schName))
	return err
}

// ListSchemas lists all schemas in user's database
func (m *RootRDBManager) ListSchemas(userUID string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.db.Exec(m.useDB(userUID)); err != nil {
		return nil, err
	}
	rows, err := m.db.Query(`SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'schema_%'`)
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
func (m *RootRDBManager) SchemaExists(userUID, schemaID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.db.Exec(m.useDB(userUID)); err != nil {
		return false, err
	}
	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	var count int
	err := m.db.QueryRow(`SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = $1`, schName).Scan(&count)
	return count > 0, err
}

// InitUserRDB creates user and database for new user
func (m *RootRDBManager) InitUserRDB(userUID string) error {
	r := newUserRDB(userUID)

	if _, err := m.db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", r.database())); err != nil {
		return err
	}
	if _, err := m.db.Exec(fmt.Sprintf("CREATE USER IF NOT EXISTS %s", r.username())); err != nil {
		return err
	}
	if _, err := m.db.Exec(fmt.Sprintf("GRANT ALL ON DATABASE %s TO %s", r.database(), r.username())); err != nil {
		return err
	}

	return nil
}

// DeleteUserRDB deletes user's database and user
func (m *RootRDBManager) DeleteUserRDB(userUID string) error {
	r := newUserRDB(userUID)
	m.db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", r.database()))
	m.db.Exec(fmt.Sprintf("DROP USER IF EXISTS %s", r.username()))
	return nil
}

// DropDatabase 直接按数据库名删除（用于清理孤儿）
func (m *RootRDBManager) DropDatabase(dbName string) error {
	_, err := m.db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", dbName))
	return err
}

// ListUserDatabases 列出 CockroachDB 中所有 db_ 前缀的数据库名
func (m *RootRDBManager) ListUserDatabases() ([]string, error) {
	rows, err := m.db.Query(`SHOW DATABASES`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dbs []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if strings.HasPrefix(name, "db_") {
			dbs = append(dbs, name)
		}
	}
	return dbs, nil
}

