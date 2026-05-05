# Nimbus

Nimbus is a small Go app for running workloads for different users or teams on Kubernetes. Each team, called a **tenant**, gets its own namespace: a separate area for its apps, with CPU and memory limits.

You can use the React dashboard to create tenants, see which apps are running, and view resource usage and estimated costs. Nimbus chooses where apps run and adjusts the number of running copies as resource use changes. PostgreSQL stores tenant details and usage samples.

## Run locally

You'll need:

- Go 1.25 or newer
- Node.js 22 and npm (the frontend Docker image also uses Node 22)
- Docker running, with Docker Compose available
- `kubectl` and [kind](https://kind.sigs.k8s.io/) for a local Kubernetes cluster

Run the commands below from the project folder unless a step says otherwise.

### 1. Prepare Kubernetes

Create a local cluster. If you already have one, skip the first command and check that `kubectl` points to it.

```bash
kind create cluster --name nimbus
kubectl get nodes
kubectl apply -f k8s/crds/computerequest.yaml
```

The last command teaches Kubernetes about `ComputeRequest`, the file format Nimbus uses to request an app.

Install Metrics Server so Nimbus can read CPU and memory usage. Skip this if it is already installed.

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

For a local kind cluster, add this setting to let Metrics Server connect without checking the node's certificate. Use it only for local development.

```bash
kubectl patch deployment metrics-server -n kube-system --type=json \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
```

Wait for it to start, then check that usage readings are available:

```bash
kubectl rollout status deployment/metrics-server -n kube-system --timeout=120s
kubectl top nodes
```

Readings can take a minute to appear. For an existing cluster, check the [Metrics Server requirements and supported Kubernetes versions](https://github.com/kubernetes-sigs/metrics-server#requirements).

### 2. Start the database

```bash
docker compose up -d postgres
docker compose exec postgres pg_isready -U nimbus -d nimbus
```

Wait until the second command reports that Postgres is accepting connections. This creates the database with the login details the API expects by default. Its data is kept in a Docker volume between restarts.

### 3. Start the API

```bash
go run ./cmd/api
```

Leave this terminal open. The API creates its database tables on startup and listens at `http://localhost:8080`.

In another terminal, check that it responds:

```bash
curl http://localhost:8080/health
```

You should see `{"status":"ok"}`.

### 4. Start the dashboard

In another terminal:

```bash
cd frontend
npm ci
npm start
```

Open [localhost:3000](http://localhost:3000). The dashboard connects to the local API and refreshes every 15 seconds.

## Try an app

In the dashboard, open **Tenants** and create a tenant named `tenant-a`. Keep the default CPU limit (`500m`, or half a CPU core) and memory limit (`512Mi`, or 512 MiB).

Creating a tenant sets up its namespace and limits. To run an app inside it, paste this command into a terminal:

```bash
kubectl apply -f - <<'YAML'
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
YAML
```

This asks Nimbus to run one copy of nginx with 0.2 CPU cores and 256 MiB of memory. In this request, `tenantId` must match the tenant's **name** (`tenant-a`), even though the field is called an ID.

Nimbus checks for new requests every 10 seconds. Check the result with:

```bash
kubectl get deployments,pods -n nimbus-tenant-a -o wide
```

Once the pod is running, the dashboard should start showing activity. Usage and invoice amounts update after a billing sample, usually within 30–60 seconds. The example runs nginx inside the cluster; it does not create a public web address.

To remove the example, delete `tenant-a` from the dashboard. This also deletes its namespace and everything inside it.

## If something isn't working

- **The dashboard shows a connection error:** check that the API is running and that `/health` responds.
- **The API cannot connect to Postgres:** check that the database is accepting connections and port 5432 is free.
- **The API cannot connect to Kubernetes:** run `kubectl get nodes` and check your selected cluster. Nimbus uses `KUBECONFIG` when set, otherwise in-cluster credentials or `~/.kube/config`.
- **A tenant shows no activity:** make sure you created a `ComputeRequest` and its pod is running. A tenant alone has no apps.
- **Usage stays empty:** run `kubectl top pods -n nimbus-tenant-a`. Nimbus needs Metrics Server readings for usage, scaling, and node scores.
- **A pod won't start:** run `kubectl describe pod -n nimbus-tenant-a <pod-name>` to see the reason, such as an image download error or a resource limit.

## Settings

The local setup works with these API defaults. Set environment variables before starting the API to change them.

| Variable | Default | Purpose |
| --- | --- | --- |
| `DB_HOST` | `127.0.0.1` | Database address |
| `DB_PORT` | `5432` | Database port |
| `DB_NAME` | `nimbus` | Database name |
| `DB_USER` | `nimbus` | Database user |
| `DB_PASSWORD` | `nimbus123` | Database password |
| `DB_SSLMODE` | `disable` | Database connection encryption setting |
| `JWT_SECRET` | Built-in demo key | Key used to sign login tokens |
| `KUBECONFIG` | See connection order above | Kubernetes connection file |

The dashboard uses `http://localhost:8080` for the API. Set `REACT_APP_API_URL` before starting or building the frontend to use another address.

## API reference

The dashboard requests an admin token automatically. For direct API calls, get one with:

```bash
curl -s http://localhost:8080/auth/token \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"admin","tenant_name":"admin"}'
```

Copy the returned token and send it as `Authorization: Bearer <token>` on protected routes.

| Route | Purpose |
| --- | --- |
| `GET /health` | Check that the API is running |
| `POST /auth/token` | Create a login token |
| `GET /tenants` | List tenants |
| `POST /tenants` | Create a tenant with `name`, `cpu_quota`, and `memory_quota` |
| `GET /tenants/:id` | Get one tenant by its generated ID |
| `DELETE /tenants/:id` | Delete a tenant and its namespace |
| `GET /billing/usage` | Get accumulated usage |
| `GET /billing/invoice/:tenant` | Get an estimated invoice by tenant ID or name |
| `GET /scheduler/status` | See pod placements and node scores |
| `GET /metrics` | Read Prometheus metrics |

Health, token creation, and metrics are public. Creating and deleting tenants requires an admin token.

## Project files

```text
cmd/api/               Starts the API and background tasks
internal/tenant/       Tenant records and namespace setup
internal/controller/   Turns app requests into running deployments
internal/scheduler/    Chooses a node for each waiting pod
internal/autoscaler/   Adjusts the number of app copies
internal/billing/      Collects usage and calculates costs
internal/middleware/   Checks tokens and handles browser access
internal/kubeutil/     Shared Kubernetes connection and metrics helpers
internal/metrics/      Defines Prometheus measurements
frontend/              React dashboard
k8s/                   Kubernetes setup, Helm chart, and monitoring files
```

## Current limits

This is a local demo. The token endpoint accepts tenant details without a password, and the dashboard uses an admin token. Real authentication is needed before sharing it as a hosted service.

Invoice amounts use demo prices. Usage samples are saved to Postgres, but the API's usage and invoice totals start over when it restarts.

The scheduler favors nodes that already have higher CPU use. It skips nodes marked unavailable for scheduling, but does not check whether a pod's resources fit. Scaling checks run every 30 seconds and keep apps between one and five copies. Editing a `ComputeRequest` does not update an existing deployment.
