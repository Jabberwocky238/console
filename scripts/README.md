# Console Deployment Guide

This guide will walk you through deploying the Console control plane on a K3s cluster.

如需克隆项目时同时拉取 submodule：                                                                                                 
git clone --recurse-submodules <repo-url>                                                                                        

或在已克隆的项目中初始化：

git submodule update --init --recursive

## Quick Start - Download Configuration Files

Download all required configuration files first:

```bash
# Set your GitHub username or raw file URL base

# Download all configuration files
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml
kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=cert-manager -n cert-manager --timeout=300s
curl -O "https://raw.githubusercontent.com/jabberwocky238/console/main/scripts/zerossl-issuer.yaml"
curl -O "https://raw.githubusercontent.com/jabberwocky238/console/main/scripts/ingress.yaml"
curl -O "https://raw.githubusercontent.com/jabberwocky238/console/main/scripts/control-plane-deployment.yaml"

export ZEROSSL_EAB_KID=your_eab_kid
export ZEROSSL_EAB_HMAC_KEY=your_eab_hmac_key
export CLOUDFLARE_API_TOKEN=your_cloudflare_token
export DOMAIN=example.com

# Deploy in order
envsubst < zerossl-issuer.yaml | kubectl apply -f -
envsubst < zerossl-issuer.yaml | kubectl delete -f -
envsubst < ingress.yaml | kubectl apply -f -
envsubst < ingress.yaml | kubectl delete -f -
envsubst < control-plane-deployment.yaml | kubectl apply -f -
envsubst < control-plane-deployment.yaml | kubectl delete -f -

# Or delete by namespace
kubectl delete namespace console
kubectl delete namespace ingress
```

### Rollout and Restart Commands
```bash
# Restart control plane
kubectl rollout restart deployment/control-plane -n console

# Restart PostgreSQL
kubectl rollout restart deployment/postgres -n console

# Check rollout status
kubectl rollout status deployment/control-plane -n console
kubectl rollout status deployment/postgres -n console

# Rollback to previous version
kubectl rollout undo deployment/control-plane -n console

# View rollout history
kubectl rollout history deployment/control-plane -n console
```

### Quick Debug Commands
```bash
# View all resources
kubectl get all -n console
kubectl get all -n ingress

# View logs
kubectl logs -f deployment/control-plane -n console
kubectl logs -f deployment/postgres -n console
kubectl logs combinator-jabberwocky7545 -n combinator
# Describe resources for troubleshooting
kubectl describe pod -l app=control-plane -n console
kubectl describe pod -l app=postgres -n console

# Execute commands in pods
kubectl exec -it deployment/control-plane -n console -- sh
kubectl exec -it deployment/postgres -n console -- psql -U postgres -d combfather

# Port forward for local testing
kubectl port-forward -n console svc/control-plane 9900:9900
kubectl port-forward -n console svc/postgres 5432:5432
```

---

## Prerequisites

- K3s cluster installed and running
- `kubectl` configured to access your cluster
- Domain name configured (for SSL certificates)
- ZeroSSL account and EAB credentials
- Cloudflare API token (for DNS-01 challenge)

## Step-by-Step Deployment

### Step 1: Install cert-manager

cert-manager is required for automatic SSL certificate management.

```bash


```

### Step 2: Set Environment Variables

```bash
export DOMAIN="yourdomain.com"
export ZEROSSL_EAB_KID="your_eab_kid"
export ZEROSSL_EAB_HMAC_KEY="your_eab_hmac_key"
export CLOUDFLARE_API_TOKEN="your_cloudflare_token"
```

### Step 3: Deploy All Components

Use the Quick Start commands at the top of this document.

## Configuration Details

### Database Configuration

The PostgreSQL database is configured with:
- **Database name**: `combfather`
- **User**: `postgres`
- **Password**: `postgres` (⚠️ Change in production!)
- **Persistent storage**: `/home/ubuntu/control-plane-psql`

### Control Plane Configuration

The control plane runs with:
- **Port**: 9900
- **JWT Secret**: `change-me-in-production` (⚠️ Change in production!)
- **Namespace**: `console`

### SSL Certificates

Certificates are automatically issued for:
- `console.${DOMAIN}` - Control plane
- `*.combinator.${DOMAIN}` - Combinator pods
- `*.s3.${DOMAIN}` - S3-compatible storage (future)

## Security Considerations

### ⚠️ IMPORTANT: Change Default Secrets

Before deploying to production, update these secrets:

1. **PostgreSQL password** in `control-plane-deployment.yaml`:
```yaml
env:
  - name: POSTGRES_PASSWORD
    value: "your-secure-password-here"
```

2. **JWT secret** in `control-plane-deployment.yaml`:
```yaml
stringData:
  jwt-secret: "your-secure-jwt-secret-here"
```

3. **Database connection string** in control plane deployment:
```yaml
env:
  - name: DATABASE_URL
    value: "host=postgres port=5432 user=postgres password=your-secure-password dbname=combfather sslmode=disable"
```

## Troubleshooting

### Pods not starting

Check pod logs:
```bash
kubectl logs -n console -l app=control-plane
kubectl logs -n console -l app=postgres
```

### Certificate not issued

Check certificate status:
```bash
kubectl describe certificate ingress-cert -n ingress
kubectl get certificaterequest -n ingress
```

Check cert-manager logs:
```bash
kubectl logs -n cert-manager -l app=cert-manager
```

### Database connection issues

Check if PostgreSQL is running:
```bash
kubectl exec -it -n console deployment/postgres -- psql -U postgres -d combfather -c "\dt"
```

### Control plane not accessible

Check service and ingress:
```bash
kubectl get svc -n console control-plane
kubectl get ingressroute -n ingress distributor
```

Test internal connectivity:
```bash
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- curl http://control-plane.console.svc.cluster.local:9900
```

## Updating the Deployment

### Update control plane image

```bash
# Build new image
docker build -t control-plane:latest .

# If using local registry, load into k3s
docker save control-plane:latest | sudo k3s ctr images import -

# Restart deployment
kubectl rollout restart deployment/control-plane -n console
```

### Update database schema

```bash
# Connect to PostgreSQL
kubectl exec -it -n console deployment/postgres -- psql -U postgres -d combfather

# Run your SQL commands
# ...
```

## Backup and Restore

### Backup PostgreSQL data

```bash
kubectl exec -n console deployment/postgres -- pg_dump -U postgres combfather > backup.sql
```

### Restore PostgreSQL data

```bash
cat backup.sql | kubectl exec -i -n console deployment/postgres -- psql -U postgres combfather
```

## Uninstalling

To remove all components:

```bash
kubectl delete -f control-plane-deployment.yaml
kubectl delete -f ingress.yaml
kubectl delete -f zerossl-issuer.yaml
```

To also remove cert-manager:

```bash
kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml
```

## API Endpoints

Once deployed, the following endpoints are available:

### Public Endpoints

- `POST /auth/register` - Register new user
- `POST /auth/login` - Login
- `POST /auth/send-code` - Send verification code
- `POST /auth/reset-password` - Reset password

### Protected Endpoints (require JWT token)

- `GET /api/rdb` - List RDB resources
- `POST /api/rdb` - Create RDB resource
- `DELETE /api/rdb/:id` - Delete RDB resource
- `GET /api/kv` - List KV resources
- `POST /api/kv` - Create KV resource
- `DELETE /api/kv/:id` - Delete KV resource
- `POST /api/combinator` - Create combinator pod
- `DELETE /api/combinator` - Delete combinator pod

## Next Steps

After deployment:

1. Access the web interface at `https://console.yourdomain.com`
2. Register a new account using the terminal interface
3. Create RDB and KV resources
4. Deploy combinator pods for data processing

## Support

For issues and questions:
- Check the troubleshooting section above
- Review pod logs for error messages
- Ensure all prerequisites are met
