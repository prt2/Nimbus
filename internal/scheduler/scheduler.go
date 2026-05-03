package scheduler

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/prt2/nimbus/internal/kubeutil"
	"github.com/prt2/nimbus/internal/metrics"
)

const SchedulerName = "nimbus-scheduler"

// Scheduler watches for unscheduled pods and places them on the best node.
// "Best" means the node with the highest live CPU utilization.
type Scheduler struct {
	k8s     kubernetes.Interface
	dynamic dynamic.Interface
	mu      sync.RWMutex
	status  StatusSnapshot
}

type NodeStatus struct {
	NodeName       string `json:"nodeName"`
	CPUUtilization int64  `json:"cpuUtilization"`
	Schedulable    bool   `json:"schedulable"`
}

type BindingEvent struct {
	PodName   string    `json:"podName"`
	Namespace string    `json:"namespace"`
	NodeName  string    `json:"nodeName"`
	Timestamp time.Time `json:"timestamp"`
}

type ScheduledPod struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	NodeName   string `json:"nodeName"`
	TenantID   string `json:"tenantId"`
	TenantName string `json:"tenantName"`
	Phase      string `json:"phase"`
}

type StatusSnapshot struct {
	LastRun        time.Time       `json:"lastRun"`
	PendingPods    int             `json:"pendingPods"`
	ScheduledPods  int             `json:"scheduledPods"`
	Pressure       int64           `json:"pressure"`
	Nodes          []NodeStatus    `json:"nodes"`
	RecentBindings []BindingEvent  `json:"recentBindings"`
	Pods           []ScheduledPod  `json:"pods"`
}

// NodeScore holds a node and its computed score.
// Higher score = better candidate for scheduling.
type NodeScore struct {
	NodeName string
	Score    int64
}

func NewScheduler() (*Scheduler, error) {
	client, err := kubeutil.NewKubernetesClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	dynamicClient, err := kubeutil.NewDynamicClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Scheduler{
		k8s:     client,
		dynamic: dynamicClient,
		status:  StatusSnapshot{},
	}, nil
}

// Start begins the scheduling loop.
// Every 5 seconds it looks for pods that need scheduling
// and places them on the best available node.
func (s *Scheduler) Start(ctx context.Context) {
	fmt.Println("Nimbus scheduler starting...")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	s.scheduleAll(ctx)
	for {
		select {
		case <-ticker.C:
			s.scheduleAll(ctx)
		case <-ctx.Done():
			fmt.Println("Scheduler shutting down...")
			return
		}
	}
}

func (s *Scheduler) scheduleAll(ctx context.Context) {
	s.refreshStatus(ctx)

	// Find all pods that:
	// 1. Want OUR scheduler (schedulerName: nimbus-scheduler)
	// 2. Haven't been assigned a node yet (nodeName is empty)
	pods, err := s.k8s.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.schedulerName=nimbus-scheduler,spec.nodeName=",
	})
	if err != nil {
		fmt.Printf("Scheduler failed to list pods: %v\n", err)
		return
	}

	if len(pods.Items) == 0 {
		return
	}

	fmt.Printf("Scheduling %d pending pod(s)\n", len(pods.Items))

	for _, pod := range pods.Items {
		if err := s.schedulePod(ctx, &pod); err != nil {
			fmt.Printf("Failed to schedule pod %s: %v\n", pod.Name, err)
		}
	}

	s.refreshStatus(ctx)
}

func (s *Scheduler) schedulePod(ctx context.Context, pod *corev1.Pod) error {
	fmt.Printf("Finding best node for pod %s\n", pod.Name)

	// Step 1: Get all schedulable nodes
	nodes, err := s.k8s.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	// Step 2: Filter out unschedulable nodes (cordoned nodes)
	var schedulableNodes []corev1.Node
	for _, node := range nodes.Items {
		if !node.Spec.Unschedulable {
			schedulableNodes = append(schedulableNodes, node)
		}
	}

	if len(schedulableNodes) == 0 {
		return fmt.Errorf("no schedulable nodes available")
	}

	// Step 3: Score each node — this is the brain of the scheduler
	scores := s.scoreNodes(ctx, schedulableNodes, pod)

	// Step 4: Pick the highest scoring node
	best := scores[0]
	fmt.Printf("Selected node %s with score %d\n", best.NodeName, best.Score)

	// Step 5: Bind the pod to that node
	if err := s.bindPod(ctx, pod, best.NodeName); err != nil {
		return err
	}
	s.recordBinding(pod, best.NodeName)
	return nil
}

// scoreNodes scores each node using a bin-packing strategy.
// We calculate how much CPU is already being USED on each node.
// More used = higher score = we prefer to fill it up further.
// This packs pods tightly onto fewer nodes, keeping others free.
func (s *Scheduler) scoreNodes(ctx context.Context, nodes []corev1.Node, pod *corev1.Pod) []NodeScore {
	var scores []NodeScore

	for _, node := range nodes {
		score := s.scoreNode(ctx, node, pod)
		scores = append(scores, NodeScore{
			NodeName: node.Name,
			Score:    score,
		})
		fmt.Printf("Node %s scored %d\n", node.Name, score)
	}

	// Sort descending — highest score first
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores
}

// scoreNode calculates a single node's score.
// Algorithm: measure live node CPU usage from metrics.k8s.io.
// Higher usage = higher score (bin packing = fill up nodes before using new ones).
func (s *Scheduler) scoreNode(ctx context.Context, node corev1.Node, pod *corev1.Pod) int64 {
	// Score from live node metrics so placement follows actual cluster load.
	nodeMetrics, err := kubeutil.ListNodeMetrics(ctx, s.dynamic)
	if err != nil {
		return 0
	}

	var usedCPUMilli int64
	for _, metric := range nodeMetrics {
		if metric.Name == node.Name {
			usedCPUMilli = metric.CPUMilli
			break
		}
	}

	allocatable := node.Status.Allocatable[corev1.ResourceCPU]
	totalCPU := allocatable.MilliValue()

	if totalCPU == 0 {
		return 0
	}

	return cpuUtilizationScore(usedCPUMilli, totalCPU)
}

// bindPod tells Kubernetes to assign this pod to this node.
// This is the actual scheduling decision — after this call,
// the kubelet on that node will pull the image and start the container.
func (s *Scheduler) bindPod(ctx context.Context, pod *corev1.Pod, nodeName string) error {
	binding := &corev1.Binding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		Target: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Node",
			Name:       nodeName,
		},
	}

	err := s.k8s.CoreV1().Pods(pod.Namespace).
		Bind(ctx, binding, metav1.CreateOptions{})
	if err != nil {
		metrics.SchedulerDecisions.WithLabelValues(nodeName, "error").Inc()
		return fmt.Errorf("failed to bind pod to node %s: %w", nodeName, err)
	}

	metrics.SchedulerDecisions.WithLabelValues(nodeName, "bound").Inc()
	fmt.Printf("Pod %s bound to node %s\n", pod.Name, nodeName)
	return nil
}

func cpuUtilizationScore(usedCPUMilli, totalCPUMilli int64) int64 {
	if totalCPUMilli == 0 {
		return 0
	}
	return (usedCPUMilli * 100) / totalCPUMilli
}

func (s *Scheduler) refreshStatus(ctx context.Context) {
	pendingPods, err := s.k8s.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.schedulerName=nimbus-scheduler,spec.nodeName=",
	})
	if err != nil {
		return
	}

	allSchedulerPods, err := s.k8s.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.schedulerName=nimbus-scheduler",
	})
	if err != nil {
		return
	}

	nodes, err := s.k8s.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	nodeMetrics, err := kubeutil.ListNodeMetrics(ctx, s.dynamic)
	if err != nil {
		return
	}

	metricByNode := make(map[string]kubeutil.NodeMetric, len(nodeMetrics))
	for _, metric := range nodeMetrics {
		metricByNode[metric.Name] = metric
	}

	nodeStatuses := make([]NodeStatus, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		metric := metricByNode[node.Name]
		allocatableCPU := node.Status.Allocatable[corev1.ResourceCPU]
		totalCPU := allocatableCPU.MilliValue()
		utilization := cpuUtilizationScore(metric.CPUMilli, totalCPU)
		nodeStatuses = append(nodeStatuses, NodeStatus{
			NodeName:       node.Name,
			CPUUtilization: utilization,
			Schedulable:    !node.Spec.Unschedulable,
		})
	}

	sort.Slice(nodeStatuses, func(i, j int) bool {
		return nodeStatuses[i].CPUUtilization > nodeStatuses[j].CPUUtilization
	})

	scheduledPods := make([]ScheduledPod, 0, len(allSchedulerPods.Items))
	activeScheduledPods := 0
	for _, pod := range allSchedulerPods.Items {
		tenantID := pod.Labels["tenant"]
		tenantName := pod.Namespace
		if len(tenantName) > len("nimbus-") && tenantName[:len("nimbus-")] == "nimbus-" {
			tenantName = tenantName[len("nimbus-"):]
		}
		scheduledPods = append(scheduledPods, ScheduledPod{
			Name:       pod.Name,
			Namespace:  pod.Namespace,
			NodeName:   pod.Spec.NodeName,
			TenantID:   tenantID,
			TenantName: tenantName,
			Phase:      string(pod.Status.Phase),
		})
		if pod.Spec.NodeName != "" && pod.Status.Phase == corev1.PodRunning {
			activeScheduledPods++
		}
	}

	pressure := int64(activeScheduledPods * 20)
	if pressure > 100 {
		pressure = 100
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.LastRun = time.Now()
	s.status.PendingPods = len(pendingPods.Items)
	s.status.ScheduledPods = activeScheduledPods
	s.status.Pressure = pressure
	s.status.Nodes = nodeStatuses
	s.status.Pods = scheduledPods
}

func (s *Scheduler) recordBinding(pod *corev1.Pod, nodeName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.RecentBindings = append([]BindingEvent{{
		PodName:   pod.Name,
		Namespace: pod.Namespace,
		NodeName:  nodeName,
		Timestamp: time.Now(),
	}}, s.status.RecentBindings...)

	if len(s.status.RecentBindings) > 10 {
		s.status.RecentBindings = s.status.RecentBindings[:10]
	}
}

func (s *Scheduler) GetStatus() StatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := s.status
	status.Nodes = append([]NodeStatus(nil), s.status.Nodes...)
	status.Pods = append([]ScheduledPod(nil), s.status.Pods...)
	status.RecentBindings = append([]BindingEvent(nil), s.status.RecentBindings...)
	return status
}
