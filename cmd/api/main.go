package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

const dbConnStr = "postgresql://nimbus:nimbus123@127.0.0.1:5432/nimbus?sslmode=disable"

type server struct {
	tenants     *tenant.DBStore
	provisioner *tenant.Provisioner
	billing     *billing.Engine
}

func main() {
	dbStore, err := tenant.NewDBStore(dbConnStr)
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

	billingEngine, err := billing.NewEngine()
	if err != nil {
		log.Fatalf("Failed to create billing engine: %v", err)
	}

	autoScaler, err := autoscaler.NewAutoscaler()
	if err != nil {
		log.Fatalf("Failed to create autoscaler: %v", err)
	}

	ctx := context.Background()
	go ctrl.Start(ctx)
	go sched.Start(ctx)
	go billingEngine.Start(ctx)
	go autoScaler.Start(ctx)

	srv := &server{
		tenants:     dbStore,
		provisioner: provisioner,
		billing:     billingEngine,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/auth/token", srv.handleToken)
	mux.HandleFunc("/tenants", srv.handleTenants)
	mux.HandleFunc("/tenants/", srv.handleTenantByID)
	mux.HandleFunc("/billing/usage", srv.handleUsage)
	mux.HandleFunc("/billing/invoice/", srv.handleInvoice)
	mux.Handle("/metrics", promhttp.Handler())

	handler := middleware.CORS(middleware.Auth(metricsMiddleware(mux)))

	httpServer := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	fmt.Println("🚀 Nimbus API server starting on port 8080...")
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleToken(w http.ResponseWriter, r *http.Request) {
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

func (s *server) handleTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenants := s.tenants.List()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"tenants": tenants,
			"count":   len(tenants),
		})
	case http.MethodPost:
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
		if err := s.provisioner.Provision(context.Background(), t); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, t)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *server) handleTenantByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/tenants/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing tenant id"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		t, err := s.tenants.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, t)
	case http.MethodDelete:
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

func (s *server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, s.billing.GetAllUsage())
}

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
	writeJSON(w, http.StatusOK, s.billing.GenerateInvoice(tenantID))
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func metricsMiddleware(next http.Handler) http.Handler {
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

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}
