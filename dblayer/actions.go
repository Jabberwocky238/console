package dblayer

import "time"

// ========== User Actions ==========

// GetVerificationCode 获取验证码
func GetVerificationCode(email, code string) (int, time.Time, error) {
	var codeID int
	var expiresAt time.Time
	err := DB.QueryRow(
		"SELECT id, expires_at FROM verification_codes WHERE email = $1 AND code = $2 AND used = false",
		email, code,
	).Scan(&codeID, &expiresAt)
	return codeID, expiresAt, err
}

// MarkCodeUsed 标记验证码已使用
func MarkCodeUsed(codeID int) error {
	_, err := DB.Exec("UPDATE verification_codes SET used = true WHERE id = $1", codeID)
	return err
}

// CreateUser 创建用户
func CreateUser(uid, email, passwordHash, secretKey string) (string, error) {
	var userUID string
	err := DB.QueryRow(
		"INSERT INTO users (uid, email, password_hash, secret_key) VALUES ($1, $2, $3, $4) RETURNING uid",
		uid, email, passwordHash, secretKey,
	).Scan(&userUID)
	return userUID, err
}

// GetUserByEmail 通过邮箱获取用户
func GetUserByEmail(email string) (*User, error) {
	var user User
	err := DB.QueryRow(
		"SELECT uid, email, password_hash, secret_key FROM users WHERE email = $1",
		email,
	).Scan(&user.UID, &user.Email, &user.PasswordHash, &user.SecretKey)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// SaveVerificationCode 保存验证码
func SaveVerificationCode(email, code string, expiresAt time.Time) error {
	_, err := DB.Exec(
		"INSERT INTO verification_codes (email, code, expires_at) VALUES ($1, $2, $3)",
		email, code, expiresAt,
	)
	return err
}

// UpdateUserPassword 更新用户密码
func UpdateUserPassword(email, passwordHash string) error {
	_, err := DB.Exec(
		"UPDATE users SET password_hash = $1 WHERE email = $2",
		passwordHash, email,
	)
	return err
}

// GetUserSecretKey 通过 UID 获取用户密钥
func GetUserSecretKey(uid string) (string, error) {
	var secretKey string
	err := DB.QueryRow(
		"SELECT secret_key FROM users WHERE uid = $1",
		uid,
	).Scan(&secretKey)
	return secretKey, err
}

// ========== RDB Actions ==========

// CreateRDB 创建 RDB 资源
func CreateRDB(userUID, rdbUID, name, rdbType, url string) (string, error) {
	var uid string
	err := DB.QueryRow(
		`INSERT INTO user_rdbs (user_id, uid, name, rdb_type, url)
		 VALUES ((SELECT id FROM users WHERE uid = $1), $2, $3, $4, $5)
		 RETURNING uid`,
		userUID, rdbUID, name, rdbType, url,
	).Scan(&uid)
	return uid, err
}

// ListRDBsByUser 获取用户的所有 RDB
func ListRDBsByUser(userUID string) ([]RDB, error) {
	rows, err := DB.Query(
		`SELECT uid, name, rdb_type, url, enabled, status, COALESCE(error_msg, '') FROM user_rdbs
		 WHERE user_id = (SELECT id FROM users WHERE uid = $1)`,
		userUID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rdbs []RDB
	for rows.Next() {
		var r RDB
		rows.Scan(&r.UID, &r.Name, &r.Type, &r.URL, &r.Enabled, &r.Status, &r.ErrorMsg)
		rdbs = append(rdbs, r)
	}
	return rdbs, nil
}

// DeleteRDB 删除 RDB 资源
func DeleteRDB(rdbUID, userUID string) (int64, error) {
	result, err := DB.Exec(
		`DELETE FROM user_rdbs
		 WHERE uid = $1 AND user_id = (SELECT id FROM users WHERE uid = $2)`,
		rdbUID, userUID,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ========== KV Actions ==========

// CreateKV 创建 KV 资源
func CreateKV(userUID, kvUID, name, kvType, url string) (string, error) {
	var uid string
	err := DB.QueryRow(
		`INSERT INTO user_kvs (user_id, uid, name, kv_type, url)
		 VALUES ((SELECT id FROM users WHERE uid = $1), $2, $3, $4, $5)
		 RETURNING uid`,
		userUID, kvUID, name, kvType, url,
	).Scan(&uid)
	return uid, err
}

// ListKVsByUser 获取用户的所有 KV
func ListKVsByUser(userUID string) ([]KV, error) {
	rows, err := DB.Query(
		`SELECT uid, name, kv_type, url, enabled, status, COALESCE(error_msg, '') FROM user_kvs
		 WHERE user_id = (SELECT id FROM users WHERE uid = $1)`,
		userUID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var kvs []KV
	for rows.Next() {
		var k KV
		rows.Scan(&k.UID, &k.Name, &k.Type, &k.URL, &k.Enabled, &k.Status, &k.ErrorMsg)
		kvs = append(kvs, k)
	}
	return kvs, nil
}

// DeleteKV 删除 KV 资源
func DeleteKV(kvUID, userUID string) (int64, error) {
	result, err := DB.Exec(
		`DELETE FROM user_kvs
		 WHERE uid = $1 AND user_id = (SELECT id FROM users WHERE uid = $2)`,
		kvUID, userUID,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SetRDBStatus 设置 RDB 状态
func SetRDBStatus(rdbUID, status, errorMsg string) error {
	_, err := DB.Exec(
		`UPDATE user_rdbs SET status = $1, error_msg = $2 WHERE uid = $3`,
		status, errorMsg, rdbUID,
	)
	return err
}

// SetKVStatus 设置 KV 状态
func SetKVStatus(kvUID, status, errorMsg string) error {
	_, err := DB.Exec(
		`UPDATE user_kvs SET status = $1, error_msg = $2 WHERE uid = $3`,
		status, errorMsg, kvUID,
	)
	return err
}

// ========== Config Generation ==========

// RDBConfigItem RDB 配置项
type RDBConfigItem struct {
	UID  string
	Type string
	URL  string
}

// KVConfigItem KV 配置项
type KVConfigItem struct {
	UID  string
	Type string
	URL  string
}

// GetUserRDBsForConfig 获取用户启用的 RDB 配置
func GetUserRDBsForConfig(userUID string) ([]RDBConfigItem, error) {
	rows, err := DB.Query(
		`SELECT uid, rdb_type, url FROM user_rdbs
		 WHERE user_id = (SELECT id FROM users WHERE uid = $1) AND enabled = true`,
		userUID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []RDBConfigItem
	for rows.Next() {
		var item RDBConfigItem
		rows.Scan(&item.UID, &item.Type, &item.URL)
		items = append(items, item)
	}
	return items, nil
}

// GetUserKVsForConfig 获取用户启用的 KV 配置
func GetUserKVsForConfig(userUID string) ([]KVConfigItem, error) {
	rows, err := DB.Query(
		`SELECT uid, kv_type, url FROM user_kvs
		 WHERE user_id = (SELECT id FROM users WHERE uid = $1) AND enabled = true`,
		userUID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []KVConfigItem
	for rows.Next() {
		var item KVConfigItem
		rows.Scan(&item.UID, &item.Type, &item.URL)
		items = append(items, item)
	}
	return items, nil
}

// ========== Worker Actions ==========

// CreateWorker 创建 Worker
func CreateWorker(workerID, ownerID, image string, port int) error {
	_, err := DB.Exec(`
		INSERT INTO workers (worker_id, owner_id, image, port)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (worker_id, owner_id) DO UPDATE SET image = $3, port = $4, updated_at = NOW()
	`, workerID, ownerID, image, port)
	return err
}

// GetWorker 获取 Worker
func GetWorker(workerID, ownerID string) (*Worker, error) {
	var w Worker
	err := DB.QueryRow(`
		SELECT worker_id, owner_id, image, port, status, COALESCE(error_msg, '')
		FROM workers WHERE worker_id = $1 AND owner_id = $2
	`, workerID, ownerID).Scan(&w.WorkerID, &w.OwnerID, &w.Image, &w.Port, &w.Status, &w.ErrorMsg)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// ListWorkersByOwner 获取用户的所有 Worker
func ListWorkersByOwner(ownerID string) ([]Worker, error) {
	rows, err := DB.Query(`
		SELECT worker_id, owner_id, image, port, status, COALESCE(error_msg, '')
		FROM workers WHERE owner_id = $1
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []Worker
	for rows.Next() {
		var w Worker
		rows.Scan(&w.WorkerID, &w.OwnerID, &w.Image, &w.Port, &w.Status, &w.ErrorMsg)
		workers = append(workers, w)
	}
	return workers, nil
}

// DeleteWorker 删除 Worker
func DeleteWorker(workerID, ownerID string) error {
	_, err := DB.Exec(`DELETE FROM workers WHERE worker_id = $1 AND owner_id = $2`, workerID, ownerID)
	return err
}

// SetWorkerStatus 设置 Worker 状态
func SetWorkerStatus(workerID, ownerID, status, errorMsg string) error {
	_, err := DB.Exec(`
		UPDATE workers SET status = $1, error_msg = $2, updated_at = NOW()
		WHERE worker_id = $3 AND owner_id = $4
	`, status, errorMsg, workerID, ownerID)
	return err
}
