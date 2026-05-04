// Turns ComputeRequest objects into Kubernetes deployments.
// Flow: read requests -> create missing deployments -> write the result back to request status.

package controller

import (
	"context"
	"encoding/json"
	"fmt"
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

	"github.com/prt2/nimbus/internal/kubeutil"
)

type ComputeRequestSpec struct {
	// These are the fields users put on the CRD.
	TenantID string `json:"tenantId"`
	Image    string `json:"image"`
	CPU      string `json:"cpu"`
	Memory   string `json:"memory"`
	Replicas int32  `json:"replicas"`
}

type ComputeRequestStatus struct {
	// Keep a small status on the CR so it is obvious what happened.
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

// Identify the ComputeRequest resource in the Kubernetes API.
var computeRequestGVR = schema.GroupVersionResource{
	Group:    "nimbus.io",
	Version:  "v1",
	Resource: "computerequests",
}

// NewController creates clients for standard Kubernetes objects and custom requests.
func NewController() (*Controller, error) {
	// Need typed client for Deployments and dynamic client for the CRD.
	k8sClient, err := kubeutil.NewKubernetesClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	dynamicClient, err := kubeutil.NewDynamicClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Controller{k8s: k8sClient, dynamic: dynamicClient}, nil
}

// Start processes requests immediately and every 10 seconds until cancelled.
func (c *Controller) Start(ctx context.Context) {
	// Simple polling loop for now. Recheck everything every 10 seconds.
	fmt.Println("ComputeRequest controller starting...")
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

// reconcileAll reads custom resource objects and converts them into Go request values.
func (c *Controller) reconcileAll(ctx context.Context) {
	// Pull all requests and reconcile them one at a time.
	list, err := c.dynamic.Resource(computeRequestGVR).
		Namespace("").
		List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Failed to list ComputeRequests: %v\n", err)
		return
	}

	fmt.Printf("Reconciling %d ComputeRequest(s)...\n", len(list.Items))

	for _, item := range list.Items {
		data, _ := json.Marshal(item.Object)
		var cr ComputeRequest
		if err := json.Unmarshal(data, &cr); err != nil {
			fmt.Printf("Failed to decode ComputeRequest: %v\n", err)
			continue
		}
		c.reconcile(ctx, &cr)
	}
}

// reconcile creates a deployment only when it is missing and records the creation result.
func (c *Controller) reconcile(ctx context.Context, cr *ComputeRequest) {
	// Keep names deterministic so reconcile can find the same objects later.
	namespace := "nimbus-" + cr.Spec.TenantID
	deploymentName := "nimbus-" + cr.Name

	_, err := c.k8s.AppsV1().Deployments(namespace).
		Get(ctx, deploymentName, metav1.GetOptions{})

	if errors.IsNotFound(err) {
		// No Deployment yet means we need to create it now.
		fmt.Printf("Creating deployment %s in %s\n", deploymentName, namespace)
		if err := c.createDeployment(ctx, cr, namespace, deploymentName); err != nil {
			fmt.Printf("Failed to create deployment: %v\n", err)
			c.updateStatus(ctx, cr, "Failed", deploymentName, err.Error())
			return
		}
		c.updateStatus(ctx, cr, "Running", deploymentName, "Deployment created successfully")
		fmt.Printf("Deployment %s created\n", deploymentName)
	} else if err != nil {
		fmt.Printf("Error checking deployment: %v\n", err)
	} else {
		// This controller only creates right now. It does not patch existing Deployments.
		fmt.Printf("Deployment %s already exists\n", deploymentName)
	}
}

// createDeployment sets the image, replica count, resource requests and limits, and custom scheduler.
func (c *Controller) createDeployment(ctx context.Context, cr *ComputeRequest, namespace, name string) error {
	// Default to one replica if the CR leaves it empty.
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
					// Route controller-managed workloads through the custom scheduler.
					SchedulerName: "nimbus-scheduler",
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

// updateStatus patches the request status without changing the user’s requested settings.
func (c *Controller) updateStatus(ctx context.Context, cr *ComputeRequest, phase, deploymentName, message string) {
	// Patch status separately so the CR shows what reconcile just did.
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
		fmt.Printf("Failed to update status: %v\n", err)
	}
}
