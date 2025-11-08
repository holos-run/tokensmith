package commands

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	envoy_auth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/holos-run/tokensmith/internal/authz"
	"github.com/holos-run/tokensmith/internal/token"
)

var (
	authzAddr              string
	authzPort              int
	workloadKubeconfig     string
	tokenExpirationSeconds int64
)

// NewAuthzCmd creates the authz command.
func NewAuthzCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "authz",
		Short: "Start the Envoy external authorization server",
		Long: `Start the Envoy external authorization (ext_authz) gRPC server.

This server validates OIDC tokens from a workload cluster and exchanges them
for tokens in the management cluster using Kubernetes TokenReview and TokenRequest APIs.`,
		RunE: runAuthz,
	}

	cmd.Flags().StringVar(&authzAddr, "addr", "0.0.0.0", "Server address")
	cmd.Flags().IntVar(&authzPort, "port", 9001, "Server port")
	cmd.Flags().StringVar(&workloadKubeconfig, "workload-kubeconfig", "",
		"Path to kubeconfig for workload cluster (if empty, uses in-cluster config)")
	cmd.Flags().Int64Var(&tokenExpirationSeconds, "token-expiration", 3600,
		"Token expiration in seconds (default: 3600 = 1 hour)")

	return cmd
}

func runAuthz(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	logger := slog.Default()

	logger.Info("initializing external authorization server",
		slog.String("addr", authzAddr),
		slog.Int("port", authzPort),
		slog.String("workload_kubeconfig", workloadKubeconfig),
		slog.Int64("token_expiration", tokenExpirationSeconds),
	)

	// Initialize Kubernetes clients
	clientConfig := token.ClientConfig{
		WorkloadKubeconfig:        workloadKubeconfig,
		UseInClusterForManagement: true,
	}

	clients, err := token.NewClients(ctx, clientConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	// Health check both clusters
	logger.Info("performing cluster health checks")
	if err := clients.HealthCheck(ctx); err != nil {
		return fmt.Errorf("cluster health check failed: %w", err)
	}
	logger.Info("cluster health checks passed")

	// Create token validator (workload cluster)
	validator := token.NewValidator(clients.Workload)

	// Create token exchanger (management cluster)
	exchangeConfig := token.ExchangeConfig{
		Audiences:         []string{"https://kubernetes.default.svc"},
		ExpirationSeconds: &tokenExpirationSeconds,
	}
	exchanger := token.NewExchanger(clients.Management, exchangeConfig)

	// Create ext_authz server
	authzServer := authz.NewServer(validator, exchanger, logger)

	// Create gRPC server
	grpcServer := grpc.NewServer()

	// Register ext_authz service
	envoy_auth.RegisterAuthorizationServer(grpcServer, authzServer)

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Create listener
	addr := fmt.Sprintf("%s:%d", authzAddr, authzPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	logger.Info("starting external authorization server",
		slog.String("addr", addr),
	)

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			errCh <- err
		}
	}()

	// Wait for interrupt signal or error
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case sig := <-sigCh:
		logger.Info("received signal, shutting down",
			slog.String("signal", sig.String()))
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Stop accepting new connections and wait for existing RPCs to complete
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-shutdownCtx.Done():
		logger.Warn("graceful shutdown timeout, forcing shutdown")
		grpcServer.Stop()
	case <-stopped:
		logger.Info("server stopped gracefully")
	}

	return nil
}
