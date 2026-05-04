// Defines and registers the measurements exposed by the API at /metrics.
// Counters add up events, gauges show current values, and histograms group request times.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// API request counter — tracks every HTTP request by method, path, status
	APIRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nimbus_api_requests_total",
			Help: "Total number of API requests",
		},
		[]string{"method", "path", "status"},
	)

	// API latency histogram — tracks how long requests take
	APIRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "nimbus_api_request_duration_seconds",
			Help:    "API request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// Tenant counter — how many tenants exist
	TenantsTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "nimbus_tenants_total",
			Help: "Total number of active tenants",
		},
	)

	// Per-tenant pod count
	TenantPodsRunning = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nimbus_tenant_pods_running",
			Help: "Number of running pods per tenant",
		},
		[]string{"tenant"},
	)

	// Per-tenant CPU usage in millicores
	TenantCPUUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nimbus_tenant_cpu_millicores",
			Help: "CPU usage in millicores per tenant",
		},
		[]string{"tenant"},
	)

	// Per-tenant memory usage in MB
	TenantMemoryUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nimbus_tenant_memory_mb",
			Help: "Memory usage in MB per tenant",
		},
		[]string{"tenant"},
	)

	// Scheduler decisions counter
	SchedulerDecisions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nimbus_scheduler_decisions_total",
			Help: "Total scheduling decisions made",
		},
		[]string{"node", "result"},
	)

	// Autoscaler actions
	AutoscalerActions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nimbus_autoscaler_actions_total",
			Help: "Total autoscaler scale up/down actions",
		},
		[]string{"namespace", "direction"},
	)

	// Billing usage tracked
	BillingCPUSeconds = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nimbus_billing_cpu_seconds_total",
			Help: "Total CPU seconds billed per tenant",
		},
		[]string{"tenant"},
	)
)
