package billing

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"path/filepath"
)

// UsageRecord tracks how much a tenant has consumed in a billing period.
// CPU is tracked in millicore-seconds, memory in megabyte-seconds.
// These get converted to cost at invoice time.
type UsageRecord struct {
	TenantID        string
	CPUMilliSeconds int64   // total CPU milliseconds consumed
	MemoryMBSeconds int64   // total memory MB-seconds consumed
	PodCount        int     // number of pods running
	LastUpdated     time.Time
}

// Invoice is generated from usage records at billing time.
type Invoice struct {
	TenantID    string
	Period      string
	CPUCost     float64
	MemoryCost  float64
	TotalCost   float64
	GeneratedAt time.Time
}

// Pricing constants — fake money but real math
const (
	CPUCostPerMillicoreHour  = 0.00001  // $ per millicore per hour
	MemoryCostPerMBHour      = 0.000001 // $ per MB per hour
)

// Engine scrapes pod resource usage per tenant namespace
// and accumulates billing data over time.
type Engine struct {
	k8s     kubernetes.Interface
	mu      sync.RWMutex
	usage   map[string]*UsageRecord // keyed by tenantID
	startTime time.Time
}

func NewEngine() (*Engine, error) {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	return &Engine{
		k8s:       client,
		usage:     make(map[string]*UsageRecord),
		startTime: time.Now(),
	}, nil
}

// Start begins the billing aggregation loop.
// Every 30 seconds it scans all nimbus namespaces,
// counts running pods, and accumulates usage.
func (e *Engine) Start(ctx context.Context) {
	fmt.Println("💰 Billing engine starting...")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	e.collect(ctx)
	for {
		select {
		case <-ticker.C:
			e.collect(ctx)
		case <-ctx.Done():
			fmt.Println("Billing engine shutting down...")
			return
		}
	}
}

// collect scans all nimbus-* namespaces and accumulates usage.
// This runs every 30 seconds — each tick adds 30 seconds worth
// of resource consumption to each tenant's running total.
func (e *Engine) collect(ctx context.Context) {
	// Find all nimbus tenant namespaces
	namespaces, err := e.k8s.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "managed-by=nimbus",
	})
	if err != nil {
		fmt.Printf("❌ Billing: failed to list namespaces: %v\n", err)
		return
	}

	for _, ns := range namespaces.Items {
		tenantID := ns.Labels["tenant-name"]
		if tenantID == "" {
			continue
		}
		e.collectTenant(ctx, tenantID, ns.Name)
	}
}

func (e *Engine) collectTenant(ctx context.Context, tenantID, namespace string) {
	pods, err := e.k8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "status.phase=Running",
	})
	if err != nil {
		return
	}

	var cpuMillis int64
	var memoryMB int64

	for _, pod := range pods.Items {
		cpuMillis += podCPUMillis(pod)
		memoryMB += podMemoryMB(pod)
	}

	// 30 seconds of usage — multiply resource by time interval
	const intervalSeconds = 30

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.usage[tenantID]; !ok {
		e.usage[tenantID] = &UsageRecord{TenantID: tenantID}
	}

	record := e.usage[tenantID]
	record.CPUMilliSeconds += cpuMillis * intervalSeconds
	record.MemoryMBSeconds += memoryMB * intervalSeconds
	record.PodCount = len(pods.Items)
	record.LastUpdated = time.Now()

	fmt.Printf("💰 [%s] pods=%d cpu=%dm memory=%dMB accumulated\n",
		tenantID, len(pods.Items), cpuMillis, memoryMB)
}

// GetUsage returns current usage for a tenant.
func (e *Engine) GetUsage(tenantID string) *UsageRecord {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.usage[tenantID]
}

// GetAllUsage returns usage for all tenants.
func (e *Engine) GetAllUsage() map[string]*UsageRecord {
	e.mu.RLock()
	defer e.mu.RUnlock()

	copy := make(map[string]*UsageRecord)
	for k, v := range e.usage {
		record := *v
		copy[k] = &record
	}
	return copy
}

// GenerateInvoice converts accumulated usage into a dollar amount.
func (e *Engine) GenerateInvoice(tenantID string) *Invoice {
	e.mu.RLock()
	record, ok := e.usage[tenantID]
	e.mu.RUnlock()

	if !ok {
		return &Invoice{TenantID: tenantID, Period: currentPeriod()}
	}

	// Convert seconds to hours for pricing
	cpuHours := float64(record.CPUMilliSeconds) / 3600
	memoryHours := float64(record.MemoryMBSeconds) / 3600

	cpuCost := cpuHours * CPUCostPerMillicoreHour
	memoryCost := memoryHours * MemoryCostPerMBHour

	return &Invoice{
		TenantID:    tenantID,
		Period:      currentPeriod(),
		CPUCost:     cpuCost,
		MemoryCost:  memoryCost,
		TotalCost:   cpuCost + memoryCost,
		GeneratedAt: time.Now(),
	}
}

// podCPUMillis sums all container CPU requests for a pod in millicores.
func podCPUMillis(pod corev1.Pod) int64 {
	var total int64
	for _, c := range pod.Spec.Containers {
		if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			total += cpu.MilliValue()
		}
	}
	return total
}

// podMemoryMB sums all container memory requests for a pod in MB.
func podMemoryMB(pod corev1.Pod) int64 {
	var total int64
	for _, c := range pod.Spec.Containers {
		if mem, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			total += mem.Value() / (1024 * 1024)
		}
	}
	return total
}

func currentPeriod() string {
	return time.Now().Format("2006-01")
}
