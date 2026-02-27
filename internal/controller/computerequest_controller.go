package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type ComputeRequestSpec struct {
	TenantID string `json:"tenantId"`
	Image    string `json:"image"`
	CPU      string `json:"cpu"`
	Memory   string `json:"memory"`
	Replicas int32  `json:"replicas"`
}

type ComputeRequestStatus struct {
	Phase          string `json:"phase"`
	DeploymentName string `json:"deploymentName"`
	Message        string `json:"message"`
}

type ComputeRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ComputeRequestSpec   `json:"spec"`
	Status            ComputeRequestStatus `json:"status,omitempty"`
}

type Controller struct {
	k8s     kubernetes.Interface
	dynamic dynamic.Interface
}

var computeRequestGVR = schema.GroupVersionResource{
	Group:    "nimbus.io",
	Version:  "v1",
	Resource: "computerequests",
}

func NewController() (*Controller, error) {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Controller{k8s: k8sClient, dynamic: dynamicClient}, nil
}

func (c *Controller) Start(ctx context.Context) {
	fmt.Println("🎮 ComputeRequest controller starting...")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	c.reconcileAll(ctx)
	for {
		select {
		case <-ticker.C:
			c.reconcileAll(ctx)
		case <-ctx.Done():
			fmt.Println("Controller shutting down...")
			return
		}
	}
}

func (c *Controller) reconcileAll(ctx context.Context) {
	list, err := c.dynamic.Resource(computeRequestGVR).
		Namespace("").
		List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Printf("❌ Failed to list ComputeRequests: %v\n", err)
		return
	}

	fmt.Printf("🔄 Reconciling %d ComputeRequest(s)...\n", len(list.Items))

	for _, item := range list.Items {
		data, _ := json.Marshal(item.Object)
		var cr ComputeRequest
		if err := json.Unmarshal(data, &cr); err != nil {
			fmt.Printf("❌ Failed to decode ComputeRequest: %v\n", err)
			continue
		}
		c.reconcile(ctx, &cr)
	}
}

func (c *Controller) reconcile(ctx context.Context, cr *ComputeRequest) {
	namespace := "nimbus-" + cr.Spec.TenantID
	deploymentName := "nimbus-" + cr.Name

	_, err := c.k8s.AppsV1().Deployments(namespace).
		Get(ctx, deploymentName, metav1.GetOptions{})

	if errors.IsNotFound(err) {
		fmt.Printf("  ➕ Creating deployment %s in %s\n", deploymentName, namespace)
		if err := c.createDeployment(ctx, cr, namespace, deploymentName); err != nil {
			fmt.Printf("  ❌ Failed to create deployment: %v\n", err)
			c.updateStatus(ctx, cr, "Failed", deploymentName, err.Error())
			return
		}
		c.updateStatus(ctx, cr, "Running", deploymentName, "Deployment created successfully")
		fmt.Printf("  ✅ Deployment %s created\n", deploymentName)
	} else if err != nil {
		fmt.Printf("  ❌ Error checking deployment: %v\n", err)
	} else {
		fmt.Printf("  ✓  Deployment %s already exists\n", deploymentName)
	}
}

func (c *Controller) createDeployment(ctx context.Context, cr *ComputeRequest, namespace, name string) error {
	replicas := cr.Spec.Replicas
	if replicas == 0 {
		replicas = 1
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"managed-by": "nimbus",
				"tenant":     cr.Spec.TenantID,
				"cr-name":    cr.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "workload",
							Image: cr.Spec.Image,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(cr.Spec.CPU),
									corev1.ResourceMemory: resource.MustParse(cr.Spec.Memory),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(cr.Spec.CPU),
									corev1.ResourceMemory: resource.MustParse(cr.Spec.Memory),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := c.k8s.AppsV1().Deployments(namespace).
		Create(ctx, deployment, metav1.CreateOptions{})
	return err
}

func (c *Controller) updateStatus(ctx context.Context, cr *ComputeRequest, phase, deploymentName, message string) {
	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"phase":          phase,
			"deploymentName": deploymentName,
			"message":        message,
		},
	}
	patchBytes, _ := json.Marshal(patch)

	_, err := c.dynamic.Resource(computeRequestGVR).
		Namespace(cr.Namespace).
		Patch(ctx, cr.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{}, "status")
	if err != nil {
		fmt.Printf("  ⚠️  Failed to update status: %v\n", err)
	}
}
