package tenant

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Tenant represents a user/org on the Nimbus platform.
// Each tenant gets their own isolated namespace in Kubernetes.
type Tenant struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Namespace   string    `json:"namespace"`    // K8s namespace this tenant owns
	CPUQuota    string    `json:"cpu_quota"`    // e.g. "2000m" = 2 CPU cores
	MemoryQuota string    `json:"memory_quota"` // e.g. "2Gi"
	CreatedAt   time.Time `json:"created_at"`
}

// Store is an in-memory database of tenants.
// The sync.RWMutex makes it safe for concurrent reads/writes
// because multiple requests can come in at the same time.
type Store struct {
	mu      sync.RWMutex
	tenants map[string]*Tenant
}

func NewStore() *Store {
	return &Store{
		tenants: make(map[string]*Tenant),
	}
}

func (s *Store) Create(name, cpuQuota, memoryQuota string) (*Tenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate names
	for _, t := range s.tenants {
		if t.Name == name {
			return nil, errors.New("tenant with this name already exists")
		}
	}

	t := &Tenant{
		ID:          uuid.NewString(),
		Name:        name,
		Namespace:   "nimbus-" + name, // e.g. nimbus-acme-corp
		CPUQuota:    cpuQuota,
		MemoryQuota: memoryQuota,
		CreatedAt:   time.Now(),
	}

	s.tenants[t.ID] = t
	return t, nil
}

func (s *Store) Get(id string) (*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tenants[id]
	if !ok {
		return nil, errors.New("tenant not found")
	}
	return t, nil
}

func (s *Store) List() []*Tenant {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Tenant, 0, len(s.tenants))
	for _, t := range s.tenants {
		list = append(list, t)
	}
	return list
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tenants[id]; !ok {
		return errors.New("tenant not found")
	}
	delete(s.tenants, id)
	return nil
}
