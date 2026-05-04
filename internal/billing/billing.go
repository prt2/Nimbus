package billing

import (
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/prt2/nimbus/internal/kubeutil"
	"github.com/prt2/nimbus/internal/metrics"
)

// UsageRecord tracks how much a tenant has consumed in a billing period.
// CPU is tracked in millicore-seconds, memory in megabyte-seconds.
// These get converted to cost at invoice time.
type UsageRecord struct {
	// Running totals for the billing loop.
	TenantID        string
	TenantName      string
	CPUMilliSeconds int64 // total CPU milliseconds consumed
	MemoryMBSeconds int64 // total memory MB-seconds consumed
	PodCount        int   // number of pods running
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

// Pricing constants for the demo dashboard.
// These are intentionally exaggerated so short-lived workloads still
// produce readable invoice totals during local testing.
const (
	CPUCostPerMillicoreHour = 0.01 // $ per millicore per hour
	MemoryCostPerMBHour     = 0.05 // $ per MB per hour
)

// Engine scrapes pod resource usage per tenant namespace
// and accumulates billing data over time.
type Engine struct {
	// usage is what the API reads. usageStore persists the same numbers.
	k8s        kubernetes.Interface
	dynamic    dynamic.Interface
	usageStore UsageStore
	mu         sync.RWMutex
	usage      map[string]*UsageRecord // keyed by tenantID
	startTime  time.Time
}

type UsageStore interface {
	// Small interface so billing does not care how persistence is implemented.
	SaveUsage(tenantID string, cpuMS, memMBS int64, podCount int) error
}

func NewEngine(store UsageStore) (*Engine, error) {
	// Need the normal client plus metrics.k8s.io access.
	client, err := kubeutil.NewKubernetesClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	dynamicClient, err := kubeutil.NewDynamicClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Engine{
		k8s:        client,
		dynamic:    dynamicClient,
		usageStore: store,
		usage:      make(map[string]*UsageRecord),
		startTime:  time.Now(),
	}, nil
}

// Start begins the billing aggregation loop.
// Every 30 seconds it scans all nimbus namespaces,
// counts running pods, and accumulates usage.
func (e *Engine) Start(ctx context.Context) {
	fmt.Println("Billing engine starting...")
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
		fmt.Printf("Billing: failed to list namespaces: %v\n", err)
		return
	}

	for _, ns := range namespaces.Items {
		tenantID := ns.Labels["tenant-id"]
		if tenantID == "" {
			continue
		}
		tenantName := ns.Labels["tenant-name"]
		e.collectTenant(ctx, tenantID, tenantName, ns.Name)
	}
}

func (e *Engine) collectTenant(ctx context.Context, tenantID, tenantName, namespace string) {
	// Only count running pods here.
	pods, err := e.k8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "status.phase=Running",
	})
	if err != nil {
		return
	}

	podMetrics, err := kubeutil.ListPodMetrics(ctx, e.dynamic, namespace)
	if err != nil {
		fmt.Printf("Billing: failed to read pod metrics in %s: %v\n", namespace, err)
		return
	}

	metricByPod := make(map[string]kubeutil.PodMetric, len(podMetrics))
	for _, metric := range podMetrics {
		metricByPod[metric.Name] = metric
	}

	var cpuMillis int64
	var memoryBytes int64

	for _, pod := range pods.Items {
		// Skip pods that do not have metrics yet. We will catch them on the next tick.
		metric, ok := metricByPod[pod.Name]
		if !ok {
			continue
		}
		cpuMillis += metric.CPUMilli
		memoryBytes += metric.MemoryBytes
	}

	memoryMB := memoryBytes / (1024 * 1024)

	// Convert an instant metrics snapshot into the 30-second billing interval.
	const intervalSeconds = 30

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.usage[tenantID]; !ok {
		e.usage[tenantID] = &UsageRecord{TenantID: tenantID}
	}

	record := e.usage[tenantID]
	record.TenantName = tenantName
	record.CPUMilliSeconds += cpuMillis * intervalSeconds
	record.MemoryMBSeconds += memoryMB * intervalSeconds
	record.PodCount = len(pods.Items)
	record.LastUpdated = time.Now()

	if e.usageStore != nil {
		// Persist every sample so a restart does not wipe the totals.
		if err := e.usageStore.SaveUsage(tenantID, cpuMillis*intervalSeconds, memoryMB*intervalSeconds, len(pods.Items)); err != nil {
			fmt.Printf("Billing: failed to persist usage for %s: %v\n", tenantID, err)
		}
	}

	metrics.TenantPodsRunning.WithLabelValues(tenantID).Set(float64(len(pods.Items)))
	metrics.TenantCPUUsage.WithLabelValues(tenantID).Set(float64(cpuMillis))
	metrics.TenantMemoryUsage.WithLabelValues(tenantID).Set(float64(memoryMB))
	metrics.BillingCPUSeconds.WithLabelValues(tenantID).Add(float64(cpuMillis*intervalSeconds) / 1000)

	fmt.Printf("Billing sample for %s: pods=%d cpu=%dm memory=%dMB\n",
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

	// Return copies so callers cannot change shared records by accident.
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
		// Fresh tenants can exist before any usage shows up.
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

func currentPeriod() string {
	return time.Now().Format("2006-01")
}
