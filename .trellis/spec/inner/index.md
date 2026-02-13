# Inner Gateway Development Guidelines

> **Purpose**: Standards for developing the Inner Gateway (internal API + task processor)

---

## Overview

The **Inner Gateway** (`cmd/inner/main.go`) is the internal processing service.

**Port**: 9901
**Access**: Internal cluster services only

---

## 🎯 Core Principle: Inner Gateway Responsibilities

**Inner Gateway ONLY does:**
1. ✅ **Receive tasks/events** from Outer Gateway
2. ✅ **Process background jobs** (async operations)
3. ✅ **Modify K8s cluster** (create/update/delete resources)
4. ✅ **Update database status** (after processing)
5. ✅ **Run cron jobs** (periodic tasks)
6. ✅ **Operate K8s controller** (watch CRDs, reconcile)

**Inner Gateway NEVER does:**
- ❌ Serve public API
- ❌ Handle user authentication
- ❌ Serve frontend
- ❌ Accept direct user requests

> **Rule**: Inner receives events, processes them, and updates cluster + database.

---

## Architecture Pattern

```
Outer Gateway
    ↓ (sends task)
Inner Gateway
    ↓
1. Receive task from /api/acceptTask
2. Enqueue to Processor
3. Worker picks up task
4. Execute job (create K8s resources, etc.)
5. Update database status ("active" or "error")
```

---

## Key Responsibilities

### 1. Task Processing

**What Inner Does**:
- Receive tasks from Outer Gateway
- Enqueue tasks to Processor
- Execute jobs in worker pool
- Update database status after completion

**Pattern**:
```go
// Inner: Accept task endpoint
func (h *TaskHandler) AcceptTask(c *gin.Context) {
    var req struct {
        TaskType string          `json:"task_type" binding:"required"`
        TaskInfo json.RawMessage `json:"task_info" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // Create task record in database
    taskID, err := dblayer.CreateConsoleTask(req.TaskType, "pending", "", string(req.TaskInfo))
    if err != nil {
        c.JSON(500, gin.H{"error": "failed to create task"})
        return
    }

    // Enqueue to processor
    job := jobs.ParseJob(req.TaskType, req.TaskInfo)
    h.processor.Submit(job)

    // Return immediately
    c.JSON(200, gin.H{"task_id": taskID})
}
```

### 2. Worker Deployment

**What Inner Does**:
- Receive deployment task from Outer
- Get worker and version from database
- Create/update WorkerApp CR in K8s
- Update database status after deployment

**Pattern**:
```go
type DeployWorkerJob struct {
    WorkerID  string
    UserUID   string
    VersionID int
}

func (j *DeployWorkerJob) Execute() error {
    // 1. Get data from database (read-only)
    worker, err := dblayer.GetWorkerByOwner(j.WorkerID, j.UserUID)
    if err != nil {
        return err
    }

    version, err := dblayer.GetDeployVersion(j.VersionID)
    if err != nil {
        return err
    }

    // 2. Create/update K8s resources
    err = controller.CreateWorkerAppCR(
        k8s.DynamicClient,
        controller.WorkerName(j.WorkerID, j.UserUID),
        j.WorkerID,
        j.UserUID,
        version.Image,
        worker.SecretKey,
        version.Port,
    )

    // 3. Update database status
    if err != nil {
        dblayer.UpdateDeployVersionStatus(j.VersionID, "error", err.Error())
        dblayer.UpdateWorkerStatus(worker.ID, "error")
        return err
    }

    dblayer.UpdateDeployVersionStatus(j.VersionID, "success", "")
    dblayer.UpdateWorkerStatus(worker.ID, "active")
    dblayer.SetActiveVersion(worker.ID, j.VersionID)

    return nil
}
```

### 3. Combinator Resource Management

**What Inner Does**:
- Receive create/delete tasks from Outer
- Create schema in CockroachDB (for RDB)
- Create namespace in TiKV (for KV)
- Update database status after creation

**Pattern**:
```go
type CreateRDBJob struct {
    UserUID    string
    Name       string
    ResourceID string
}

func (j *CreateRDBJob) Execute() error {
    // 1. Create schema in CockroachDB
    schemaName := fmt.Sprintf("user_%s_%s", j.UserUID, j.ResourceID)
    err := k8s.RDBManager.CreateSchema(schemaName)

    // 2. Update database status
    if err != nil {
        dblayer.UpdateCombinatorResourceStatus(j.ResourceID, "error", err.Error())
        return err
    }

    dblayer.UpdateCombinatorResourceStatus(j.ResourceID, "active", "")
    return nil
}

type DeleteRDBJob struct {
    UserUID    string
    ResourceID string
}

func (j *DeleteRDBJob) Execute() error {
    // 1. Drop schema in CockroachDB
    schemaName := fmt.Sprintf("user_%s_%s", j.UserUID, j.ResourceID)
    err := k8s.RDBManager.DropSchema(schemaName)

    // 2. No need to update database (already deleted by Outer)
    return err
}
```

### 4. K8s Controller

**What Inner Does**:
- Watch WorkerApp CRD changes
- Reconcile K8s resources (Deployment, Service, IngressRoute, ConfigMap, Secret)
- Handle sub-resource deletion (auto-recreate)
- Auto-restart on config changes

**Pattern**:
```go
func (wc *WorkerController) reconcile(u *unstructured.Unstructured) {
    w := workerFromUnstructured(u)
    if w == nil {
        return
    }

    ctx := context.Background()
    wc.ctrl.updateStatus(u, WorkerAppGVR, "Deploying", "")

    // Ensure all resources in order
    if err := w.EnsureConfigMap(ctx); err != nil {
        log.Printf("[controller] ensure configmap failed: %v", err)
        wc.ctrl.updateStatus(u, WorkerAppGVR, "Failed", err.Error())
        return
    }

    if err := w.EnsureSecret(ctx); err != nil {
        log.Printf("[controller] ensure secret failed: %v", err)
        wc.ctrl.updateStatus(u, WorkerAppGVR, "Failed", err.Error())
        return
    }

    if err := w.EnsureDeployment(ctx); err != nil {
        log.Printf("[controller] ensure deployment failed: %v", err)
        wc.ctrl.updateStatus(u, WorkerAppGVR, "Failed", err.Error())
        return
    }

    if err := w.EnsureService(ctx); err != nil {
        log.Printf("[controller] ensure service failed: %v", err)
        wc.ctrl.updateStatus(u, WorkerAppGVR, "Failed", err.Error())
        return
    }

    if err := w.EnsureIngressRoute(ctx); err != nil {
        log.Printf("[controller] ensure ingress route failed: %v", err)
        wc.ctrl.updateStatus(u, WorkerAppGVR, "Failed", err.Error())
        return
    }

    log.Printf("[controller] reconcile %s success", u.GetName())
    wc.ctrl.updateStatus(u, WorkerAppGVR, "Running", "")
}
```

### 5. Cron Jobs

**What Inner Does**:
- Run periodic tasks (user audit, domain verification)
- Schedule jobs at fixed intervals
- Execute jobs via Processor

**Pattern**:
```go
// Register cron jobs
cron.RegisterJob(24*time.Hour, jobs.NewUserAuditJob())
cron.RegisterJob(12*time.Hour, jobs.NewDomainCheckJob())

// User audit job
type UserAuditJob struct{}

func (j *UserAuditJob) Execute() error {
    // 1. Get all users from database
    users, err := dblayer.ListAllUsers()
    if err != nil {
        return err
    }

    // 2. Check each user's resources
    for _, user := range users {
        workers, _ := dblayer.ListWorkersByUser(user.UID)
        resources, _ := dblayer.ListCombinatorResources(user.UID, "")

        log.Printf("[audit] User %s: %d workers, %d resources",
            user.Email, len(workers), len(resources))
    }

    return nil
}

// Domain verification job
type DomainCheckJob struct{}

func (j *DomainCheckJob) Execute() error {
    // 1. Get all pending domains from database
    domains := k8s.ListCustomDomainsByStatus("pending")

    // 2. Verify each domain's TXT record
    for _, domain := range domains {
        verified := k8s.VerifyDNSTXT(domain.TXTName, domain.TXTValue)

        if verified {
            // 3. Create IngressRoute in K8s
            k8s.CreateIngressRouteForDomain(domain)

            // 4. Update database status
            k8s.UpdateCustomDomainStatus(domain.CDID, "success")
        }
    }

    return nil
}
```

### 6. Combinator Internal API

**What Inner Does**:
- Provide secret retrieval for Combinator pods
- Receive usage reports from Combinator pods
- Store usage data in database

**Pattern**:
```go
// Retrieve secret by ID (called by Combinator pods)
func (h *CombinatorInternalHandler) RetrieveSecretByID(c *gin.Context) {
    userID := c.Query("user_id")
    resourceID := c.Query("resource_id")

    // Get secret from database
    secretKey, err := dblayer.GetUserSecretKey(userID)
    if err != nil {
        c.JSON(404, gin.H{"error": "secret not found"})
        return
    }

    c.JSON(200, gin.H{"secret": secretKey})
}

// Report usage (called by Combinator pods)
func (h *CombinatorInternalHandler) ReportUsage(c *gin.Context) {
    var report dblayer.CombinatorResourceReport
    if err := c.ShouldBindJSON(&report); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // Store usage report in database
    if err := dblayer.CreateResourceReport(&report); err != nil {
        c.JSON(500, gin.H{"error": "failed to store report"})
        return
    }

    c.JSON(200, gin.H{"message": "report received"})
}
```

---

## API Endpoints

### Internal Routes (No Auth)

```
POST /api/worker/deploy
Body: {
  "user_uid": "string",
  "worker_id": "string",
  "image": "string",
  "port": number
}

GET /api/combinator/retrieveSecretByID?user_id=xxx&resource_id=xxx

POST /api/combinator/reportUsage
Body: {
  "user_id": "string",
  "resource_id": "string",
  "resource_type": "rdb|kv",
  "datachange": number,
  "timespan_start": "timestamp",
  "timespan_end": "timestamp"
}

POST /api/acceptTask
Body: {
  "task_type": "string",
  "task_info": "json"
}
```

---

## Coding Standards

### Job Pattern (Standard)

```go
type MyJob struct {
    // Job parameters
    UserUID    string
    ResourceID string
}

func (j *MyJob) Execute() error {
    // 1. Read from database (if needed)
    resource, err := dblayer.GetResource(j.ResourceID)
    if err != nil {
        return err
    }

    // 2. Perform operations (K8s, external services, etc.)
    err = k8s.CreateSomething(resource)

    // 3. Update database status
    if err != nil {
        dblayer.UpdateResourceStatus(j.ResourceID, "error", err.Error())
        return err
    }

    dblayer.UpdateResourceStatus(j.ResourceID, "active", "")
    return nil
}
```

### Database Status Update Pattern

```go
// Good: Update status after processing
func (j *DeployWorkerJob) Execute() error {
    // ... perform deployment ...

    if err != nil {
        // Update to "error" status
        dblayer.UpdateDeployVersionStatus(j.VersionID, "error", err.Error())
        dblayer.UpdateWorkerStatus(worker.ID, "error")
        return err
    }

    // Update to "active" status
    dblayer.UpdateDeployVersionStatus(j.VersionID, "success", "")
    dblayer.UpdateWorkerStatus(worker.ID, "active")
    dblayer.SetActiveVersion(worker.ID, j.VersionID)

    return nil
}
```

### K8s Resource Creation Pattern

```go
// Good: Idempotent resource creation
func (w *WorkerAppSpec) EnsureDeployment(ctx context.Context) error {
    deployment := w.buildDeployment()

    existing, err := k8s.K8sClient.AppsV1().
        Deployments(k8s.WorkerNamespace).
        Get(ctx, w.name(), metav1.GetOptions{})

    if err != nil {
        // Create new
        _, err = k8s.K8sClient.AppsV1().
            Deployments(k8s.WorkerNamespace).
            Create(ctx, deployment, metav1.CreateOptions{})
        return err
    }

    // Update existing
    deployment.ResourceVersion = existing.ResourceVersion
    _, err = k8s.K8sClient.AppsV1().
        Deployments(k8s.WorkerNamespace).
        Update(ctx, deployment, metav1.UpdateOptions{})
    return err
}
```

### Error Handling Pattern

```go
// Good: Log errors and update status
func (j *CreateRDBJob) Execute() error {
    err := k8s.RDBManager.CreateSchema(schemaName)

    if err != nil {
        log.Printf("[job] CreateRDB failed for %s: %v", j.ResourceID, err)
        dblayer.UpdateCombinatorResourceStatus(j.ResourceID, "error", err.Error())
        return err
    }

    log.Printf("[job] CreateRDB success for %s", j.ResourceID)
    dblayer.UpdateCombinatorResourceStatus(j.ResourceID, "active", "")
    return nil
}
```

---

## What Inner NEVER Does

### ❌ Don't: Accept Direct User Requests

```go
// BAD: Inner should NOT do this
func CreateWorker(c *gin.Context) {
    // ❌ Don't handle user requests in Inner
    userUID := c.GetString("user_id")  // No auth middleware in Inner!

    // ✅ Instead: Only accept tasks from Outer
}
```

### ❌ Don't: Serve Public API

```go
// BAD: Inner should NOT do this
router.GET("/api/worker", ListWorkers)  // ❌ Public endpoint

// ✅ Instead: Only internal endpoints
router.POST("/api/acceptTask", AcceptTask)  // ✅ Internal only
```

### ❌ Don't: Create Database Records

```go
// BAD: Inner should NOT do this
func (j *DeployWorkerJob) Execute() error {
    // ❌ Don't create new records in Inner
    workerID := uuid.New().String()
    dblayer.CreateWorker(workerID, userUID, name)

    // ✅ Instead: Only update existing records
    dblayer.UpdateWorkerStatus(worker.ID, "active")
}
```

---

## Processor & Cron

### Processor Configuration

```go
// Create processor with buffer and workers
proc := k8s.NewProcessor(256, 4)  // 256 buffer, 4 workers
proc.Start()
defer proc.Close()

// Submit jobs
proc.Submit(jobs.NewDeployWorkerJob(workerID, userUID, versionID))
```

### Cron Configuration

```go
// Create cron scheduler
cron := k8s.NewCronScheduler(proc)
cron.Start()
defer cron.Close()

// Register periodic jobs
cron.RegisterJob(24*time.Hour, jobs.NewUserAuditJob())
cron.RegisterJob(12*time.Hour, jobs.NewDomainCheckJob())

// Submit one-time job
proc.Submit(jobs.NewUserAuditJob())
```

---

## Job Types

### Worker Jobs

- `DeployWorkerJob` - Deploy worker to K8s (create WorkerApp CR)
- `DeleteWorkerCRJob` - Delete WorkerApp CR from K8s
- `SyncEnvJob` - Sync environment variables to ConfigMap
- `SyncSecretJob` - Sync secrets to Secret

### Combinator Jobs

- `CreateRDBJob` - Create RDB schema in CockroachDB
- `DeleteRDBJob` - Drop RDB schema from CockroachDB
- `CreateKVJob` - Create KV namespace in TiKV
- `DeleteKVJob` - Delete KV namespace from TiKV

### Periodic Jobs

- `UserAuditJob` - Audit user resources (24h interval)
- `DomainCheckJob` - Verify custom domains (12h interval)

### Auth Jobs

- `RegisterUserJob` - Post-registration setup

---

## Environment Variables

Required:
- `DOMAIN` - Base domain for ingress (e.g., `example.com`)

Optional:
- `ENV=test` - Enable debug mode

Database connection:
- Passed via `-d` flag: `postgresql://user:pass@host:port/db?sslmode=disable`

K8s config:
- Passed via `-k` flag (empty for in-cluster config)

---

## Logging Standards

```go
// Good: Structured logging with context
log.Printf("[controller] WorkerApp added: %s", name)
log.Printf("[controller] reconcile %s success", name)
log.Printf("[job] DeployWorker started for %s", workerID)
log.Printf("[job] CreateRDB success for %s", resourceID)
log.Printf("Warning: CockroachDB init failed: %v", err)
log.Printf("Failed to send task: %v", err)

// Bad: Unstructured logging
log.Println("something happened")
log.Println(err)
```

**Prefixes**:
- `[controller]` - K8s controller operations
- `[job]` - Background job execution
- `[processor]` - Task processor operations
- `[cron]` - Cron scheduler operations

---

## Summary

**Inner Gateway = Processing Layer**

- ✅ Receive tasks from Outer
- ✅ Process background jobs
- ✅ Modify K8s cluster
- ✅ Update database status
- ✅ Run cron jobs
- ✅ Operate K8s controller

**Inner Gateway ≠ API Layer**

- ❌ No public API
- ❌ No user authentication
- ❌ No frontend serving
- ❌ No direct user requests
- ❌ No creating database records (only updates)

> **Remember**: Inner receives events, processes them, and updates cluster + database.
