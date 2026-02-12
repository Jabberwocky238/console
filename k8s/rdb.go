package k8s

import (
	"container/list"
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// RootRDBManager holds persistent connections to CockroachDB
type RootRDBManager struct {
	rootmu  sync.RWMutex
	rootDB  *sql.DB
	rootDSN string

	userMgr *userDBManager
}

// userDBManager manages user database connection pool
type userDBManager struct {
	mu      sync.RWMutex
	userDBs map[string]*userDBEntry
	lruList *list.List
}

type userDBEntry struct {
	mu      sync.RWMutex
	db      *sql.DB
	element *list.Element
}

func newUserDBManager() *userDBManager {
	return &userDBManager{
		userDBs: make(map[string]*userDBEntry, UserDBPoolSize),
		lruList: list.New(),
	}
}

func (e *userDBEntry) close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.db != nil {
		e.db.Close()
		e.db = nil
	}
}

func (e *userDBEntry) getDB() *sql.DB {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.db
}

// getOrCreateUserDB returns a healthy user *sql.DB from pool, reconnecting if needed
func (mgr *userDBManager) getOrCreateUserDB(userUID string) (*sql.DB, *userRDB, error) {
	userRDB := newUserRDB(userUID)

	// Fast path: try to get existing connection with read lock
	mgr.mu.RLock()
	entry, exists := mgr.userDBs[userUID]
	mgr.mu.RUnlock()

	if exists {
		// Check connection health outside manager lock
		db := entry.getDB()
		if db != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := db.PingContext(ctx)
			cancel()
			if err == nil {
				// Connection is healthy, update LRU with write lock
				mgr.mu.Lock()
				mgr.lruList.MoveToFront(entry.element)
				mgr.mu.Unlock()
				return db, userRDB, nil
			}
			// Connection exists but unhealthy
			log.Printf("[rdb] user %s connection lost, reconnecting...", userUID)
		}
		// db == nil means connection is being closed or not initialized, fall through to slow path
	}

	// Slow path: need to create/reconnect, acquire write lock
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	// Double check after acquiring write lock
	if entry, exists := mgr.userDBs[userUID]; exists {
		db := entry.getDB()
		if db != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := db.PingContext(ctx)
			cancel()
			if err == nil {
				mgr.lruList.MoveToFront(entry.element)
				return db, userRDB, nil
			}
		}
		// Clean up dead connection
		entry.close()
		mgr.lruList.Remove(entry.element)
		delete(mgr.userDBs, userUID)
	}

	// Evict oldest connection if pool is full (LRU)
	if len(mgr.userDBs) >= UserDBPoolSize {
		oldest := mgr.lruList.Back()
		if oldest != nil {
			oldUID := oldest.Value.(string)
			if entry, exists := mgr.userDBs[oldUID]; exists {
				entry.close()
				delete(mgr.userDBs, oldUID)
			}
			mgr.lruList.Remove(oldest)
			log.Printf("[rdb] evicted LRU user connection: %s", oldUID)
		}
	}

	// Try to connect
	for i := 0; i < 3; i++ {
		db, err := sql.Open("postgres", userRDB.dsn())
		if err != nil {
			return nil, nil, fmt.Errorf("sql.Open failed: %w", err)
		}

		// Configure connection pool
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(2)
		db.SetConnMaxLifetime(5 * time.Minute)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := db.PingContext(ctx); err != nil {
			cancel()
			log.Printf("[rdb] user %s ping attempt %d/3 failed: %v", userUID, i+1, err)
			db.Close()
			continue
		}
		cancel()
		log.Printf("[rdb] user %s connected on attempt %d/3", userUID, i+1)

		// Add to LRU cache
		element := mgr.lruList.PushFront(userUID)
		mgr.userDBs[userUID] = &userDBEntry{
			db:      db,
			element: element,
		}
		return db, userRDB, nil
	}
	return nil, nil, fmt.Errorf("user db unreachable after 3 attempts")
}

// closeAll closes all user connections
func (mgr *userDBManager) closeAll() {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	for _, entry := range mgr.userDBs {
		entry.close()
	}
}

var (
	UserDBPoolSize = 64
)

func InitRDBManager() error {
	RDBManager = &RootRDBManager{
		rootDSN: CockroachDBAdminDSN,
		userMgr: newUserDBManager(),
	}
	_, err := RDBManager.tryGetRootDB()
	return err
}

// tryGetRootDB returns a healthy root *sql.DB, reconnecting if needed (up to 3 attempts)
func (m *RootRDBManager) tryGetRootDB() (*sql.DB, error) {
	// Fast path: try read lock first
	m.rootmu.RLock()
	if m.rootDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := m.rootDB.PingContext(ctx)
		cancel()
		if err == nil {
			db := m.rootDB
			m.rootmu.RUnlock()
			return db, nil
		}
	}
	m.rootmu.RUnlock()

	// Slow path: need to reconnect, acquire write lock
	m.rootmu.Lock()
	defer m.rootmu.Unlock()

	// Double check after acquiring write lock
	if m.rootDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := m.rootDB.PingContext(ctx)
		cancel()
		if err == nil {
			return m.rootDB, nil
		}
		log.Println("[rdb] root connection lost, reconnecting...")
		m.rootDB.Close()
		m.rootDB = nil
	}

	for i := range 3 {
		db, err := sql.Open("postgres", m.rootDSN)
		if err != nil {
			return nil, fmt.Errorf("sql.Open failed: %w", err)
		}

		// Configure connection pool for root connection
		db.SetMaxOpenConns(20)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(10 * time.Minute)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := db.PingContext(ctx); err != nil {
			cancel()
			log.Printf("[rdb] root ping attempt %d/3 failed: %v", i+1, err)
			db.Close()
			continue
		}
		cancel()
		log.Printf("[rdb] root reconnected on attempt %d/3", i+1)
		m.rootDB = db
		return db, nil
	}
	return nil, fmt.Errorf("cockroachdb root unreachable after 3 attempts")
}

// tryGetUserDB returns a healthy user *sql.DB from pool, reconnecting if needed
func (m *RootRDBManager) tryGetUserDB(userUID string) (*sql.DB, *userRDB, error) {
	return m.userMgr.getOrCreateUserDB(userUID)
}

// Close closes the persistent admin connection
func (m *RootRDBManager) Close() error {
	m.rootmu.Lock()
	if m.rootDB != nil {
		m.rootDB.Close()
	}
	m.rootmu.Unlock()

	m.userMgr.closeAll()

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

func (r *userRDB) dsn() string {
	return fmt.Sprintf("postgresql://%s@%s:%s/%s?sslmode=disable",
		r.username(), CockroachDBHost, CockroachDBPort, r.database())
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

// CreateSchema creates a new schema in user's database
func (m *RootRDBManager) CreateSchema(userUID, schemaID string) error {
	db, r, err := m.tryGetUserDB(userUID)
	if err != nil {
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
	db, _, err := m.tryGetUserDB(userUID)
	if err != nil {
		return err
	}
	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	_, err = db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schName))
	return err
}

// ListSchemas lists all schemas in user's database
func (m *RootRDBManager) ListSchemas(userUID string) ([]string, error) {
	db, _, err := m.tryGetUserDB(userUID)
	if err != nil {
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
	db, _, err := m.tryGetUserDB(userUID)
	if err != nil {
		return false, err
	}
	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = $1`, schName).Scan(&count)
	return count > 0, err
}

// InitUserRDB creates user and database for new user
func (m *RootRDBManager) InitUserRDB(userUID string) error {
	db, err := m.tryGetRootDB()
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
	db, err := m.tryGetRootDB()
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
	db, err := m.tryGetRootDB()
	if err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", dbName))
	return err
}

// RootListUserDatabases 列出 CockroachDB 中所有 db_ 前缀的数据库名
func (m *RootRDBManager) RootListUserDatabases() ([]string, error) {
	db, err := m.tryGetRootDB()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT database_name FROM [SHOW DATABASES] WHERE database_name LIKE 'db_%'`)
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
		dbs = append(dbs, name)
	}
	return dbs, nil
}

const _DatabaseSizeSQL = `
SELECT 
    SUM((s."rowCount" * s."avgSize")::INT8) AS total_bytes
FROM system.table_statistics AS s
JOIN system.namespace AS n ON s."tableID" = n.id
JOIN system.namespace AS db ON n."parentID" = db.id
WHERE db.name = 'db_jabber147008'
  AND s."createdAt" = (SELECT MAX("createdAt") FROM system.table_statistics WHERE "tableID" = s."tableID");
`

const DatabaseSizeSQL = `
SELECT 
    SUM((s."rowCount" * s."avgSize")::INT8) AS total_bytes
FROM system.table_statistics AS s
JOIN system.namespace AS n ON s."tableID" = n.id
JOIN system.namespace AS db ON n."parentID" = db.id
WHERE db.name = $1
  AND s."createdAt" = (SELECT MAX("createdAt") FROM system.table_statistics WHERE "tableID" = s."tableID");
`

// DatabaseSize returns total size of user's database in bytes
func (m *RootRDBManager) DatabaseSize(userUID string) (int64, error) {
	db, err := m.tryGetRootDB()
	if err != nil {
		return 0, err
	}
	var size int64
	err = db.QueryRow(DatabaseSizeSQL, newUserRDB(userUID).database()).Scan(&size)
	return size, err
}

const _AllTableInDatabaseSQL = `
SELECT 
    db.name AS db_name,
	s."statisticID", 
    s."columnIDs",
    sc.name AS schema_name,
    n.name AS table_name,
    (s."rowCount" * s."avgSize")::INT8 AS table_bytes,
    s."rowCount"::INT8 AS est_rows,
    s."createdAt" AS last_updated
FROM 
    system.table_statistics AS s
JOIN 
    system.namespace AS n ON s."tableID" = n.id
JOIN 
    system.namespace AS sc ON n."parentSchemaID" = sc.id
JOIN 
    system.namespace AS db ON n."parentID" = db.id
WHERE 
    db.name = 'db_jabber147008'
ORDER BY 
    n.name ASC, last_updated DESC;
`

const _SchemaSizeSQL = `
SELECT 
    SUM((s."rowCount" * s."avgSize")::INT8) AS schema_bytes,
    MAX(s."createdAt") AS last_updated
FROM system.table_statistics AS s
JOIN system.namespace AS n ON s."tableID" = n.id
JOIN system.namespace AS sc ON n."parentSchemaID" = sc.id
JOIN system.namespace AS db ON n."parentID" = db.id
WHERE db.name = 'db_jabber147008' 
  AND sc.name = 'schema_303737e93eb57281'
  AND s."createdAt" = (SELECT MAX("createdAt") FROM system.table_statistics WHERE "tableID" = s."tableID");
`

const SchemaSizeSQL = `
SELECT 
    SUM((s."rowCount" * s."avgSize")::INT8) AS schema_bytes,
    MAX(s."createdAt") AS last_updated
FROM system.table_statistics AS s
JOIN system.namespace AS n ON s."tableID" = n.id
JOIN system.namespace AS sc ON n."parentSchemaID" = sc.id
JOIN system.namespace AS db ON n."parentID" = db.id
WHERE db.name = $1
  AND sc.name = $2
  AND s."createdAt" = (SELECT MAX("createdAt") FROM system.table_statistics WHERE "tableID" = s."tableID");
`

type schemaSizeResult struct {
	TableName  string
	TableID    int64
	TotalBytes int64
	EstRows    int64
}

// SchemaSize returns total size of a specific schema in bytes
func (m *RootRDBManager) SchemaSize(userUID, schemaID string) (int64, error) {
	db, err := m.tryGetRootDB()
	if err != nil {
		return 0, err
	}
	var size int64
	schName := fmt.Sprintf("schema_%s", sanitize(schemaID))
	err = db.QueryRow(SchemaSizeSQL, newUserRDB(userUID).database(), schName).Scan(&size)
	return size, err
}

const _ForceAnalyzeSQL = `
SELECT 
    sc.name AS schema_name,
    n.name AS table_name
FROM system.namespace AS n
JOIN system.namespace AS sc ON n."parentSchemaID" = sc.id
JOIN system.namespace AS db ON n."parentID" = db.id
WHERE db.name = 'db_jabber147008'
ORDER BY schema_name, table_name;
`

const ForceAnalyzeSQL = `
SELECT 
    sc.name AS schema_name,
    n.name AS table_name
FROM system.namespace AS n
JOIN system.namespace AS sc ON n."parentSchemaID" = sc.id
JOIN system.namespace AS db ON n."parentID" = db.id
WHERE db.name = $1
ORDER BY schema_name, table_name;
`

func (m *RootRDBManager) ForceAnalyze(userUID string) error {
	rootdb, err := m.tryGetRootDB()
	if err != nil {
		return err
	}

	var results []struct {
		SchemaName string
		TableName  string
	}
	// root只能查到表名，无法直接连接到用户数据库执行 ANALYZE，所以只能返回表列表让外部调用者逐个连接分析
	rows, err := rootdb.Query(ForceAnalyzeSQL, newUserRDB(userUID).database())
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r struct {
			SchemaName string
			TableName  string
		}
		if err := rows.Scan(&r.SchemaName, &r.TableName); err != nil {
			return err
		}
		results = append(results, r)
	}
	userdb, rdb, err := m.tryGetUserDB(userUID)
	if err != nil {
		return err
	}
	for _, r := range results {
		fullTableName := fmt.Sprintf("%s.%s.%s", rdb.database(), r.SchemaName, r.TableName)
		log.Printf("[rdb] running ANALYZE on %s", fullTableName)
		if _, err := userdb.Exec(fmt.Sprintf("ANALYZE %s", fullTableName)); err != nil {
			log.Printf("[rdb] ANALYZE failed on %s: %v", fullTableName, err)
			return err
		}
	}
	return nil
}
