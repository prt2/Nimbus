// Starts the API and background workers, then connects each HTTP route to its handler.
// Flow: read settings -> connect to Postgres and Kubernetes -> start workers -> serve requests.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prt2/nimbus/internal/autoscaler"
	"github.com/prt2/nimbus/internal/billing"
	"github.com/prt2/nimbus/internal/controller"
	"github.com/prt2/nimbus/internal/metrics"
	"github.com/prt2/nimbus/internal/middleware"
	"github.com/prt2/nimbus/internal/scheduler"
	"github.com/prt2/nimbus/internal/tenant"
)

type server struct {
	// Keep the shared services here so handlers can stay thin.
	tenants     *tenant.DBStore
	provisioner *tenant.Provisioner
	billing     *billing.Engine
	scheduler   *scheduler.Scheduler
}

// main connects the services and starts the HTTP server and periodic workers.
func main() {
	// Load shared config first, then build the pieces that need DB/K8s access.
	middleware.SetJWTSecret(os.Getenv("JWT_SECRET"))

	dbStore, err := tenant.NewDBStore(dbConnStr())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	provisioner, err := tenant.NewProvisioner()
	if err != nil {
		log.Fatalf("Failed to connect to Kubernetes: %v", err)
	}

	ctrl, err := controller.NewController()
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}

	sched, err := scheduler.NewScheduler()
	if err != nil {
		log.Fatalf("Failed to create scheduler: %v", err)
	}

	billingEngine, err := billing.NewEngine(dbStore)
	if err != nil {
		log.Fatalf("Failed to create billing engine: %v", err)
	}

	autoScaler, err := autoscaler.NewAutoscaler()
	if err != nil {
		log.Fatalf("Failed to create autoscaler: %v", err)
	}

	ctx := context.Background()
	// These run in the background the whole time the API is up.
	go ctrl.Start(ctx)
	go sched.Start(ctx)
	go billingEngine.Start(ctx)
	go autoScaler.Start(ctx)

	srv := &server{
		tenants:     dbStore,
		provisioner: provisioner,
		billing:     billingEngine,
		scheduler:   sched,
	}

	mux := http.NewServeMux()
	// Small API surface for the dashboard and local testing.
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/auth/token", srv.handleToken)
	mux.HandleFunc("/tenants", srv.handleTenants)
	mux.HandleFunc("/tenants/", srv.handleTenantByID)
	mux.HandleFunc("/billing/usage", srv.handleUsage)
	mux.HandleFunc("/billing/invoice/", srv.handleInvoice)
	mux.HandleFunc("/scheduler/status", srv.handleSchedulerStatus)
	mux.Handle("/metrics", promhttp.Handler())

	// CORS first for the frontend, auth around the protected routes, metrics last.
	handler := middleware.CORS(middleware.Auth(metricsMiddleware(mux)))

	httpServer := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	fmt.Println("Nimbus API server starting on port 8080...")
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// healthHandler returns a simple response showing that the API is reachable.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleToken reads a tenant ID and name and returns a signed token for them.
func (s *server) handleToken(w http.ResponseWriter, r *http.Request) {
	// Frontend grabs a token here before it does anything else.
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		TenantID   string `json:"tenant_id"`
		TenantName string `json:"tenant_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	token, err := middleware.GenerateToken(req.TenantID, req.TenantName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// handleTenants lists visible tenants or lets an admin create a tenant and its namespace.
func (s *server) handleTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Dashboard hits this a lot, so keep the response simple.
		tenants := s.tenants.List()
		// Non-admin callers only see their own tenant record.
		if !middleware.IsAdmin(r.Context()) {
			tenantID, err := middleware.TenantIDFromContext(r.Context())
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
				return
			}
			tenants = filterTenantsByID(tenants, tenantID)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"tenants": tenants,
			"count":   len(tenants),
		})
	case http.MethodPost:
		// Create the DB row first, then make the namespace/quota in the cluster.
		if !middleware.IsAdmin(r.Context()) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "only admin can create tenants"})
			return
		}
		var req struct {
			Name        string `json:"name"`
			CPUQuota    string `json:"cpu_quota"`
			MemoryQuota string `json:"memory_quota"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		if req.CPUQuota == "" {
			req.CPUQuota = "500m"
		}
		if req.MemoryQuota == "" {
			req.MemoryQuota = "512Mi"
		}
		t, err := s.tenants.Create(req.Name, req.CPUQuota, req.MemoryQuota)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		// Roll back the DB row if the cluster step fails.
		if err := s.provisioner.Provision(context.Background(), t); err != nil {
			_ = s.tenants.Delete(t.ID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, t)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleTenantByID returns one permitted tenant or lets an admin delete it.
func (s *server) handleTenantByID(w http.ResponseWriter, r *http.Request) {
	// Everything after /tenants/ is treated as the tenant ID.
	id := strings.TrimPrefix(r.URL.Path, "/tenants/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing tenant id"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		// Fetch first, then check whether this caller can see it.
		t, err := s.tenants.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if !canAccessTenant(r.Context(), t) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		writeJSON(w, http.StatusOK, t)
	case http.MethodDelete:
		// Tear down cluster state first, then drop the tenant row.
		if !middleware.IsAdmin(r.Context()) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "only admin can delete tenants"})
			return
		}
		t, err := s.tenants.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if err := s.provisioner.Deprovision(context.Background(), t); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.tenants.Delete(id)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleUsage returns accumulated usage, limited to the caller’s tenant unless they are admin.
func (s *server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	// Admin can inspect the full platform view; tenant tokens are scoped.
	if middleware.IsAdmin(r.Context()) {
		writeJSON(w, http.StatusOK, s.billing.GetAllUsage())
		return
	}
	tenantID, err := middleware.TenantIDFromContext(r.Context())
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	record := s.billing.GetUsage(tenantID)
	if record == nil {
		// No usage yet is normal for a fresh tenant.
		writeJSON(w, http.StatusOK, map[string]*billing.UsageRecord{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]*billing.UsageRecord{tenantID: record})
}

// handleInvoice resolves a tenant ID or name, checks access, and calculates its invoice.
func (s *server) handleInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	tenantID := strings.TrimPrefix(r.URL.Path, "/billing/invoice/")
	if tenantID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing tenant id"})
		return
	}
	// Accept either tenant ID or name here. Makes the UI and curl calls easier.
	if t, err := s.tenants.Get(tenantID); err == nil {
		tenantID = t.ID
	} else if t, err := s.tenants.GetByName(tenantID); err == nil {
		tenantID = t.ID
	}
	if !middleware.IsAdmin(r.Context()) {
		tokenTenantID, err := middleware.TenantIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		if tokenTenantID != tenantID {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
	}
	writeJSON(w, http.StatusOK, s.billing.GenerateInvoice(tenantID))
}

// handleSchedulerStatus returns cached scheduler information and filters the pod list for tenants.
func (s *server) handleSchedulerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	status := s.scheduler.GetStatus()
	if middleware.IsAdmin(r.Context()) {
		writeJSON(w, http.StatusOK, status)
		return
	}

	// Tenant tokens should only see their own scheduler view.
	tenantID, err := middleware.TenantIDFromContext(r.Context())
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	filteredPods := make([]scheduler.ScheduledPod, 0, len(status.Pods))
	for _, pod := range status.Pods {
		if pod.TenantID == tenantID {
			filteredPods = append(filteredPods, pod)
		}
	}
	status.Pods = filteredPods
	writeJSON(w, http.StatusOK, status)
}

// writeJSON sends an HTTP status and encodes the response body as JSON.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	// Small helper so handlers do not repeat the JSON boilerplate.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// metricsMiddleware measures how long each handled request takes and records its response status.
func metricsMiddleware(next http.Handler) http.Handler {
	// Track request count and latency for every route.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(rw.status)
		metrics.APIRequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
		metrics.APIRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader remembers the status code before forwarding it to the real response writer.
func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

// dbConnStr builds a Postgres URL from environment settings, using local defaults when unset.
func dbConnStr() string {
	host := getenvDefault("DB_HOST", "127.0.0.1")
	port := getenvDefault("DB_PORT", "5432")
	name := getenvDefault("DB_NAME", "nimbus")
	user := getenvDefault("DB_USER", "nimbus")
	password := getenvDefault("DB_PASSWORD", "nimbus123")
	sslmode := getenvDefault("DB_SSLMODE", "disable")

	// Keep the connection string env-driven so local runs, Docker, and Helm
	// all use the same code path.
	return (&url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(user, password),
		Host:   host + ":" + port,
		Path:   name,
		RawQuery: url.Values{
			"sslmode": []string{sslmode},
		}.Encode(),
	}).String()
}

// getenvDefault uses a fallback when an environment variable is empty or missing.
func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// filterTenantsByID keeps only records whose ID matches the caller’s tenant.
func filterTenantsByID(tenants []*tenant.Tenant, tenantID string) []*tenant.Tenant {
	// Used when a tenant-scoped token should only see one record.
	filtered := make([]*tenant.Tenant, 0, 1)
	for _, t := range tenants {
		if t.ID == tenantID {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// canAccessTenant allows an admin or the owner of this tenant to view the record.
func canAccessTenant(ctx context.Context, t *tenant.Tenant) bool {
	// Admin sees everything. Tenant tokens only get their own row.
	if middleware.IsAdmin(ctx) {
		return true
	}
	tenantID, err := middleware.TenantIDFromContext(ctx)
	return err == nil && tenantID == t.ID
}
