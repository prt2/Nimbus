package kubeutil

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	podMetricsGVR = schema.GroupVersionResource{
		Group:    "metrics.k8s.io",
		Version:  "v1beta1",
		Resource: "pods",
	}
	nodeMetricsGVR = schema.GroupVersionResource{
		Group:    "metrics.k8s.io",
		Version:  "v1beta1",
		Resource: "nodes",
	}
)

type PodMetric struct {
	Name        string
	Namespace   string
	CPUMilli    int64
	MemoryBytes int64
}

type NodeMetric struct {
	Name        string
	CPUMilli    int64
	MemoryBytes int64
}

func ListPodMetrics(ctx context.Context, client dynamic.Interface, namespace string) ([]PodMetric, error) {
	list, err := client.Resource(podMetricsGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	metrics := make([]PodMetric, 0, len(list.Items))
	for _, item := range list.Items {
		cpuMilli, memoryBytes, err := aggregateContainerUsage(item)
		if err != nil {
			return nil, err
		}

		metrics = append(metrics, PodMetric{
			Name:        item.GetName(),
			Namespace:   item.GetNamespace(),
			CPUMilli:    cpuMilli,
			MemoryBytes: memoryBytes,
		})
	}
	return metrics, nil
}

func ListNodeMetrics(ctx context.Context, client dynamic.Interface) ([]NodeMetric, error) {
	list, err := client.Resource(nodeMetricsGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	metrics := make([]NodeMetric, 0, len(list.Items))
	for _, item := range list.Items {
		cpuMilli, memoryBytes, err := aggregateTopLevelUsage(item)
		if err != nil {
			return nil, err
		}

		metrics = append(metrics, NodeMetric{
			Name:        item.GetName(),
			CPUMilli:    cpuMilli,
			MemoryBytes: memoryBytes,
		})
	}
	return metrics, nil
}

func aggregateTopLevelUsage(item unstructured.Unstructured) (int64, int64, error) {
	usage, found, err := unstructured.NestedStringMap(item.Object, "usage")
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read node usage: %w", err)
	}
	if !found {
		return 0, 0, nil
	}

	var cpuMilli int64
	if cpuRaw, ok := usage["cpu"]; ok {
		qty, err := resource.ParseQuantity(cpuRaw)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to parse cpu quantity %q: %w", cpuRaw, err)
		}
		cpuMilli = qty.MilliValue()
	}

	var memoryBytes int64
	if memRaw, ok := usage["memory"]; ok {
		qty, err := resource.ParseQuantity(memRaw)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to parse memory quantity %q: %w", memRaw, err)
		}
		memoryBytes = qty.Value()
	}

	return cpuMilli, memoryBytes, nil
}

func aggregateContainerUsage(item unstructured.Unstructured) (int64, int64, error) {
	containers, found, err := unstructured.NestedSlice(item.Object, "containers")
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read metrics containers: %w", err)
	}
	if !found {
		return 0, 0, nil
	}

	var cpuMilli int64
	var memoryBytes int64

	for _, container := range containers {
		containerMap, ok := container.(map[string]interface{})
		if !ok {
			continue
		}

		usage, ok := containerMap["usage"].(map[string]interface{})
		if !ok {
			continue
		}

		if cpuRaw, ok := usage["cpu"].(string); ok {
			qty, err := resource.ParseQuantity(cpuRaw)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to parse cpu quantity %q: %w", cpuRaw, err)
			}
			cpuMilli += qty.MilliValue()
		}

		if memRaw, ok := usage["memory"].(string); ok {
			qty, err := resource.ParseQuantity(memRaw)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to parse memory quantity %q: %w", memRaw, err)
			}
			memoryBytes += qty.Value()
		}
	}

	return cpuMilli, memoryBytes, nil
}
