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

func NewDBStore(connStr string) (*DBStore, error) {
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
	fmt.Println("✅ Connected to PostgreSQL")
	return store, nil
}

func (s *DBStore) migrate() error {
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

func (s *DBStore) Create(name, cpuQuota, memoryQuota string) (*Tenant, error) {
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

func (s *DBStore) Get(id string) (*Tenant, error) {
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

func (s *DBStore) List() []*Tenant {
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

func (s *DBStore) Delete(id string) error {
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

func (s *DBStore) SaveUsage(tenantID string, cpuMS, memMBS int64, podCount int) error {
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
