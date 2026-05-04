package autoscaler

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/prt2/nimbus/internal/kubeutil"
	"github.com/prt2/nimbus/internal/metrics"
)

const (
	// Simple demo thresholds for now.
	scaleUpThreshold   = 70
	scaleDownThreshold = 30
	maxReplicas        = 5
	minReplicas        = 1
)

type Autoscaler struct {
	k8s     kubernetes.Interface
	dynamic dynamic.Interface
}

func NewAutoscaler() (*Autoscaler, error) {
	// Same idea as the scheduler: normal APIs plus metrics access.
	client, err := kubeutil.NewKubernetesClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	dynamicClient, err := kubeutil.NewDynamicClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Autoscaler{k8s: client, dynamic: dynamicClient}, nil
}

func (a *Autoscaler) Start(ctx context.Context) {
	// Check every 30 seconds whether replicas still make sense.
	fmt.Println("Autoscaler starting...")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	a.reconcile(ctx)
	for {
		select {
		case <-ticker.C:
			a.reconcile(ctx)
		case <-ctx.Done():
			fmt.Println("Autoscaler shutting down...")
			return
		}
	}
}

func (a *Autoscaler) reconcile(ctx context.Context) {
	// Only look at namespaces Nimbus created.
	namespaces, err := a.k8s.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "managed-by=nimbus",
	})
	if err != nil {
		fmt.Printf("Autoscaler: failed to list namespaces: %v\n", err)
		return
	}
	for _, ns := range namespaces.Items {
		a.reconcileNamespace(ctx, ns.Name)
	}
}

func (a *Autoscaler) reconcileNamespace(ctx context.Context, namespace string) {
	// Only touch deployments we created.
	deployments, err := a.k8s.AppsV1().Deployments(namespace).
		List(ctx, metav1.ListOptions{
			LabelSelector: "managed-by=nimbus",
		})
	if err != nil {
		return
	}

	for _, deploy := range deployments.Items {
		currentReplicas := int32(1)
		if deploy.Spec.Replicas != nil {
			currentReplicas = *deploy.Spec.Replicas
		}

		// Use the hotter of CPU or memory for the decision.
		cpuUtilization, memoryUtilization, err := a.resourceUtilization(ctx, namespace, &deploy, currentReplicas)
		if err != nil {
			fmt.Printf("Autoscaler: failed to compute utilization for %s/%s: %v\n", namespace, deploy.Name, err)
			continue
		}

		// Scale from live usage divided by requested resources, and use the
		// more saturated dimension to make the decision.
		utilization := maxInt32(cpuUtilization, memoryUtilization)
		newReplicas := currentReplicas

		if utilization >= scaleUpThreshold && currentReplicas < maxReplicas {
			newReplicas = currentReplicas + 1
			fmt.Printf("[%s/%s] cpu=%d%% memory=%d%% scaling up %d->%d\n",
				namespace, deploy.Name, cpuUtilization, memoryUtilization, currentReplicas, newReplicas)
			metrics.AutoscalerActions.WithLabelValues(namespace, "up").Inc()
		} else if utilization <= scaleDownThreshold && currentReplicas > minReplicas {
			newReplicas = currentReplicas - 1
			fmt.Printf("[%s/%s] cpu=%d%% memory=%d%% scaling down %d->%d\n",
				namespace, deploy.Name, cpuUtilization, memoryUtilization, currentReplicas, newReplicas)
			metrics.AutoscalerActions.WithLabelValues(namespace, "down").Inc()
		} else {
			fmt.Printf("[%s/%s] cpu=%d%% memory=%d%% replicas=%d no change\n",
				namespace, deploy.Name, cpuUtilization, memoryUtilization, currentReplicas)
			continue
		}

		deploy.Spec.Replicas = &newReplicas
		_, err = a.k8s.AppsV1().Deployments(namespace).
			Update(ctx, &deploy, metav1.UpdateOptions{})
		if err != nil {
			fmt.Printf("Failed to scale %s: %v\n", deploy.Name, err)
		}
	}
}

func (a *Autoscaler) resourceUtilization(ctx context.Context, namespace string, deploy *appsv1.Deployment, replicas int32) (int32, int32, error) {
	if replicas == 0 {
		return 0, 0, nil
	}

	// Match pods back to this Deployment before reading usage.
	selector := labels.SelectorFromSet(deploy.Spec.Selector.MatchLabels).String()
	pods, err := a.k8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return 0, 0, err
	}

	podMetrics, err := kubeutil.ListPodMetrics(ctx, a.dynamic, namespace)
	if err != nil {
		return 0, 0, err
	}

	metricByPod := make(map[string]kubeutil.PodMetric, len(podMetrics))
	for _, metric := range podMetrics {
		metricByPod[metric.Name] = metric
	}

	var usedCPUMilli int64
	var usedMemoryBytes int64
	var requestedCPUMilli int64
	var requestedMemoryBytes int64

	for _, pod := range pods.Items {
		metric, ok := metricByPod[pod.Name]
		if ok {
			usedCPUMilli += metric.CPUMilli
			usedMemoryBytes += metric.MemoryBytes
		}
		for _, container := range pod.Spec.Containers {
			if cpu, ok := container.Resources.Requests["cpu"]; ok {
				requestedCPUMilli += cpu.MilliValue()
			}
			if memory, ok := container.Resources.Requests["memory"]; ok {
				requestedMemoryBytes += memory.Value()
			}
		}
	}

	var cpuUtilization int32
	if requestedCPUMilli > 0 {
		// Compare live usage against the resources the pod asked for.
		cpuUtilization = int32((usedCPUMilli * 100) / requestedCPUMilli)
	}

	var memoryUtilization int32
	if requestedMemoryBytes > 0 {
		memoryUtilization = int32((usedMemoryBytes * 100) / requestedMemoryBytes)
	}

	return cpuUtilization, memoryUtilization, nil
}

func maxInt32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
