package dblayer

// ========== Worker 基础操作 ==========

// CreateWorker 创建 worker 记录
func CreateWorker(userUID, workerID, workerName string) error {
	_, err := DB.Exec(
		`INSERT INTO workers (user_uid, worker_id, worker_name) VALUES ($1, $2, $3)`,
		userUID, workerID, workerName,
	)
	return err
}

// ListWorkersByUser 获取用户的所有 worker
func ListWorkersByUser(userUID string) ([]*Worker, error) {
	rows, err := DB.Query(
		`SELECT user_uid, worker_id, worker_name, status, active_version_id, env_json, secrets_json, created_at
		 FROM workers WHERE user_uid = $1 ORDER BY created_at DESC`, userUID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []*Worker
	for rows.Next() {
		var w Worker
		if err := rows.Scan(&w.UserUID, &w.WorkerID, &w.WorkerName, &w.Status, &w.ActiveVersionID, &w.EnvJSON, &w.SecretsJSON, &w.CreatedAt); err != nil {
			return nil, err
		}
		workers = append(workers, &w)
	}
	return workers, nil
}

// ========== DeployVersion 操作 ==========

// UpdateDeployVersionStatus 更新部署版本状态和消息
func UpdateDeployVersionStatus(versionID int, status, msg string) error {
	_, err := DB.Exec(
		`UPDATE worker_deploy_versions SET status = $1, msg = $2 WHERE id = $3`,
		status, msg, versionID,
	)
	return err
}

// ListDeployVersions 获取 worker 的部署版本，支持分页
func ListDeployVersions(workerID string, limit, offset int) ([]*WorkerDeployVersion, error) {
	rows, err := DB.Query(
		`SELECT id, worker_id, image, port, status, msg, created_at
		 FROM worker_deploy_versions WHERE worker_id = $1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		workerID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []*WorkerDeployVersion
	for rows.Next() {
		var v WorkerDeployVersion
		if err := rows.Scan(&v.ID, &v.WorkerID, &v.Image, &v.Port, &v.Status, &v.Msg, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, &v)
	}
	return versions, nil
}

// ========== Worker 组合查询 ==========

// GetWorkerByOwner 验证 worker 归属并返回，单次查询
func GetWorkerByOwner(workerID, userUID string) (*Worker, error) {
	var w Worker
	err := DB.QueryRow(
		`SELECT user_uid, worker_id, worker_name, status, active_version_id, env_json, secrets_json, created_at
		 FROM workers WHERE worker_id = $1 AND user_uid = $2`, workerID, userUID,
	).Scan(&w.UserUID, &w.WorkerID, &w.WorkerName, &w.Status, &w.ActiveVersionID, &w.EnvJSON, &w.SecretsJSON, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// GetWorkerEnvByOwner 验证归属并返回 env_json，单次查询
func GetWorkerEnvByOwner(workerID, userUID string) (string, error) {
	var envJSON string
	err := DB.QueryRow(
		`SELECT env_json FROM workers WHERE worker_id = $1 AND user_uid = $2`,
		workerID, userUID,
	).Scan(&envJSON)
	return envJSON, err
}

// SetWorkerEnvByOwner 验证归属并更新 env_json，单次操作
func SetWorkerEnvByOwner(workerID, userUID, envJSON string) error {
	res, err := DB.Exec(
		`UPDATE workers SET env_json = $1, status = 'loading' WHERE worker_id = $2 AND user_uid = $3`,
		envJSON, workerID, userUID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetWorkerSecretsByOwner 验证归属并返回 secrets_json，单次查询
func GetWorkerSecretsByOwner(workerID, userUID string) (string, error) {
	var secretsJSON string
	err := DB.QueryRow(
		`SELECT secrets_json FROM workers WHERE worker_id = $1 AND user_uid = $2`,
		workerID, userUID,
	).Scan(&secretsJSON)
	return secretsJSON, err
}

// SetWorkerSecretsByOwner 验证归属并更新 secrets_json，单次操作
func SetWorkerSecretsByOwner(workerID, userUID, secretsJSON string) error {
	res, err := DB.Exec(
		`UPDATE workers SET secrets_json = $1, status = 'loading' WHERE worker_id = $2 AND user_uid = $3`,
		secretsJSON, workerID, userUID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteWorkerByOwner 验证归属并删除 worker，单次操作
func DeleteWorkerByOwner(workerID, userUID string) error {
	res, err := DB.Exec(
		`DELETE FROM workers WHERE worker_id = $1 AND user_uid = $2`,
		workerID, userUID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateDeployVersionForOwner 验证 worker 归属后创建部署版本，返回 version id
func CreateDeployVersionForOwner(workerID, userUID, image string, port int) (int, error) {
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// 验证归属并设 status=loading
	res, err := tx.Exec(
		`UPDATE workers SET status = 'loading' WHERE worker_id = $1 AND user_uid = $2`,
		workerID, userUID,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, ErrNotFound
	}

	var id int
	err = tx.QueryRow(
		`INSERT INTO worker_deploy_versions (worker_id, image, port, status)
		 VALUES ($1, $2, $3, 'loading') RETURNING id`,
		workerID, image, port,
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, tx.Commit()
}

// GetDeployVersionWithWorker 获取部署版本及其关联的 worker，两表 JOIN 单次查询
func GetDeployVersionWithWorker(versionID int) (*WorkerDeployVersion, *Worker, string, error) {
	var v WorkerDeployVersion
	var w Worker
	var userSK string
	err := DB.QueryRow(
		`SELECT v.id, v.worker_id, v.image, v.port, v.status, v.msg, v.created_at, u.secret_key
		        w.user_uid, w.worker_id, w.worker_name, w.status, w.active_version_id, w.env_json, w.secrets_json, w.created_at
		 FROM worker_deploy_versions v
		 JOIN workers w ON w.worker_id = v.worker_id
		 JOIN users u ON u.uid = w.user_uid
		 WHERE v.id = $1`, versionID,
	).Scan(
		&v.ID, &v.WorkerID, &v.Image, &v.Port, &v.Status, &v.Msg, &v.CreatedAt, &userSK,
		&w.UserUID, &w.WorkerID, &w.WorkerName, &w.Status, &w.ActiveVersionID, &w.EnvJSON, &w.SecretsJSON, &w.CreatedAt,
	)
	if err != nil {
		return nil, nil, "", err
	}
	return &v, &w, userSK, nil
}

// DeployVersionSuccess 部署成功：更新 version status + 设置 active_version_id，单次事务
func DeployVersionSuccess(versionID int, workerID string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`UPDATE worker_deploy_versions SET status = 'success', msg = '' WHERE id = $1`,
		versionID,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		`UPDATE workers SET active_version_id = $1, status = 'active' WHERE worker_id = $2`,
		versionID, workerID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateWorkerStatus 更新 worker 状态
func UpdateWorkerStatus(workerID, status string) error {
	_, err := DB.Exec(
		`UPDATE workers SET status = $1 WHERE worker_id = $2`,
		status, workerID,
	)
	return err
}
