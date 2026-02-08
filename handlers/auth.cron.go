package handlers

import (
	"context"
	"fmt"
	"log"

	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/k8s"
	"jabberwocky238/console/k8s/controller"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const userPageSize = 1000

// UserAuditJob 定期审计：检查用户初始化状态 + 清理孤儿服务
type UserAuditJob struct{}

func (j *UserAuditJob) Type() string { return "auth.user_audit" }
func (j *UserAuditJob) ID() string   { return "periodic" }

func (j *UserAuditJob) Do() error {
	if k8s.K8sClient == nil || k8s.DynamicClient == nil {
		log.Println("[audit] k8s client not initialized, skip")
		return nil
	}

	// 1. 分页扫描所有用户 UID，构建 set
	userSet, err := loadAllUserUIDs()
	if err != nil {
		return err
	}
	log.Printf("[audit] loaded %d users from database", len(userSet))

	ctx := context.Background()

	// 2. 拉取所有 CR 列表（失败则中止，避免误判空列表导致跳过清理）
	combinatorCRs, err := k8s.DynamicClient.Resource(controller.CombinatorAppGVR).
		Namespace(k8s.CombinatorNamespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list combinator CRs: %w", err)
	}
	log.Printf("[audit] found %d combinator CRs", len(combinatorCRs.Items))

	workerCRs, err := k8s.DynamicClient.Resource(controller.WorkerAppGVR).
		Namespace(k8s.WorkerNamespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list worker CRs: %w", err)
	}
	log.Printf("[audit] found %d worker CRs", len(workerCRs.Items))

	// 3. 一次性拉取所有 db_ 数据库列表
	var existingDBs []string
	if k8s.RDBManager != nil {
		existingDBs, err = k8s.RDBManager.RootListUserDatabases()
		if err != nil {
			log.Printf("[audit] list cockroachdb databases failed: %v", err)
			existingDBs = nil
		}
	}

	// 4. 检查用户是否已初始化，未初始化则补建
	checkUserInitialization(userSet, combinatorCRs)
	if existingDBs != nil {
		checkUserRDBInitialization(userSet, existingDBs)
	}

	// 5. 清理孤儿 CR（删 CR → controller onDelete 级联清理子资源）
	cleanOrphanWorkers(userSet, workerCRs)
	cleanOrphanCombinators(userSet, combinatorCRs)
	if existingDBs != nil {
		cleanOrphanRDBs(userSet, existingDBs)
	}

	log.Println("[audit] user audit completed")
	return nil
}

// loadAllUserUIDs 分页加载所有用户 UID，返回 set
func loadAllUserUIDs() (map[string]struct{}, error) {
	userSet := make(map[string]struct{})
	for offset := 0; ; offset += userPageSize {
		uids, err := dblayer.ListUserUIDsPaged(userPageSize, offset)
		if err != nil {
			return nil, err
		}
		for _, uid := range uids {
			userSet[uid] = struct{}{}
		}
		if len(uids) < userPageSize {
			break
		}
	}
	return userSet, nil
}

// checkUserInitialization 检查每个用户是否有 CombinatorApp CR，没有则补建
func checkUserInitialization(userSet map[string]struct{}, crList *unstructured.UnstructuredList) {
	initialized := make(map[string]struct{})
	for _, item := range crList.Items {
		spec, _ := item.Object["spec"].(map[string]interface{})
		if spec == nil {
			continue
		}
		if ownerID, ok := spec["ownerID"].(string); ok {
			initialized[ownerID] = struct{}{}
		}
	}

	for uid := range userSet {
		if _, ok := initialized[uid]; ok {
			continue
		}
		log.Printf("[audit] user %s not initialized, creating CombinatorApp CR", uid)
		config := controller.EmptyCombinatorConfig()
		if err := controller.CreateCombinatorAppCR(k8s.DynamicClient, uid, config); err != nil {
			log.Printf("[audit] create CombinatorApp CR for %s failed: %v", uid, err)
		}
	}
}

// cleanOrphanWorkers 删除 owner 不存在的 WorkerApp CR
func cleanOrphanWorkers(userSet map[string]struct{}, crList *unstructured.UnstructuredList) {
	for _, item := range crList.Items {
		spec, _ := item.Object["spec"].(map[string]interface{})
		if spec == nil {
			continue
		}
		ownerID, _ := spec["ownerID"].(string)
		if ownerID == "" {
			continue
		}
		if _, ok := userSet[ownerID]; ok {
			continue
		}
		name := item.GetName()
		log.Printf("[audit] orphan worker CR %s (owner %s), deleting", name, ownerID)
		if err := controller.DeleteWorkerAppCR(k8s.DynamicClient, name); err != nil {
			log.Printf("[audit] delete worker CR %s failed: %v", name, err)
		}
	}
}

// cleanOrphanCombinators 删除 owner 不存在的 CombinatorApp CR
func cleanOrphanCombinators(userSet map[string]struct{}, crList *unstructured.UnstructuredList) {
	for _, item := range crList.Items {
		spec, _ := item.Object["spec"].(map[string]interface{})
		if spec == nil {
			continue
		}
		ownerID, _ := spec["ownerID"].(string)
		if ownerID == "" {
			continue
		}
		if _, ok := userSet[ownerID]; ok {
			continue
		}
		log.Printf("[audit] orphan combinator CR (owner %s), deleting", ownerID)
		if err := controller.DeleteCombinatorAppCR(k8s.DynamicClient, ownerID); err != nil {
			log.Printf("[audit] delete combinator CR for %s failed: %v", ownerID, err)
		}
	}
}

// checkUserRDBInitialization 检查每个用户是否有 CockroachDB database，没有则补建
func checkUserRDBInitialization(userSet map[string]struct{}, existingDBs []string) {
	dbSet := make(map[string]struct{}, len(existingDBs))
	for _, db := range existingDBs {
		dbSet[db] = struct{}{}
	}

	for uid := range userSet {
		if _, ok := dbSet[k8s.RDBManager.DatabaseName(uid)]; ok {
			continue
		}
		log.Printf("[audit] user %s missing RDB, initializing", uid)
		if err := k8s.RDBManager.InitUserRDB(uid); err != nil {
			log.Printf("[audit] init RDB for %s failed: %v", uid, err)
		}
	}
}

// cleanOrphanRDBs 删除 owner 不存在的 db_ 数据库
func cleanOrphanRDBs(userSet map[string]struct{}, existingDBs []string) {
	// 正向构建：所有合法用户对应的 db 名
	validDBs := make(map[string]struct{}, len(userSet))
	for uid := range userSet {
		validDBs[k8s.RDBManager.DatabaseName(uid)] = struct{}{}
	}

	for _, dbName := range existingDBs {
		if _, ok := validDBs[dbName]; ok {
			continue
		}
		log.Printf("[audit] orphan database %s, dropping", dbName)
		// 反解 uid 不可靠，直接用 admin 连接 DROP
		if err := k8s.RDBManager.DropDatabase(dbName); err != nil {
			log.Printf("[audit] drop database %s failed: %v", dbName, err)
		}
	}
}
