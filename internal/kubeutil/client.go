// Builds shared Kubernetes clients using local or in-cluster connection settings.

package kubeutil

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// LoadConfig tries KUBECONFIG first, then in-cluster credentials, then ~/.kube/config.
func LoadConfig() (*rest.Config, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	if config, err := rest.InClusterConfig(); err == nil {
		return config, nil
	}

	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

// NewKubernetesClient creates a client for standard objects such as pods and deployments.
func NewKubernetesClient() (kubernetes.Interface, error) {
	config, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kube config: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}
	return client, nil
}

// NewDynamicClient creates a client for custom resources and metrics without fixed Go types.
func NewDynamicClient() (dynamic.Interface, error) {
	config, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kube config: %w", err)
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}
	return client, nil
}
