package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/prt2/nimbus/internal/controller"
	"github.com/prt2/nimbus/internal/scheduler"
	"github.com/prt2/nimbus/internal/tenant"
)

type server struct {
	tenants     *tenant.Store
	provisioner *tenant.Provisioner
}

func main() {
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

	// Both controller and scheduler run as background goroutines
	// This is the power of Go concurrency — they run truly in parallel
	ctx := context.Background()
	go ctrl.Start(ctx)
	go sched.Start(ctx)

	srv := &server{
		tenants:     tenant.NewStore(),
		provisioner: provisioner,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/tenants", srv.handleTenants)
	mux.HandleFunc("/tenants/", srv.handleTenantByID)

	httpServer := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
