// Stores tenant details and accumulated usage in PostgreSQL.
// Flow: connect -> create missing tables -> read or write records using SQL parameters.

package tenant

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type DBStore struct {
	db *sql.DB
}

// NewDBStore connects to Postgres, checks the connection, and creates missing tables.
func NewDBStore(connStr string) (*DBStore, error) {
	// sql.Open is lazy, so ping here and fail fast if Postgres is down.
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to db: %w", err)
	}
	store := &DBStore{db: db}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate: %w", err)
	}
	fmt.Println("Connected to PostgreSQL")
	return store, nil
}

// migrate creates the tenant and usage tables if they do not already exist.
func (s *DBStore) migrate() error {
	// Keep bootstrap simple for local runs. Create tables if missing.
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tenants (
			id           TEXT PRIMARY KEY,
			name         TEXT UNIQUE NOT NULL,
			namespace    TEXT NOT NULL,
			cpu_quota    TEXT NOT NULL,
			memory_quota TEXT NOT NULL,
			created_at   TIMESTAMP NOT NULL
		);
		CREATE TABLE IF NOT EXISTS usage_records (
			tenant_id          TEXT PRIMARY KEY,
			cpu_milli_seconds  BIGINT DEFAULT 0,
			memory_mb_seconds  BIGINT DEFAULT 0,
			pod_count          INT DEFAULT 0,
			last_updated       TIMESTAMP
		);
	`)
	return err
}

// Create assigns a unique ID and inserts a tenant with its quotas and namespace name.
func (s *DBStore) Create(name, cpuQuota, memoryQuota string) (*Tenant, error) {
	// Namespace naming stays predictable so the rest of the code can derive it.
	t := &Tenant{
		ID:          uuid.NewString(),
		Name:        name,
		Namespace:   "nimbus-" + name,
		CPUQuota:    cpuQuota,
		MemoryQuota: memoryQuota,
		CreatedAt:   time.Now(),
	}
	_, err := s.db.Exec(`
		INSERT INTO tenants (id, name, namespace, cpu_quota, memory_quota, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, t.Name, t.Namespace, t.CPUQuota, t.MemoryQuota, t.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("tenant already exists or db error: %w", err)
	}
	return t, nil
}

// Get loads one tenant by its unique ID.
func (s *DBStore) Get(id string) (*Tenant, error) {
	// Normal lookup path from the API.
	t := &Tenant{}
	err := s.db.QueryRow(`
		SELECT id, name, namespace, cpu_quota, memory_quota, created_at
		FROM tenants WHERE id = $1`, id).
		Scan(&t.ID, &t.Name, &t.Namespace, &t.CPUQuota, &t.MemoryQuota, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tenant not found")
	}
	return t, err
}

// GetByName loads one tenant by its human-readable name.
func (s *DBStore) GetByName(name string) (*Tenant, error) {
	// Handy for endpoints that accept either ID or name.
	t := &Tenant{}
	err := s.db.QueryRow(`
		SELECT id, name, namespace, cpu_quota, memory_quota, created_at
		FROM tenants WHERE name = $1`, name).
		Scan(&t.ID, &t.Name, &t.Namespace, &t.CPUQuota, &t.MemoryQuota, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tenant not found")
	}
	return t, err
}

// List reads all tenants with the newest records first.
func (s *DBStore) List() []*Tenant {
	// Newest first reads a little better in the dashboard.
	rows, err := s.db.Query(`
		SELECT id, name, namespace, cpu_quota, memory_quota, created_at
		FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var tenants []*Tenant
	for rows.Next() {
		t := &Tenant{}
		rows.Scan(&t.ID, &t.Name, &t.Namespace, &t.CPUQuota, &t.MemoryQuota, &t.CreatedAt)
		tenants = append(tenants, t)
	}
	return tenants
}

// Delete removes a tenant row and reports an error if the ID was absent.
func (s *DBStore) Delete(id string) error {
	// By the time this runs, the namespace should already be gone.
	res, err := s.db.Exec(`DELETE FROM tenants WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}

// SaveUsage inserts the first usage sample or adds the sample to existing totals.
func (s *DBStore) SaveUsage(tenantID string, cpuMS, memMBS int64, podCount int) error {
	// Billing samples stack up over time, so add to the total instead of replacing it.
	_, err := s.db.Exec(`
		INSERT INTO usage_records (tenant_id, cpu_milli_seconds, memory_mb_seconds, pod_count, last_updated)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id) DO UPDATE SET
			cpu_milli_seconds = usage_records.cpu_milli_seconds + EXCLUDED.cpu_milli_seconds,
			memory_mb_seconds = usage_records.memory_mb_seconds + EXCLUDED.memory_mb_seconds,
			pod_count         = EXCLUDED.pod_count,
			last_updated      = EXCLUDED.last_updated`,
		tenantID, cpuMS, memMBS, podCount, time.Now(),
	)
	return err
}
