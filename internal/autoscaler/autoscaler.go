package autoscaler

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const (
	scaleUpThreshold   = 70
	scaleDownThreshold = 30
	maxReplicas        = 5
	minReplicas        = 1
)

type Autoscaler struct {
	k8s kubernetes.Interface
}

func NewAutoscaler() (*Autoscaler, error) {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}
	return &Autoscaler{k8s: client}, nil
}

func (a *Autoscaler) Start(ctx context.Context) {
	fmt.Println("📈 Autoscaler starting...")
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
	namespaces, err := a.k8s.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "managed-by=nimbus",
	})
	if err != nil {
		fmt.Printf("❌ Autoscaler: failed to list namespaces: %v\n", err)
		return
	}
	for _, ns := range namespaces.Items {
		a.reconcileNamespace(ctx, ns.Name)
	}
}

func (a *Autoscaler) reconcileNamespace(ctx context.Context, namespace string) {
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

		readyReplicas := deploy.Status.ReadyReplicas
		desiredReplicas := deploy.Status.Replicas

		if desiredReplicas == 0 {
			continue
		}

		utilization := int32(100)
		if desiredReplicas > 0 {
			utilization = (readyReplicas * 100) / desiredReplicas
		}

		newReplicas := currentReplicas

		if utilization >= scaleUpThreshold && currentReplicas < maxReplicas {
			newReplicas = currentReplicas + 1
			fmt.Printf("📈 [%s/%s] utilization=%d%% → scaling UP %d→%d\n",
				namespace, deploy.Name, utilization, currentReplicas, newReplicas)
		} else if utilization <= scaleDownThreshold && currentReplicas > minReplicas {
			newReplicas = currentReplicas - 1
			fmt.Printf("📉 [%s/%s] utilization=%d%% → scaling DOWN %d→%d\n",
				namespace, deploy.Name, utilization, currentReplicas, newReplicas)
		} else {
			fmt.Printf("📊 [%s/%s] utilization=%d%% → replicas=%d (no change)\n",
				namespace, deploy.Name, utilization, currentReplicas)
			continue
		}

		deploy.Spec.Replicas = &newReplicas
		_, err := a.k8s.AppsV1().Deployments(namespace).
			Update(ctx, &deploy, metav1.UpdateOptions{})
		if err != nil {
			fmt.Printf("❌ Failed to scale %s: %v\n", deploy.Name, err)
		}
	}
}
