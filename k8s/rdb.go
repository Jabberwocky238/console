package k8s

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"

	_ "github.com/lib/pq"
)

// RootRDBManager holds a persistent admin connection to CockroachDB
type RootRDBManager struct {
	mu  sync.Mutex
	db  *sql.DB
	dsn string
}

func InitRDBManager() error {
	RDBManager = &RootRDBManager{dsn: CockroachDBAdminDSN}
	_, err := RDBManager.tryGetDB()
	return err
}

// tryGetDB returns a healthy *sql.DB, reconnecting if needed (up to 3 attempts)
func (m *RootRDBManager) tryGetDB() (*sql.DB, error) {
	if m.db != nil {
		if err := m.db.Ping(); err == nil {
			return m.db, nil
		}
		log.Println("[rdb] existing connection lost, reconnecting...")
		m.db.Close()
		m.db = nil
	}
	for i := 0; i < 3; i++ {
		db, err := sql.Open("postgres", m.dsn)
		if err != nil {
			return nil, fmt.Errorf("sql.Open failed: %w", err)
		}
		if err := db.Ping(); err != nil {
			log.Printf("[rdb] ping attempt %d/3 failed: %v", i+1, err)
			db.Close()
			continue
		}
		log.Printf("[rdb] reconnected on attempt %d/3", i+1)
		m.db = db
		return db, nil
	}
	return nil, fmt.Errorf("cockroachdb unreachable after 3 attempts")
}

// Close closes the persistent admin connection
func (m *RootRDBManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db != nil {
		return m.db.Close()
	}
	return nil
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
	db, err := m.tryGetDB()
	if err != nil {
		return err
	}
	r := newUserRDB(userUID)
	if _, err := db.Exec(m.useDB(userUID)); err != nil {
		return err
	}
	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	if _, err := db.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schName)); err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf("GRANT ALL ON SCHEMA %s TO %s", schName, r.username()))
	return err
}

// DeleteSchema deletes a schema from user's database
func (m *RootRDBManager) DeleteSchema(userUID, schemaID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	db, err := m.tryGetDB()
	if err != nil {
		return err
	}
	if _, err := db.Exec(m.useDB(userUID)); err != nil {
		return err
	}
	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	_, err = db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schName))
	return err
}

// ListSchemas lists all schemas in user's database
func (m *RootRDBManager) ListSchemas(userUID string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	db, err := m.tryGetDB()
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(m.useDB(userUID)); err != nil {
		return nil, err
	}
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
func (m *RootRDBManager) SchemaExists(userUID, schemaID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	db, err := m.tryGetDB()
	if err != nil {
		return false, err
	}
	if _, err := db.Exec(m.useDB(userUID)); err != nil {
		return false, err
	}
	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = $1`, schName).Scan(&count)
	return count > 0, err
}

// InitUserRDB creates user and database for new user
func (m *RootRDBManager) InitUserRDB(userUID string) error {
	db, err := m.tryGetDB()
	if err != nil {
		return err
	}
	r := newUserRDB(userUID)
	if _, err := db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", r.database())); err != nil {
		return err
	}
	if _, err := db.Exec(fmt.Sprintf("CREATE USER IF NOT EXISTS %s", r.username())); err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf("GRANT ALL ON DATABASE %s TO %s", r.database(), r.username()))
	return err
}

// DeleteUserRDB deletes user's database and user
func (m *RootRDBManager) DeleteUserRDB(userUID string) error {
	db, err := m.tryGetDB()
	if err != nil {
		return err
	}
	r := newUserRDB(userUID)
	db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", r.database()))
	db.Exec(fmt.Sprintf("DROP USER IF EXISTS %s", r.username()))
	return nil
}

// DropDatabase 直接按数据库名删除（用于清理孤儿）
func (m *RootRDBManager) DropDatabase(dbName string) error {
	db, err := m.tryGetDB()
	if err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", dbName))
	return err
}

// ListUserDatabases 列出 CockroachDB 中所有 db_ 前缀的数据库名
func (m *RootRDBManager) ListUserDatabases() ([]string, error) {
	db, err := m.tryGetDB()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SHOW DATABASES`)
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

// DatabaseSize returns total size of user's database in bytes
func (m *RootRDBManager) DatabaseSize(userUID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	db, err := m.tryGetDB()
	if err != nil {
		return 0, err
	}
	if _, err := db.Exec(m.useDB(userUID)); err != nil {
		return 0, err
	}
	var size int64
	err = db.QueryRow(
		`SELECT COALESCE(SUM(total_bytes), 0)
		 FROM crdb_internal.table_span_stats`).Scan(&size)
	return size, err
}

// SchemaSize returns total size of a specific schema in bytes
func (m *RootRDBManager) SchemaSize(userUID, schemaID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	db, err := m.tryGetDB()
	if err != nil {
		return 0, err
	}
	if _, err := db.Exec(m.useDB(userUID)); err != nil {
		return 0, err
	}
	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	var size int64
	err = db.QueryRow(
		`SELECT COALESCE(SUM(s.total_bytes), 0)
		 FROM crdb_internal.table_span_stats s
		 JOIN crdb_internal.tables t ON t.table_id = s.table_id
		 WHERE t.schema_name = $1`, schName).Scan(&size)
	return size, err
}
