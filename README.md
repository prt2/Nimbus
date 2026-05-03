# Nimbus

Nimbus is a small multi-tenant Kubernetes control plane written in Go.

It does four main things:

- Creates one Kubernetes namespace per tenant and applies resource quotas.
- Watches a `ComputeRequest` CRD and creates workloads for it.
- Schedules those workloads with a custom bin-packing scheduler.
- Tracks tenant CPU and memory usage for autoscaling and billing.

The repo also includes a React dashboard, Helm chart, Docker setup, and basic Prometheus/Grafana manifests.

## Stack

- Go
- Kubernetes
- PostgreSQL
- React
- Docker
- Helm
- Prometheus and Grafana

## Project Layout

```text
cmd/api/main.go                  API entrypoint
internal/tenant/                 tenant storage and namespace provisioning
internal/controller/             ComputeRequest controller
internal/scheduler/              custom scheduler
internal/autoscaler/             autoscaling loop
internal/billing/                usage collection and invoicing
internal/middleware/             JWT auth and CORS
internal/kubeutil/               shared Kubernetes and metrics helpers
frontend/                        React dashboard
k8s/crds/                        ComputeRequest CRD
k8s/helm/nimbus/                 Helm chart
k8s/monitoring/                  Prometheus and Grafana manifests
```

## How It Works

### Tenants

Tenants are stored in PostgreSQL.  
When a tenant is created through the API, Nimbus:

- writes the tenant record to Postgres
- creates a Kubernetes namespace
- applies a `ResourceQuota` for CPU, memory, and pod count

### Workloads

Workloads are defined through a `ComputeRequest` custom resource.

The controller checks for new requests every 10 seconds. If the matching Deployment does not exist yet, it creates one in the tenant namespace.

### Scheduler

Workloads created by the controller use `nimbus-scheduler`.

The scheduler checks for pending pods every 5 seconds. It scores nodes by live CPU utilization from `metrics.k8s.io` and picks the busiest schedulable node first. That gives the scheduler a bin-packing bias instead of spreading pods evenly.

### Autoscaling

The autoscaler checks Nimbus-managed Deployments every 30 seconds.

It compares live CPU and memory usage against the resources requested by the running pods:

- scale up at 70% or higher
- scale down at 30% or lower
- keep replicas between 1 and 5

### Billing

The billing engine collects live pod CPU and memory usage every 30 seconds.

It stores accumulated tenant usage in memory and also persists usage totals to PostgreSQL. Invoices are generated through the API from the accumulated usage records.

### Authentication

The API uses JWT tokens signed with HS256.

- `/auth/token` issues a token
- most API routes require `Authorization: Bearer <token>`
- admin tokens can view all tenants
- non-admin tokens are scoped to their own tenant

## Prerequisites

- Go 1.21+
- Node.js 18+
- Docker
- kubectl
- kind or another Kubernetes cluster
- PostgreSQL
- metrics-server installed in the cluster

## Run Locally

### 1. Start a cluster

```bash
kind create cluster --name nimbus
kubectl apply -f k8s/crds/computerequest.yaml
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

The metrics server is required for:

- scheduler node scoring
- autoscaling
- billing

### 2. Start PostgreSQL

If PostgreSQL is already installed locally:

```bash
psql postgresql://localhost:5432/postgres -c "CREATE USER nimbus WITH PASSWORD 'nimbus123';"
psql postgresql://localhost:5432/postgres -c "CREATE DATABASE nimbus OWNER nimbus;"
```

Or run it in Docker:

```bash
docker run -d \
  --name nimbus-postgres \
  -e POSTGRES_USER=nimbus \
  -e POSTGRES_PASSWORD=nimbus123 \
  -e POSTGRES_DB=nimbus \
  -p 5432:5432 \
  postgres:15
```

### 3. Start the API

```bash
go mod tidy
go run cmd/api/main.go
```

The API reads these environment variables if set:

- `DB_HOST`
- `DB_PORT`
- `DB_NAME`
- `DB_USER`
- `DB_PASSWORD`
- `DB_SSLMODE`
- `JWT_SECRET`
- `KUBECONFIG`

### 4. Start the frontend

```bash
cd frontend
npm install
npm start
```

Frontend URL:

```text
http://localhost:3000
```

## End-to-End Demo

This is the quickest way to make the dashboard show real activity.

### 1. Create an admin token

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/auth/token \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"admin","tenant_name":"admin"}' | jq -r .token)
```

### 2. Create a tenant

```bash
curl -s -X POST http://localhost:8080/tenants \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"tenant-a","cpu_quota":"500m","memory_quota":"512Mi"}'
```

Verify it:

```bash
kubectl get ns nimbus-tenant-a
kubectl get resourcequota -n nimbus-tenant-a
curl -s http://localhost:8080/tenants -H "Authorization: Bearer $TOKEN"
```

### 3. Create a workload for that tenant

```bash
kubectl apply -f - <<'EOF'
apiVersion: nimbus.io/v1
kind: ComputeRequest
metadata:
  name: web-server
  namespace: nimbus-tenant-a
spec:
  tenantId: tenant-a
  image: nginx
  cpu: "200m"
  memory: "256Mi"
  replicas: 1
EOF
```

Verify it:

```bash
kubectl get computerequest -n nimbus-tenant-a
kubectl get deployment -n nimbus-tenant-a
kubectl get pods -n nimbus-tenant-a -o wide
```

Expected result:

- the controller creates `deployment/nimbus-web-server`
- the pod starts in `nimbus-tenant-a`
- the pod uses `nimbus-scheduler`
- the scheduler binds it to a node

You can check the scheduler assignment directly:

```bash
kubectl get pod -n nimbus-tenant-a -o jsonpath='{range .items[*]}{.metadata.name}{"  scheduler="}{.spec.schedulerName}{"  node="}{.spec.nodeName}{"\n"}{end}'
```

### 4. Refresh the frontend

Open `http://localhost:3000` and click `Refresh`.

What should update:

- `Tenants`: `tenant-a` appears
- `Workloads`: tenant is no longer idle
- `Overview`: pod count is above 0
- `Scheduler`: scheduled pod, node score, and recent binding appear
- `Billing`: usage starts showing after the next billing loop

### 5. Wait for billing

The billing engine samples usage every 30 seconds.

After about 30 to 60 seconds:

```bash
curl -s http://localhost:8080/billing/usage \
  -H "Authorization: Bearer $TOKEN"

curl -s http://localhost:8080/billing/invoice/tenant-a \
  -H "Authorization: Bearer $TOKEN"
```

Expected result:

- `CPUMilliSeconds` and `MemoryMBSeconds` start increasing
- invoice totals become non-zero

### 6. Optional: watch it live

```bash
kubectl get pods -n nimbus-tenant-a -w
```

## What To Check In The UI

If everything is working, the dashboard should show:

- `Overview`: `Tenants = 1`, `Active Tenants = 1`, `Pods = 1`
- `Workloads`: `tenant-a` with `1 pod`
- `Scheduler`: one scheduled pod and one recent binding
- `Billing`: one usage record and a non-zero invoice after the billing loop runs

## Why The Dashboard Can Look Empty

If the tenant exists but the pages still show zeros, the usual reason is:

- you created a tenant, but not a workload

A tenant only creates:

- a Postgres record
- a Kubernetes namespace
- a `ResourceQuota`

It does not create a pod by itself.  
To make `Overview`, `Workloads`, `Scheduler`, and `Billing` show activity, you need a `ComputeRequest`.

## Run With Docker Compose

```bash
docker compose up --build
```

This starts:

- PostgreSQL
- the Go API
- the React frontend

The compose setup mounts your local kubeconfig into the API container so it can still talk to the cluster.

## API

### Get a token

```http
POST /auth/token
Content-Type: application/json
```

Example body:

```json
{
  "tenant_id": "admin",
  "tenant_name": "admin"
}
```

### Tenant routes

```text
GET    /tenants
POST   /tenants
GET    /tenants/:id
DELETE /tenants/:id
```

### Billing routes

```text
GET /billing/usage
GET /billing/invoice/:tenant
```

### Metrics

```text
GET /metrics
```

## Example ComputeRequest

```yaml
apiVersion: nimbus.io/v1
kind: ComputeRequest
metadata:
  name: my-service
  namespace: nimbus-acme
spec:
  tenantId: acme
  image: nginx
  cpu: "200m"
  memory: "256Mi"
  replicas: 1
```

Apply it with:

```bash
kubectl apply -f my-request.yaml
```

## Monitoring

If you want Prometheus and Grafana locally:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install monitoring prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --set grafana.adminPassword=nimbus123

kubectl apply -f k8s/monitoring/servicemonitor.yaml
kubectl apply -f k8s/monitoring/grafana-dashboard.yaml
```

## Helm

```bash
helm install nimbus k8s/helm/nimbus
helm upgrade nimbus k8s/helm/nimbus --set image.tag=v2
helm uninstall nimbus
```
