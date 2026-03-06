# Nimbus

A Kubernetes-based multi-tenant cloud control plane built in Go. Nimbus lets you provision isolated compute environments through a REST API, enforce resource quotas per tenant, schedule pods using a custom bin-packing scheduler, autoscale deployments based on utilization, and track usage-based billing — similar in architecture to AWS ECS or Google Cloud Run.

---

## What Was Built

**Multi-Tenant Provisioning**
Each tenant gets a dedicated Kubernetes namespace with CPU, memory, and pod limits enforced via ResourceQuota. Tenants are created through the REST API and provisioned into the cluster automatically.

**Custom Scheduler**
Replaces the default Kubernetes scheduler with a bin-packing algorithm. Nodes are scored by existing CPU utilization — pods are placed on the busiest node first to maximize density before using idle capacity.

**Controller Reconcile Loop**
Watches `ComputeRequest` custom resources and automatically provisions the corresponding Deployment. Idempotent — runs every 10 seconds and only acts when state diverges from desired.

**Autoscaling Engine**
Monitors deployments every 30 seconds. Scales up when utilization exceeds 70%, scales down below 30%. Respects min/max replica bounds (1-5).

**Billing Engine**
Scrapes CPU and memory requests per tenant namespace every 30 seconds. Accumulates millicore-seconds and MB-seconds, then generates invoices with real cost calculations stored in PostgreSQL.

**JWT Authentication**
Every API endpoint is protected with JWT tokens signed with HS256. Tokens carry tenant identity and expire after 24 hours.

**React Dashboard**
A frontend dashboard showing live tenant status, workload pod counts, billing invoices, and scheduler decisions.

**Observability**
Prometheus metrics exposed at `/metrics` covering API request rates, latency histograms, per-tenant pod counts, CPU usage, billing totals, and autoscaler actions. Grafana dashboard included.

---

## Tech Stack

| Component | Technology |
|---|---|
| API server | Go |
| Kubernetes client | client-go |
| Database | PostgreSQL 15 |
| Authentication | JWT (golang-jwt) |
| Frontend | React, Recharts |
| Packaging | Helm |
| Metrics | Prometheus, Grafana |
| Local cluster | kind (Kubernetes in Docker) |

---

## Project Structure

```
nimbus/
├── cmd/
│   └── api/
│       └── main.go                        # Entry point
├── internal/
│   ├── tenant/
│   │   ├── tenant.go                      # Tenant model
│   │   ├── store_db.go                    # PostgreSQL store
│   │   └── provisioner.go                 # Namespace + quota provisioning
│   ├── controller/
│   │   └── computerequest_controller.go   # CRD reconcile loop
│   ├── scheduler/
│   │   └── scheduler.go                   # Custom bin-packing scheduler
│   ├── autoscaler/
│   │   └── autoscaler.go                  # Scale up/down loop
│   ├── billing/
│   │   └── billing.go                     # Usage tracking and invoicing
│   ├── metrics/
│   │   └── metrics.go                     # Prometheus metric definitions
│   └── middleware/
│       ├── auth.go                        # JWT middleware
│       └── cors.go                        # CORS middleware
├── k8s/
│   ├── crds/
│   │   └── computerequest.yaml            # ComputeRequest CRD
│   ├── monitoring/
│   │   ├── servicemonitor.yaml            # Prometheus scrape config
│   │   └── grafana-dashboard.yaml         # Grafana dashboard ConfigMap
│   └── helm/
│       └── nimbus/                        # Helm chart
└── frontend/                              # React dashboard
```

---

## Running Locally

### Prerequisites

- Go 1.21+
- Docker Desktop (running)
- kind
- kubectl
- Helm
- Node.js 18+
- PostgreSQL (local install or Docker)

### 1. Create a local Kubernetes cluster

```bash
kind create cluster --name nimbus
kubectl apply -f k8s/crds/computerequest.yaml
```

### 2. Start PostgreSQL

If you have PostgreSQL installed locally:

```bash
psql postgresql://localhost:5432/postgres -c "CREATE USER nimbus WITH PASSWORD 'nimbus123';"
psql postgresql://localhost:5432/postgres -c "CREATE DATABASE nimbus OWNER nimbus;"
```

Or via Docker (use port 5433 to avoid conflicts):

```bash
docker run -d \
  --name nimbus-postgres \
  -e POSTGRES_USER=nimbus \
  -e POSTGRES_PASSWORD=nimbus123 \
  -e POSTGRES_DB=nimbus \
  -p 5433:5432 \
  postgres:15
```

### 3. Run the API server

```bash
go mod tidy
go run cmd/api/main.go
```

Expected output:

```
Connected to PostgreSQL
ComputeRequest controller starting...
Nimbus custom scheduler starting...
Billing engine starting...
Autoscaler starting...
Nimbus API server starting on port 8080...
```

### 4. Run the frontend

```bash
cd frontend
npm install
npm start
```

Dashboard available at `http://localhost:3000`

### 5. Install monitoring (optional)

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install monitoring prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --set grafana.adminPassword=nimbus123

kubectl apply -f k8s/monitoring/servicemonitor.yaml
kubectl apply -f k8s/monitoring/grafana-dashboard.yaml

kubectl port-forward -n monitoring svc/monitoring-grafana 3002:80
```

Grafana available at `http://localhost:3002` — login with `admin / nimbus123`

---

## API Reference

### Authentication

```bash
POST /auth/token
Content-Type: application/json

{ "tenant_id": "admin", "tenant_name": "admin" }
```

Returns a JWT token. Pass it as `Authorization: Bearer <token>` on all other requests.

### Tenants

```bash
GET    /tenants           # List all tenants
POST   /tenants           # Create tenant
GET    /tenants/:id       # Get tenant by ID
DELETE /tenants/:id       # Delete tenant and deprovision namespace
```

### Billing

```bash
GET /billing/usage              # Current usage across all tenants
GET /billing/invoice/:tenant    # Generate invoice for a tenant
```

### Metrics

```bash
GET /metrics    # Prometheus metrics (no auth required)
```

### Deploying a Workload

Workloads are submitted as `ComputeRequest` custom resources via kubectl:

```bash
kubectl apply -f - <<EOF
apiVersion: nimbus.io/v1
kind: ComputeRequest
metadata:
  name: my-service
  namespace: nimbus-<tenant-name>
spec:
  tenantId: <tenant-name>
  image: nginx
  cpu: "200m"
  memory: "256Mi"
  replicas: 1
EOF
```

The controller picks this up within 10 seconds and creates the corresponding Deployment.

---

## Deploy with Helm

```bash
helm install nimbus k8s/helm/nimbus
helm upgrade nimbus k8s/helm/nimbus --set image.tag=v2
helm uninstall nimbus
```