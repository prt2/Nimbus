package tenant

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"path/filepath"
)

// Provisioner talks to the Kubernetes API to create real infrastructure
// for each tenant — namespace, resource quotas, and limits.
type Provisioner struct {
	k8s kubernetes.Interface
}

func NewProvisioner() (*Provisioner, error) {
	// Load your local kubeconfig (the same file kubectl uses)
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	return &Provisioner{k8s: client}, nil
}

// Provision creates all K8s resources for a new tenant.
// This is called every time POST /tenants succeeds.
func (p *Provisioner) Provision(ctx context.Context, t *Tenant) error {
	if err := p.createNamespace(ctx, t); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}
	if err := p.createResourceQuota(ctx, t); err != nil {
		return fmt.Errorf("failed to create resource quota: %w", err)
	}
	fmt.Printf("✅ Provisioned tenant %s → namespace %s\n", t.Name, t.Namespace)
	return nil
}

func (p *Provisioner) createNamespace(ctx context.Context, t *Tenant) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: t.Namespace,
			// Labels let us find all Nimbus namespaces later
			Labels: map[string]string{
				"managed-by":  "nimbus",
				"tenant-id":   t.ID,
				"tenant-name": t.Name,
			},
		},
	}

	_, err := p.k8s.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	return err
}

func (p *Provisioner) createResourceQuota(ctx context.Context, t *Tenant) error {
	// ResourceQuota enforces hard limits on what a tenant can consume.
	// If a tenant tries to exceed these, Kubernetes rejects the request.
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nimbus-quota",
			Namespace: t.Namespace,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(t.CPUQuota),
				corev1.ResourceMemory: resource.MustParse(t.MemoryQuota),
				// Limit number of pods too — prevents runaway workloads
				corev1.ResourcePods: resource.MustParse("10"),
			},
		},
	}

	_, err := p.k8s.CoreV1().ResourceQuotas(t.Namespace).Create(ctx, quota, metav1.CreateOptions{})
	return err
}

// Deprovision cleans up all K8s resources when a tenant is deleted.
// Deleting the namespace automatically deletes everything inside it.
func (p *Provisioner) Deprovision(ctx context.Context, t *Tenant) error {
	err := p.k8s.CoreV1().Namespaces().Delete(ctx, t.Namespace, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}
	fmt.Printf("🗑️  Deprovisioned tenant %s → deleted namespace %s\n", t.Name, t.Namespace)
	return nil
}
