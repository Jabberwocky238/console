package dblayer

import (
	"fmt"
	"time"
)

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

// ListUserUIDsPaged 分页获取所有用户 UID
func ListUserUIDsPaged(limit, offset int) ([]string, error) {
	rows, err := DB.Query(
		`SELECT uid FROM users ORDER BY uid LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uids []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		uids = append(uids, uid)
	}
	return uids, nil
}

// ========== CustomDomain Actions ==========

// CreateCustomDomain 创建自定义域名
func CreateCustomDomain(cdid, userUID, domain, target, txtName, txtValue, status string) error {
	_, err := DB.Exec(
		`INSERT INTO custom_domains (cdid, user_uid, domain, target, txt_name, txt_value, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		cdid, userUID, domain, target, txtName, txtValue, status,
	)
	return err
}

// GetCustomDomain 获取自定义域名
func GetCustomDomain(cdid string) (*CustomDomain, error) {
	var cd CustomDomain
	err := DB.QueryRow(
		`SELECT id, cdid, user_uid, domain, target, txt_name, txt_value, status, created_at
		 FROM custom_domains WHERE cdid = $1`,
		cdid,
	).Scan(&cd.ID, &cd.CDID, &cd.UserUID, &cd.Domain, &cd.Target, &cd.TXTName, &cd.TXTValue, &cd.Status, &cd.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &cd, nil
}

// ListCustomDomains 获取用户的所有自定义域名
func ListCustomDomains(userUID string) ([]*CustomDomain, error) {
	rows, err := DB.Query(
		`SELECT id, cdid, user_uid, domain, target, txt_name, txt_value, status, created_at
		 FROM custom_domains WHERE user_uid = $1`,
		userUID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []*CustomDomain
	for rows.Next() {
		var cd CustomDomain
		if err := rows.Scan(&cd.ID, &cd.CDID, &cd.UserUID, &cd.Domain, &cd.Target, &cd.TXTName, &cd.TXTValue, &cd.Status, &cd.CreatedAt); err != nil {
			return nil, err
		}
		domains = append(domains, &cd)
	}
	return domains, nil
}

// UpdateCustomDomainStatus 更新自定义域名状态
func UpdateCustomDomainStatus(cdid, status string) error {
	_, err := DB.Exec(
		`UPDATE custom_domains SET status = $1 WHERE cdid = $2`,
		status, cdid,
	)
	return err
}

// DeleteCustomDomain 删除自定义域名
func DeleteCustomDomain(cdid string) error {
	_, err := DB.Exec(`DELETE FROM custom_domains WHERE cdid = $1`, cdid)
	return err
}

// ListAllSuccessDomains 获取所有成功状态的域名（用于定期检查）
func ListAllSuccessDomains() ([]*CustomDomain, error) {
	rows, err := DB.Query(
		`SELECT id, cdid, user_uid, domain, target, txt_name, txt_value, status, created_at
		 FROM custom_domains WHERE status = 'success'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []*CustomDomain
	for rows.Next() {
		var cd CustomDomain
		if err := rows.Scan(&cd.ID, &cd.CDID, &cd.UserUID, &cd.Domain, &cd.Target, &cd.TXTName, &cd.TXTValue, &cd.Status, &cd.CreatedAt); err != nil {
			return nil, err
		}
		domains = append(domains, &cd)
	}
	return domains, nil
}

// ========== CombinatorResource Actions ==========

// CreateCombinatorResource 创建 combinator 资源记录
func CreateCombinatorResource(userUID, resourceType, resourceID string) error {
	var newID int
	err := DB.QueryRow(
		`INSERT INTO combinator_resources (user_uid, resource_type, resource_id)
		 VALUES ($1, $2, $3) RETURNING id`,
		userUID, resourceType, resourceID,
	).Scan(&newID)
	return err
}

// GetCombinatorResource 获取单个资源
func GetCombinatorResource(userUID, resourceType, resourceID string) (*CombinatorResource, error) {
	var cr CombinatorResource
	err := DB.QueryRow(
		`SELECT id, user_uid, resource_type, resource_id, status, msg, created_at
		 FROM combinator_resources WHERE user_uid = $1 AND resource_type = $2 AND resource_id = $3`,
		userUID, resourceType, resourceID,
	).Scan(&cr.ID, &cr.UserUID, &cr.ResourceType, &cr.ResourceID, &cr.Status, &cr.Msg, &cr.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &cr, nil
}

// ListCombinatorResources 获取用户某类型的所有资源
func ListCombinatorResources(userUID, resourceType string) ([]*CombinatorResource, error) {
	rows, err := DB.Query(
		`SELECT id, user_uid, resource_type, resource_id, status, msg, created_at
		 FROM combinator_resources WHERE user_uid = $1 AND resource_type = $2`,
		userUID, resourceType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var resources []*CombinatorResource
	for rows.Next() {
		var cr CombinatorResource
		if err := rows.Scan(&cr.ID, &cr.UserUID, &cr.ResourceType, &cr.ResourceID, &cr.Status, &cr.Msg, &cr.CreatedAt); err != nil {
			return nil, err
		}
		resources = append(resources, &cr)
	}
	return resources, nil
}

// ListActiveCombinatorResources 获取用户所有 active 状态的资源
func ListActiveCombinatorResources(userUID string) ([]*CombinatorResource, error) {
	rows, err := DB.Query(
		`SELECT id, user_uid, resource_type, resource_id, status, msg, created_at
		 FROM combinator_resources WHERE user_uid = $1 AND status = 'active'`,
		userUID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var resources []*CombinatorResource
	for rows.Next() {
		var cr CombinatorResource
		if err := rows.Scan(&cr.ID, &cr.UserUID, &cr.ResourceType, &cr.ResourceID, &cr.Status, &cr.Msg, &cr.CreatedAt); err != nil {
			return nil, err
		}
		resources = append(resources, &cr)
	}
	return resources, nil
}

// UpdateCombinatorResourceStatus 更新资源状态
func UpdateCombinatorResourceStatus(userUID, resourceType, resourceID, status, msg string) error {
	_, err := DB.Exec(
		`UPDATE combinator_resources SET status = $1, msg = $2
		 WHERE user_uid = $3 AND resource_type = $4 AND resource_id = $5`,
		status, msg, userUID, resourceType, resourceID,
	)
	return err
}

// DeleteCombinatorResource 删除资源记录
func DeleteCombinatorResource(userUID, resourceType, resourceID string) error {
	_, err := DB.Exec(
		`DELETE FROM combinator_resources WHERE user_uid = $1 AND resource_type = $2 AND resource_id = $3`,
		userUID, resourceType, resourceID,
	)
	return err
}

func SaveCombinatorResourceReport(report *CombinatorResourceReport) error {
	_, err := DB.Exec(
		`INSERT INTO combinator_resource_reports (
		user_uid, resource_id, datachange, record_start, record_end)
		 VALUES ($1, $2, $3, $4, $5)`,
		report.UserID, report.ResourceID, report.DataChange,
		report.TimespanStart, report.TimespanEnd,
	)
	return err
}

func BatchSaveCombinatorResourceReports(reports []CombinatorResourceReport) error {
	if len(reports) == 0 {
		return nil
	}

	// Build batch insert query
	query := `INSERT INTO combinator_resource_reports (user_uid, resource_id, datachange, record_start, record_end) VALUES `
	values := []interface{}{}

	for i, report := range reports {
		if i > 0 {
			query += ", "
		}
		paramOffset := i * 5
		query += fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", paramOffset+1, paramOffset+2, paramOffset+3, paramOffset+4, paramOffset+5)
		values = append(values, report.UserID, report.ResourceID, report.DataChange, report.TimespanStart, report.TimespanEnd)
	}

	_, err := DB.Exec(query, values...)
	return err
}

func CalculateDataChangeSum(userID, resourceType string, resourceID string) (int, error) {
	var sum int
	err := DB.QueryRow(
		`SELECT COALESCE(SUM(datachange), 0) FROM combinator_resource_reports
		 WHERE user_id = $1 AND resource_type = $2 AND resource_id = $3`,
		userID, resourceType, resourceID,
	).Scan(&sum)
	return sum, err
}
