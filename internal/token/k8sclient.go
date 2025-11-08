package token

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ClientConfig holds configuration for Kubernetes clients.
type ClientConfig struct {
	// WorkloadKubeconfig is the path to the kubeconfig for the workload cluster.
	// If empty, uses in-cluster configuration.
	WorkloadKubeconfig string

	// UseInClusterForManagement indicates whether to use in-cluster config for
	// the management cluster. Defaults to true.
	UseInClusterForManagement bool
}

// Clients holds Kubernetes clients for both workload and management clusters.
type Clients struct {
	// Workload is the client for the workload cluster (source of tokens).
	Workload kubernetes.Interface

	// Management is the client for the management cluster (target for token exchange).
	Management kubernetes.Interface
}

// NewClients creates and initializes Kubernetes clients for both clusters.
func NewClients(ctx context.Context, config ClientConfig) (*Clients, error) {
	// Initialize workload cluster client
	workloadClient, err := newWorkloadClient(config.WorkloadKubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create workload cluster client: %w", err)
	}

	// Initialize management cluster client
	managementClient, err := newManagementClient(config.UseInClusterForManagement)
	if err != nil {
		return nil, fmt.Errorf("failed to create management cluster client: %w", err)
	}

	return &Clients{
		Workload:   workloadClient,
		Management: managementClient,
	}, nil
}

// newWorkloadClient creates a Kubernetes client for the workload cluster.
func newWorkloadClient(kubeconfigPath string) (kubernetes.Interface, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath != "" {
		// Load from kubeconfig file
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig from %s: %w", kubeconfigPath, err)
		}
	} else {
		// Use in-cluster configuration
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load in-cluster config: %w", err)
		}
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return client, nil
}

// newManagementClient creates a Kubernetes client for the management cluster.
func newManagementClient(useInCluster bool) (kubernetes.Interface, error) {
	if !useInCluster {
		return nil, fmt.Errorf("management cluster currently only supports in-cluster configuration")
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load in-cluster config: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return client, nil
}

// HealthCheck verifies connectivity to both clusters.
func (c *Clients) HealthCheck(ctx context.Context) error {
	// Check workload cluster
	if _, err := c.Workload.Discovery().ServerVersion(); err != nil {
		return fmt.Errorf("workload cluster health check failed: %w", err)
	}

	// Check management cluster
	if _, err := c.Management.Discovery().ServerVersion(); err != nil {
		return fmt.Errorf("management cluster health check failed: %w", err)
	}

	return nil
}
