package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/k8s"
	"jabberwocky238/console/k8s/controller"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func workerURL(workerID, userUID string) string {
	return fmt.Sprintf("https://%s.worker.%s", controller.WorkerName(workerID, userUID), k8s.Domain)
}

type workerTask struct {
	kind      string // "deploy", "sync_env", "sync_secret", "delete_cr"
	versionID int
	workerID  string
	userUID   string
	data      map[string]string
}

type WorkerHandler struct {
	queue chan workerTask
}

func NewWorkerHandler() *WorkerHandler {
	return &WorkerHandler{
		queue: make(chan workerTask, 100),
	}
}

func (h *WorkerHandler) Start() {
	go func() {
		for task := range h.queue {
			h.process(task)
		}
	}()
	log.Println("[worker-handler] started")
}

// CreateWorker 创建 worker 记录
func (h *WorkerHandler) CreateWorker(c *gin.Context) {
	userUID := c.GetString("user_id")

	var req struct {
		WorkerName string `json:"worker_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	workerID := uuid.New().String()[:8]

	if err := dblayer.CreateWorker(userUID, workerID, req.WorkerName); err != nil {
		c.JSON(500, gin.H{"error": "failed to create worker"})
		return
	}

	c.JSON(200, gin.H{
		"worker_id":   workerID,
		"worker_name": req.WorkerName,
	})
}

// DeleteWorker 删除 worker（库 + K8s 资源）
func (h *WorkerHandler) DeleteWorker(c *gin.Context) {
	userUID := c.GetString("user_id")
	workerID := c.Param("id")

	// 异步删 CR（可能不存在）
	h.queue <- workerTask{kind: "delete_cr", workerID: workerID, userUID: userUID}

	// 单次操作：验证归属 + 删除
	if err := dblayer.DeleteWorkerByOwner(workerID, userUID); err != nil {
		if err == dblayer.ErrNotFound {
			c.JSON(404, gin.H{"error": "worker not found"})
		} else {
			c.JSON(500, gin.H{"error": "failed to delete worker"})
		}
		return
	}

	c.JSON(200, gin.H{"message": "worker deleted"})
}

// ListWorkers 列出用户所有 worker
func (h *WorkerHandler) ListWorkers(c *gin.Context) {
	userUID := c.GetString("user_id")

	workers, err := dblayer.ListWorkersByUser(userUID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to list workers"})
		return
	}

	result := make([]gin.H, len(workers))
	for i, w := range workers {
		result[i] = gin.H{
			"worker_id":         w.WorkerID,
			"worker_name":       w.WorkerName,
			"status":            w.Status,
			"active_version_id": w.ActiveVersionID,
			"url":               workerURL(w.WorkerID, w.UserUID),
		}
	}
	c.JSON(200, result)
}

// GetWorker 获取单个 worker 详情，附带最近10条 version
func (h *WorkerHandler) GetWorker(c *gin.Context) {
	userUID := c.GetString("user_id")
	workerID := c.Param("id")

	w, err := dblayer.GetWorkerByOwner(workerID, userUID)
	if err != nil {
		c.JSON(404, gin.H{"error": "worker not found"})
		return
	}

	offset := 0
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	versions, err := dblayer.ListDeployVersions(workerID, 10, offset)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to list versions"})
		return
	}

	c.JSON(200, gin.H{
		"worker":   w,
		"versions": versions,
		"url":      workerURL(w.WorkerID, w.UserUID),
	})
}

// DeployWorker 触发 worker 部署，立刻返回 200，异步执行
func (h *WorkerHandler) DeployWorker(c *gin.Context) {
	var req struct {
		UserUID  string `json:"user_uid" binding:"required"`
		WorkerID string `json:"worker_id" binding:"required"`
		Image    string `json:"image" binding:"required"`
		Port     int    `json:"port" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// 单次操作：验证归属 + 创建部署版本
	versionID, err := dblayer.CreateDeployVersionForOwner(req.WorkerID, req.UserUID, req.Image, req.Port)
	if err != nil {
		if err == dblayer.ErrNotFound {
			c.JSON(404, gin.H{"error": "worker not found"})
		} else {
			c.JSON(500, gin.H{"error": "failed to create deploy version"})
		}
		return
	}

	h.queue <- workerTask{kind: "deploy", versionID: versionID}

	c.JSON(200, gin.H{
		"worker_id":  req.WorkerID,
		"version_id": versionID,
		"status":     "loading",
	})
}

// GetWorkerEnv 获取 worker 环境变量
func (h *WorkerHandler) GetWorkerEnv(c *gin.Context) {
	userUID := c.GetString("user_id")
	workerID := c.Param("id")

	// 单次查询：验证归属 + 获取 env_json
	envJSON, err := dblayer.GetWorkerEnvByOwner(workerID, userUID)
	if err != nil {
		c.JSON(404, gin.H{"error": "worker not found"})
		return
	}

	var envMap map[string]string
	json.Unmarshal([]byte(envJSON), &envMap)
	c.JSON(200, envMap)
}

// SetWorkerEnv 设置单条 worker 环境变量（merge 到现有 env）
func (h *WorkerHandler) SetWorkerEnv(c *gin.Context) {
	userUID := c.GetString("user_id")
	workerID := c.Param("id")

	var req struct {
		Key    string `json:"key" binding:"required"`
		Value  string `json:"value"`
		Delete bool   `json:"delete"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// 读取现有 env
	envJSON, err := dblayer.GetWorkerEnvByOwner(workerID, userUID)
	if err != nil {
		c.JSON(404, gin.H{"error": "worker not found"})
		return
	}
	var envMap map[string]string
	json.Unmarshal([]byte(envJSON), &envMap)
	if envMap == nil {
		envMap = map[string]string{}
	}

	// merge
	if req.Delete {
		delete(envMap, req.Key)
	} else {
		envMap[req.Key] = req.Value
	}

	data, _ := json.Marshal(envMap)
	if err := dblayer.SetWorkerEnvByOwner(workerID, userUID, string(data)); err != nil {
		c.JSON(500, gin.H{"error": "failed to set env"})
		return
	}

	h.queue <- workerTask{kind: "sync_env", workerID: workerID, userUID: userUID, data: envMap}

	c.JSON(200, envMap)
}

// GetWorkerSecrets 获取 worker secrets
func (h *WorkerHandler) GetWorkerSecrets(c *gin.Context) {
	userUID := c.GetString("user_id")
	workerID := c.Param("id")

	// 单次查询：验证归属 + 获取 secrets_json
	secretsJSON, err := dblayer.GetWorkerSecretsByOwner(workerID, userUID)
	if err != nil {
		c.JSON(404, gin.H{"error": "worker not found"})
		return
	}

	var secrets []string
	json.Unmarshal([]byte(secretsJSON), &secrets)
	c.JSON(200, secrets)
}

// SetWorkerSecrets 设置/删除单条 worker secret
func (h *WorkerHandler) SetWorkerSecrets(c *gin.Context) {
	userUID := c.GetString("user_id")
	workerID := c.Param("id")

	var req struct {
		Key    string `json:"key" binding:"required"`
		Value  string `json:"value"`
		Delete bool   `json:"delete"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// 读取现有 secrets key 列表
	secretsJSON, err := dblayer.GetWorkerSecretsByOwner(workerID, userUID)
	if err != nil {
		c.JSON(404, gin.H{"error": "worker not found"})
		return
	}
	var keys []string
	json.Unmarshal([]byte(secretsJSON), &keys)

	if req.Delete {
		// 删除 key
		filtered := keys[:0]
		for _, k := range keys {
			if k != req.Key {
				filtered = append(filtered, k)
			}
		}
		keys = filtered
	} else {
		// 去重追加
		found := false
		for _, k := range keys {
			if k == req.Key {
				found = true
				break
			}
		}
		if !found {
			keys = append(keys, req.Key)
		}
	}

	keysData, _ := json.Marshal(keys)
	if err := dblayer.SetWorkerSecretsByOwner(workerID, userUID, string(keysData)); err != nil {
		c.JSON(500, gin.H{"error": "failed to set secrets"})
		return
	}

	h.queue <- workerTask{
		kind: "sync_secret", workerID: workerID, userUID: userUID,
		data: map[string]string{req.Key: req.Value},
	}

	c.JSON(200, keys)
}
