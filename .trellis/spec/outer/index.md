# Outer Gateway Development Guidelines

> **Purpose**: Standards for developing the Outer Gateway (public API + frontend)

---

## Overview

The **Outer Gateway** (`cmd/outer/main.go`) is the public-facing service.

**Port**: 9900
**Access**: Public internet (with authentication)

### Components

| Component | Purpose | Location |
|-----------|---------|----------|
| **API** | RESTful API for user operations | `cmd/outer/main.go`, `handlers/` |
| **Web Frontend** | User interface (Terminal + GUI) | `web/` |
| **Deployment Scripts** | K8s deployment YAML files | `scripts/` |

---

## 🎯 Core Principle: Outer Gateway Responsibilities

**Outer Gateway ONLY does:**
1. ✅ **Write to database** (create records)
2. ✅ **Send events/tasks** to Inner Gateway
3. ✅ **Read from database** (query records)
4. ✅ **Serve frontend** static files
5. ✅ **Authenticate users** (JWT, password hashing)

**Outer Gateway NEVER does:**
- ❌ Modify K8s cluster
- ❌ Process background jobs
- ❌ Update database status (except user auth)
- ❌ Create K8s resources
- ❌ Execute async operations

> **Rule**: Outer writes data and sends events. Inner processes events and modifies cluster.

---

## Architecture Pattern

```
User Request
    ↓
Outer Gateway
    ↓
1. Validate request (auth, input)
2. Write to database (status: "loading")
3. Send task to Inner Gateway
4. Return 200 immediately
    ↓
Inner Gateway (async)
    ↓
1. Receive task
2. Process (create K8s resources, etc.)
3. Update database status ("active" or "error")
```

---

## Key Responsibilities

### 1. User Authentication

**What Outer Does**:
- Validate email/password
- Generate JWT tokens
- Send verification codes (via Resend API)
- Hash passwords with bcrypt
- Create user records in database

**Pattern**:
```go
// Outer: Register user
func Register(c *gin.Context) {
    // 1. Validate input
    var req RegisterRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // 2. Write to database
    userUID, err := dblayer.CreateUser(uid, email, hash, secretKey)
    if err != nil {
        c.JSON(400, gin.H{"error": "email already exists"})
        return
    }

    // 3. Send task to Inner (for post-registration setup)
    SendTask(jobs.NewRegisterUserJob(userUID))

    // 4. Return immediately
    token, _ := GenerateToken(userUID, email)
    c.JSON(200, gin.H{
        "user_id": userUID,
        "token": token,
        "secret_key": secretKey,
    })
}
```

### 2. Worker Management

**What Outer Does**:
- Create worker record in database
- List user's workers (read-only)
- Get worker details (read-only)
- Delete worker record from database
- Update env/secrets in database
- Send deployment task to Inner

**Pattern**:
```go
// Outer: Create worker
func CreateWorker(c *gin.Context) {
    userUID := c.GetString("user_id")
    var req struct {
        WorkerName string `json:"worker_name" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    workerID := uuid.New().String()[:8]

    // ONLY write to database
    if err := dblayer.CreateWorker(workerID, userUID, req.WorkerName); err != nil {
        c.JSON(500, gin.H{"error": "failed to create worker"})
        return
    }

    // Return immediately (no K8s operations)
    c.JSON(200, gin.H{
        "worker_id": workerID,
        "worker_name": req.WorkerName,
    })
}

// Outer: Deploy worker
func DeployWorker(c *gin.Context) {
    var req DeployRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // 1. Write version record to database (status: "loading")
    versionID, err := dblayer.CreateDeployVersionForOwner(
        req.WorkerID, req.UserUID, req.Image, req.Port)
    if err != nil {
        c.JSON(404, gin.H{"error": "worker not found"})
        return
    }

    // 2. Send task to Inner Gateway
    if err := SendTask(jobs.NewDeployWorkerJob(req.WorkerID, req.UserUID, versionID)); err != nil {
        c.JSON(500, gin.H{"error": "failed to enqueue deploy task"})
        return
    }

    // 3. Return immediately with "loading" status
    c.JSON(200, gin.H{
        "version_id": versionID,
        "status": "loading",  // Inner will update to "success" or "error"
    })
}

// Outer: Delete worker
func DeleteWorker(c *gin.Context) {
    userUID := c.GetString("user_id")
    workerID := c.Param("id")

    // 1. Send task to Inner (to delete K8s resources)
    SendTask(jobs.NewDeleteWorkerCRJob(workerID, userUID))

    // 2. Delete from database
    if err := dblayer.DeleteWorkerByOwner(workerID, userUID); err != nil {
        c.JSON(404, gin.H{"error": "worker not found"})
        return
    }

    // 3. Return immediately
    c.JSON(200, gin.H{"message": "worker deleted"})
}
```

### 3. Combinator Resources

**What Outer Does**:
- Create resource record in database (status: "loading")
- List user's resources (read-only)
- Get resource details (read-only)
- Delete resource record from database
- Send create/delete tasks to Inner

**Pattern**:
```go
// Outer: Create RDB
func CreateRDB(c *gin.Context) {
    userUID := c.GetString("user_id")
    var req struct {
        Name string `json:"name" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // 1. Write to database (status: "loading")
    resourceID := GenerateResourceUID()
    if err := dblayer.CreateCombinatorResource(userUID, "rdb", resourceID); err != nil {
        c.JSON(500, gin.H{"error": "failed to create resource"})
        return
    }

    // 2. Send task to Inner (to create schema in CockroachDB)
    if err := SendTask(jobs.NewCreateRDBJob(userUID, req.Name, resourceID)); err != nil {
        c.JSON(500, gin.H{"error": "failed to enqueue create task"})
        return
    }

    // 3. Return immediately with "loading" status
    c.JSON(200, gin.H{
        "id": resourceID,
        "status": "loading",  // Inner will update to "active" or "error"
    })
}
```

### 4. Custom Domains

**What Outer Does**:
- Create domain record in database (status: "pending")
- List user's domains (read-only)
- Get domain details (read-only)
- Delete domain record from database
- Trigger verification (send task to Inner)

**Pattern**:
```go
// Outer: Add custom domain
func AddCustomDomain(c *gin.Context) {
    userUID := c.GetString("user_id")
    var req struct {
        Domain string `json:"domain" binding:"required"`
        Target string `json:"target" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // 1. Write to database (status: "pending")
    cd, err := k8s.NewCustomDomain(userUID, req.Domain, req.Target)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    // 2. Trigger verification (Inner will verify TXT and create IngressRoute)
    cd.StartVerification()

    // 3. Return immediately
    c.JSON(200, gin.H{
        "id": cd.ID,
        "domain": cd.Domain,
        "txt_name": cd.TXTName,
        "txt_value": cd.TXTValue,
        "status": "pending",  // Inner will update to "success" or "error"
    })
}
```

### 5. Frontend Serving

**What Outer Does**:
- Serve React SPA from `dist/`
- Serve static assets from `dist/assets/`
- Support both Terminal UI and GUI

```go
// Serve frontend
router.Static("/assets", "./dist/assets")
router.GET("/", func(c *gin.Context) {
    c.File("./dist/index.html")
})
```

**Frontend Architecture**:
- **Terminal UI**: Command-line interface for API interaction (ensures API simplicity)
- **GUI**: Graphical interface for user convenience

> **Important**: Terminal UI is mandatory - it guarantees API simplicity and must be preserved. GUI is optional for better UX.

---

## API Endpoints

### Public Routes (No Auth)

```
POST /api/auth/register       # Create user + send task
POST /api/auth/login          # Validate + return JWT
POST /api/auth/send-code      # Send verification email
POST /api/auth/reset-password # Update password
```

### Protected Routes (JWT Required)

**Workers**:
```
GET    /api/worker            # Read from database
GET    /api/worker/:id        # Read from database
POST   /api/worker            # Write to database
DELETE /api/worker/:id        # Write to database + send task
GET    /api/worker/:id/env    # Read from database
POST   /api/worker/:id/env    # Write to database + send task
GET    /api/worker/:id/secret # Read from database
POST   /api/worker/:id/secret # Write to database + send task
```

**Combinator**:
```
GET    /api/rdb               # Read from database
GET    /api/rdb/:id           # Read from database
POST   /api/rdb               # Write to database + send task
DELETE /api/rdb/:id           # Write to database + send task
GET    /api/kv                # Read from database
POST   /api/kv                # Write to database + send task
DELETE /api/kv/:id            # Write to database + send task
```

**Custom Domains**:
```
GET    /api/domain            # Read from database
GET    /api/domain/:id        # Read from database
POST   /api/domain            # Write to database + send task
DELETE /api/domain/:id        # Write to database + send task
```

### Sensitive Routes (HMAC Signature Required)

```
POST /api/worker/deploy       # Write to database + send task
```

---

## Coding Standards

### Handler Pattern (Standard)

```go
func (h *Handler) CreateResource(c *gin.Context) {
    // 1. Get authenticated user
    userUID := c.GetString("user_id")

    // 2. Validate input
    var req CreateRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // 3. Write to database (status: "loading")
    resourceID := GenerateID()
    if err := dblayer.CreateResource(userUID, resourceID); err != nil {
        c.JSON(500, gin.H{"error": "failed to create resource"})
        return
    }

    // 4. Send task to Inner Gateway
    if err := SendTask(jobs.NewCreateResourceJob(userUID, resourceID)); err != nil {
        c.JSON(500, gin.H{"error": "failed to enqueue task"})
        return
    }

    // 5. Return immediately
    c.JSON(200, gin.H{
        "id": resourceID,
        "status": "loading",
    })
}
```

### Read-Only Handler Pattern

```go
func (h *Handler) ListResources(c *gin.Context) {
    userUID := c.GetString("user_id")

    // ONLY read from database
    resources, err := dblayer.ListResources(userUID)
    if err != nil {
        c.JSON(500, gin.H{"error": "failed to list resources"})
        return
    }

    c.JSON(200, gin.H{"resources": resources})
}
```

### Delete Handler Pattern

```go
func (h *Handler) DeleteResource(c *gin.Context) {
    userUID := c.GetString("user_id")
    resourceID := c.Param("id")

    // 1. Send task to Inner (to clean up K8s/external resources)
    SendTask(jobs.NewDeleteResourceJob(userUID, resourceID))

    // 2. Delete from database
    if err := dblayer.DeleteResource(userUID, resourceID); err != nil {
        c.JSON(404, gin.H{"error": "resource not found"})
        return
    }

    // 3. Return immediately
    c.JSON(200, gin.H{"message": "deleted"})
}
```

---

## What Outer NEVER Does

### ❌ Don't: Modify K8s Cluster

```go
// BAD: Outer should NOT do this
func DeployWorker(c *gin.Context) {
    // ❌ Don't create K8s resources in Outer
    k8s.K8sClient.AppsV1().Deployments(namespace).Create(...)

    // ✅ Instead: Send task to Inner
    SendTask(jobs.NewDeployWorkerJob(...))
}
```

### ❌ Don't: Update Database Status

```go
// BAD: Outer should NOT do this
func CreateRDB(c *gin.Context) {
    resourceID := GenerateID()
    dblayer.CreateCombinatorResource(userUID, "rdb", resourceID)

    // ❌ Don't update status in Outer
    dblayer.UpdateCombinatorResourceStatus(resourceID, "active", "")

    // ✅ Instead: Let Inner update status after processing
    SendTask(jobs.NewCreateRDBJob(userUID, name, resourceID))
}
```

### ❌ Don't: Process Background Jobs

```go
// BAD: Outer should NOT do this
func CreateResource(c *gin.Context) {
    resourceID := GenerateID()
    dblayer.CreateResource(resourceID)

    // ❌ Don't process synchronously in Outer
    go func() {
        // Create external resources...
        dblayer.UpdateStatus(resourceID, "active")
    }()

    // ✅ Instead: Send task to Inner
    SendTask(jobs.NewCreateResourceJob(resourceID))
}
```

---

## Task Sending Pattern

```go
// Send task to Inner Gateway
func SendTask(job Job) error {
    // Serialize job
    taskInfo, _ := json.Marshal(job)

    // Send to Inner Gateway's /api/acceptTask endpoint
    resp, err := http.Post(
        "http://control-plane-inner.console.svc.cluster.local:9901/api/acceptTask",
        "application/json",
        bytes.NewBuffer(taskInfo),
    )

    if err != nil || resp.StatusCode != 200 {
        return fmt.Errorf("failed to send task")
    }

    return nil
}
```

---

## Frontend Guidelines

### Directory Structure

```
web/
├── src/
│   ├── components/          # Reusable components
│   │   ├── terminal/        # Terminal UI components
│   │   └── gui/             # GUI components
│   ├── pages/               # Page components
│   ├── hooks/               # Custom hooks
│   ├── utils/               # Utility functions
│   ├── types/               # TypeScript types
│   ├── App.tsx              # Main app component
│   └── main.tsx             # Entry point
├── dist/                    # Built frontend (served by outer)
├── package.json
└── vite.config.ts
```

### Tech Stack

- **Framework**: React 19
- **Build Tool**: Vite 7
- **Styling**: Tailwind CSS 4
- **Routing**: React Router 7
- **Language**: TypeScript

### UI Architecture

#### 1. Terminal UI (Mandatory)

**Purpose**: Ensures API simplicity and provides direct API interaction

**Features**:
- Command-line interface in browser
- Direct API calls with curl-like syntax
- JSON response display
- Request/response history
- Authentication token management

**Why Mandatory**:
- Forces API to be simple and consistent
- Provides debugging interface
- Ensures all operations are API-accessible
- Serves as API documentation

**Example Terminal Commands**:
```bash
# Register user
> register email@example.com password123

# Login
> login email@example.com password123

# Create worker
> worker create my-worker

# Deploy worker
> worker deploy worker-id nginx:latest 80

# List workers
> worker list
```

#### 2. GUI (Optional, for UX)

**Purpose**: User-friendly interface for common operations

**Features**:
- Dashboard with resource overview
- Visual worker management
- Drag-and-drop deployment
- Real-time status updates
- Resource usage charts

**Design Principles**:
- Every GUI action must have Terminal equivalent
- GUI is a wrapper around Terminal commands
- Terminal UI is the source of truth

### API Integration

```typescript
// Good: Centralized API client
const API_BASE = '/api';

async function createWorker(token: string, name: string) {
  const response = await fetch(`${API_BASE}/worker`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ worker_name: name }),
  });

  if (!response.ok) {
    throw new Error('Failed to create worker');
  }

  return response.json();
}

// Poll for status updates
async function pollWorkerStatus(token: string, workerId: string) {
  const response = await fetch(`${API_BASE}/worker/${workerId}`, {
    headers: {
      'Authorization': `Bearer ${token}`,
    },
  });

  const data = await response.json();
  return data.status; // "loading", "active", "error"
}
```

---

## Deployment Scripts

### Overview

The `scripts/` directory contains K8s deployment YAML files for the entire Console platform.

**Location**: `scripts/`
**Purpose**: Deploy and configure all platform components on Kubernetes

### Directory Structure

```
scripts/
├── init.sql                          # Database schema initialization
├── README.md                         # Deployment guide
├── ns.yaml                           # Namespace definitions
├── control-plane-deployment.yaml     # Outer + Inner gateway deployment
├── combinator-deployment.yaml        # Combinator service deployment
├── cockroachdb-deployment.yaml       # CockroachDB (RDB) deployment
├── tikv-deployment.yaml              # TiKV (KV) deployment
├── seaweedfs-deployment.yaml         # SeaweedFS (S3) deployment
├── ingress.yaml                      # Traefik IngressRoute configuration
├── cert-manager-rfc2136.yaml         # cert-manager DNS-01 challenge
├── zerossl-issuer.yaml               # ZeroSSL certificate issuer
└── powerdns-geoip-deployment.yaml    # PowerDNS with GeoIP (optional)
```

### Key Deployment Files

#### 1. Control Plane Deployment

**File**: `control-plane-deployment.yaml`

Deploys both Outer and Inner gateways:
- **Outer Gateway**: Port 9900 (public)
- **Inner Gateway**: Port 9901 (internal)
- Shared PostgreSQL database
- Environment variables (DOMAIN, RESEND_API_KEY)

#### 2. Combinator Deployment

**File**: `combinator-deployment.yaml`

Deploys the unified resource gateway:
- RDB access (CockroachDB)
- KV access (TiKV)
- S3 access (SeaweedFS)
- Message queue (future)

#### 3. Database Deployments

**CockroachDB** (`cockroachdb-deployment.yaml`):
- Multi-tenant RDB
- Schema per user
- SQL-compatible

**TiKV** (`tikv-deployment.yaml`):
- Distributed KV store
- Namespace per user
- High performance

**SeaweedFS** (`seaweedfs-deployment.yaml`):
- S3-compatible object storage
- Bucket per user

#### 4. Ingress Configuration

**File**: `ingress.yaml`

Traefik IngressRoute for:
- `console.${DOMAIN}` → Outer Gateway (9900)
- `*.worker.${DOMAIN}` → Worker pods
- `*.combinator.${DOMAIN}` → Combinator pods

#### 5. Certificate Management

**cert-manager** (`cert-manager-rfc2136.yaml`):
- Automatic SSL certificate issuance
- DNS-01 challenge via RFC2136

**ZeroSSL** (`zerossl-issuer.yaml`):
- Certificate issuer configuration
- EAB credentials

### Deployment Order

```bash
# 1. Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# 2. Set environment variables
export DOMAIN="example.com"
export ZEROSSL_EAB_KID="your_eab_kid"
export ZEROSSL_EAB_HMAC_KEY="your_eab_hmac_key"
export RESEND_API_KEY="your_resend_key"

# 3. Deploy in order
kubectl apply -f scripts/ns.yaml
envsubst < scripts/cockroachdb-deployment.yaml | kubectl apply -f -
envsubst < scripts/tikv-deployment.yaml | kubectl apply -f -
envsubst < scripts/seaweedfs-deployment.yaml | kubectl apply -f -
envsubst < scripts/combinator-deployment.yaml | kubectl apply -f -
envsubst < scripts/zerossl-issuer.yaml | kubectl apply -f -
envsubst < scripts/ingress.yaml | kubectl apply -f -
envsubst < scripts/control-plane-deployment.yaml | kubectl apply -f -

# 4. Initialize database
kubectl exec -it postgres-0 -n console -- psql -U postgres -d combfather -f /init.sql
```

### Environment Variables

Required for deployment:
- `DOMAIN` - Base domain (e.g., `example.com`)
- `ZEROSSL_EAB_KID` - ZeroSSL EAB key ID
- `ZEROSSL_EAB_HMAC_KEY` - ZeroSSL EAB HMAC key
- `RESEND_API_KEY` - Resend API key for email
- `CLOUDFLARE_API_TOKEN` - Cloudflare API token (for DNS)

### Database Initialization

**File**: `scripts/init.sql`

Creates all required tables:
- `users` - User accounts
- `verification_codes` - Email verification
- `workers` - Worker instances
- `worker_deploy_versions` - Deployment versions
- `custom_domains` - Custom domain mappings
- `combinator_resources` - RDB/KV resources
- `combinator_resource_reports` - Usage reports
- `console_tasks` - Background tasks

### Deployment Best Practices

1. **Use envsubst**: Always use `envsubst` to inject environment variables
2. **Check order**: Deploy dependencies first (databases before services)
3. **Verify health**: Check pod status after each deployment
4. **Review logs**: Monitor logs for errors during startup
5. **Test connectivity**: Verify services can reach each other

### Common Deployment Commands

```bash
# Check deployment status
kubectl get all -n console
kubectl get all -n combinator
kubectl get all -n worker

# View logs
kubectl logs -f deployment/control-plane-outer -n console
kubectl logs -f deployment/control-plane-inner -n console

# Restart deployments
kubectl rollout restart deployment/control-plane-outer -n console
kubectl rollout restart deployment/control-plane-inner -n console

# Port forward for testing
kubectl port-forward -n console svc/control-plane-outer 9900:9900
kubectl port-forward -n console svc/control-plane-inner 9901:9901
```

---

## Environment Variables

Required:
- `DOMAIN` - Base domain (e.g., `example.com`)
- `RESEND_API_KEY` - Resend API key for email

Optional:
- `ENV=test` - Enable debug mode (CORS, skip env checks)

---

## Summary

**Outer Gateway = API Layer + Frontend + Deployment**

- ✅ Validate requests
- ✅ Write to database
- ✅ Send tasks to Inner
- ✅ Return immediately
- ✅ Serve frontend (Terminal UI + GUI)
- ✅ Provide deployment scripts

**Outer Gateway ≠ Processing Layer**

- ❌ No K8s operations
- ❌ No status updates (except auth)
- ❌ No background processing
- ❌ No external resource creation

> **Remember**:
> - Outer writes and sends. Inner processes and updates.
> - Terminal UI is mandatory - ensures API simplicity.
> - GUI is optional - improves user experience.
> - Scripts deploy the entire platform on K8s.
