# Console Platform Architecture

> **Purpose**: Core architectural principles and component overview

---

## Overview

**Console** is a Serverless platform (similar to Cloudflare Workers or Vercel) that provides:
- Worker deployment (containerized applications)
- Combinator resources (RDB, KV, S3, MQ)
- Custom domain management
- Multi-tenant isolation

---

## Core Architecture Pattern

### Dual Gateway Design

```
┌─────────────────────────────────────────────────────────┐
│                    User / Client                        │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              Outer Gateway (Port 9900)                  │
│  ┌──────────────────────────────────────────────────┐  │
│  │  1. Validate request (auth, input)               │  │
│  │  2. Write to database (status: "loading")        │  │
│  │  3. Send task to Inner Gateway                   │  │
│  │  4. Return 200 immediately                       │  │
│  └──────────────────────────────────────────────────┘  │
└────────────────────┬────────────────────────────────────┘
                     │ (HTTP POST /api/acceptTask)
                     ▼
┌─────────────────────────────────────────────────────────┐
│              Inner Gateway (Port 9901)                  │
│  ┌──────────────────────────────────────────────────┐  │
│  │  1. Receive task from Outer                      │  │
│  │  2. Enqueue to Processor                         │  │
│  │  3. Worker picks up task                         │  │
│  │  4. Execute job (create K8s resources, etc.)     │  │
│  │  5. Update database status ("active"/"error")    │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

---

## 🎯 Critical Principle: Responsibility Separation

### Outer Gateway Responsibilities

**ONLY does:**
- ✅ Write to database (create records)
- ✅ Send events/tasks to Inner Gateway
- ✅ Read from database (query records)
- ✅ Serve frontend static files
- ✅ Authenticate users (JWT, password hashing)

**NEVER does:**
- ❌ Modify K8s cluster
- ❌ Process background jobs
- ❌ Update database status (except user auth)
- ❌ Create K8s resources
- ❌ Execute async operations

### Inner Gateway Responsibilities

**ONLY does:**
- ✅ Receive tasks/events from Outer Gateway
- ✅ Process background jobs (async operations)
- ✅ Modify K8s cluster (create/update/delete resources)
- ✅ Update database status (after processing)
- ✅ Run cron jobs (periodic tasks)
- ✅ Operate K8s controller (watch CRDs, reconcile)

**NEVER does:**
- ❌ Serve public API
- ❌ Handle user authentication
- ❌ Serve frontend
- ❌ Accept direct user requests
- ❌ Create database records (only updates)

> **Rule**: Outer writes data and sends events. Inner processes events and modifies cluster.

---

## Component Overview

### 1. Outer Gateway (`cmd/outer/main.go`)

**Port**: 9900
**Access**: Public internet (with authentication)

**Components**:
- **API Layer**: RESTful API for user operations
- **Web Frontend**: User interface (Terminal UI + GUI)
- **Deployment Scripts**: K8s deployment YAML files

**Key Features**:
- User authentication (JWT, email verification)
- Worker management API
- Combinator resource API
- Custom domain management
- Frontend serving

### 2. Inner Gateway (`cmd/inner/main.go`)

**Port**: 9901
**Access**: Internal cluster services only

**Components**:
- **Task Processor**: Background job execution (256 buffer, 4 workers)
- **K8s Controller**: Watch WorkerApp CRD, reconcile resources
- **Cron Scheduler**: Periodic tasks (user audit, domain verification)

**Key Features**:
- Worker deployment to K8s
- Combinator resource creation (RDB/KV schemas)
- K8s resource reconciliation
- Background job processing
- Internal API for Combinator pods

### 3. Web Frontend (`web/`)

**Tech Stack**: React 19 + TypeScript + Vite + Tailwind CSS

**Dual UI System**:

#### Terminal UI (Mandatory)
- Command-line interface in browser
- Direct API calls with curl-like syntax
- JSON response display
- **Why mandatory**: Forces API to be simple and consistent

#### GUI (Optional)
- Graphical interface for user convenience
- Dashboard, visual management, drag-and-drop
- **Design principle**: Every GUI action must have Terminal equivalent

### 4. Deployment Scripts (`scripts/`)

**Purpose**: K8s deployment YAML files for entire platform

**Key Files**:
- `control-plane-deployment.yaml` - Outer + Inner gateways
- `combinator-deployment.yaml` - Combinator service
- `cockroachdb-deployment.yaml` - CockroachDB (RDB)
- `tikv-deployment.yaml` - TiKV (KV)
- `ingress.yaml` - Traefik routing
- `init.sql` - Database schema

### 5. Worker System

**User-deployed containerized applications**:
- Custom Docker images
- Environment variables and secrets
- Version control
- Auto-generated URLs: `https://{workerID}.worker.{DOMAIN}`

**K8s Resources** (auto-created by Inner):
- WorkerApp CRD (custom resource)
- Deployment (pod management)
- Service (internal networking)
- IngressRoute (Traefik routing)
- ConfigMap (environment variables)
- Secret (sensitive data)

### 6. Combinator System

**Unified resource gateway** (independent submodule):
- **RDB**: CockroachDB (schema per user)
- **KV**: TiKV (namespace per user)
- **S3**: SeaweedFS (bucket per user)
- **MQ**: Message queue (planned)

**Access Pattern**:
- Workers access via Combinator SDK
- Combinator pods retrieve secrets from Inner
- Usage reports sent to Inner

### 7. Custom Domain Service

**Features**:
- TXT record verification
- Auto-create IngressRoute
- cert-manager integration
- SSL certificate auto-issuance

**Flow**:
1. User adds domain (Outer writes to DB)
2. Outer sends verification task to Inner
3. Inner verifies TXT record (cron job)
4. Inner creates IngressRoute in K8s
5. Inner updates DB status

---

## Data Flow Examples

### Example 1: Worker Deployment

```
1. User: POST /api/worker/deploy
   ↓
2. Outer:
   - Validate request (HMAC signature)
   - Create deploy_version record (status: "loading")
   - Send DeployWorkerJob to Inner
   - Return 200 {"version_id": 123, "status": "loading"}
   ↓
3. Inner:
   - Receive task
   - Get worker + version from DB
   - Create WorkerApp CR in K8s
   - Update deploy_version status ("success")
   - Update worker status ("active")
   ↓
4. K8s Controller:
   - Watch WorkerApp CR
   - Create Deployment, Service, IngressRoute, ConfigMap, Secret
   - Update WorkerApp status ("Running")
   ↓
5. User: GET /api/worker/:id
   - Outer returns worker with status "active"
```

### Example 2: RDB Creation

```
1. User: POST /api/rdb
   ↓
2. Outer:
   - Create combinator_resource record (status: "loading")
   - Send CreateRDBJob to Inner
   - Return 200 {"id": "abc123", "status": "loading"}
   ↓
3. Inner:
   - Receive task
   - Create schema in CockroachDB
   - Update combinator_resource status ("active")
   ↓
4. User: GET /api/rdb/:id
   - Outer returns resource with status "active"
```

---

## Database Schema

**PostgreSQL** (control plane data):
- `users` - User accounts
- `workers` - Worker instances
- `worker_deploy_versions` - Deployment versions
- `combinator_resources` - RDB/KV resources
- `custom_domains` - Domain mappings
- `console_tasks` - Background tasks

**CockroachDB** (user RDB):
- Schema per user: `user_{uid}_{resource_id}`
- SQL-compatible

**TiKV** (user KV):
- Namespace per user
- High-performance key-value store

---

## Key Design Decisions

### 1. Why Dual Gateway?

**Separation of Concerns**:
- Outer handles user-facing operations (fast response)
- Inner handles heavy processing (K8s, external services)
- Prevents blocking user requests

**Security**:
- Inner is not exposed to public internet
- Only Outer has authentication logic
- Inner trusts Outer's validation

**Scalability**:
- Outer can scale independently (stateless)
- Inner can scale based on job queue depth
- Different resource requirements

### 2. Why Terminal UI is Mandatory?

**API Simplicity**:
- Forces every operation to be API-accessible
- Prevents "GUI-only" features
- Serves as living API documentation

**Debugging**:
- Direct API interaction
- See raw requests/responses
- Test edge cases easily

**Consistency**:
- GUI is just a wrapper around Terminal commands
- Terminal is the source of truth
- Ensures API-first design

### 3. Why Async Task Pattern?

**User Experience**:
- Immediate response (no waiting)
- Non-blocking operations
- Status polling for updates

**Reliability**:
- Retry failed jobs
- Job queue persistence
- Error isolation

**Resource Management**:
- Worker pool limits concurrency
- Prevents resource exhaustion
- Fair scheduling

---

## Technology Stack

### Backend
- **Language**: Go 1.25
- **Web Framework**: Gin
- **Database**: PostgreSQL (control plane)
- **K8s Client**: client-go
- **JWT**: golang-jwt/jwt/v5

### Frontend
- **Framework**: React 19
- **Build Tool**: Vite 7
- **Styling**: Tailwind CSS 4
- **Language**: TypeScript

### Infrastructure
- **Orchestration**: Kubernetes (K3s)
- **Ingress**: Traefik
- **Certificates**: cert-manager + ZeroSSL
- **RDB**: CockroachDB
- **KV**: TiKV
- **S3**: SeaweedFS

---

## Common Patterns

### Pattern 1: Async Operation

```go
// Outer: Create record + send task
func CreateResource(c *gin.Context) {
    resourceID := GenerateID()
    dblayer.CreateResource(userUID, resourceID)  // status: "loading"
    SendTask(jobs.NewCreateResourceJob(userUID, resourceID))
    c.JSON(200, gin.H{"id": resourceID, "status": "loading"})
}

// Inner: Process task + update status
func (j *CreateResourceJob) Execute() error {
    err := k8s.CreateSomething(j.ResourceID)
    if err != nil {
        dblayer.UpdateResourceStatus(j.ResourceID, "error", err.Error())
        return err
    }
    dblayer.UpdateResourceStatus(j.ResourceID, "active", "")
    return nil
}
```

### Pattern 2: K8s CRD Reconciliation

```go
// Inner: Controller watches CRD
func (wc *WorkerController) reconcile(u *unstructured.Unstructured) {
    w := workerFromUnstructured(u)
    wc.ctrl.updateStatus(u, WorkerAppGVR, "Deploying", "")

    // Ensure all resources
    w.EnsureConfigMap(ctx)
    w.EnsureSecret(ctx)
    w.EnsureDeployment(ctx)
    w.EnsureService(ctx)
    w.EnsureIngressRoute(ctx)

    wc.ctrl.updateStatus(u, WorkerAppGVR, "Running", "")
}
```

### Pattern 3: Ownership Validation

```go
// Database: Atomic operation with ownership check
func DeleteWorkerByOwner(workerID, userUID string) error {
    result, err := DB.Exec(`
        DELETE FROM workers
        WHERE wid = $1 AND user_uid = $2
    `, workerID, userUID)

    if rowsAffected == 0 {
        return ErrNotFound
    }
    return err
}
```

---

## Related Documentation

- [Outer Gateway Guidelines](./ outer/index.md) - API, frontend, deployment
- [Inner Gateway Guidelines](./inner/index.md) - Task processing, K8s operations
- [Cross-Layer Thinking Guide](./guides/cross-layer-thinking-guide.md) - Data flow patterns

---

## Quick Reference

### Outer Gateway
- **Port**: 9900
- **Role**: API layer, write DB, send tasks
- **Never**: Modify K8s, process jobs, update status

### Inner Gateway
- **Port**: 9901
- **Role**: Process tasks, modify K8s, update status
- **Never**: Public API, user auth, create DB records

### Terminal UI
- **Status**: Mandatory
- **Purpose**: Ensure API simplicity
- **Principle**: Source of truth for all operations

### GUI
- **Status**: Optional
- **Purpose**: User convenience
- **Principle**: Wrapper around Terminal commands
