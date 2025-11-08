package commands

import (
	"os"

	"github.com/holos-run/tokensmith/internal/logging"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	logLevel  string
	logFormat string
)

// NewRootCmd creates the root command for tokensmith.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tokensmith",
		Short: "Tokensmith - Envoy External Authorizer for OIDC Token Exchange",
		Long: `Tokensmith is an Envoy external authorizer (ext_authz) for Istio 1.27+
that exchanges OIDC ID tokens for Kubernetes service accounts in one cluster
for ID tokens for valid Kubernetes service accounts in another cluster.

This enables secure cross-cluster authentication in multi-cluster Kubernetes
environments using native service account identities.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Configure logging before any command runs
			cfg := logging.Config{
				Level:  logging.ParseLevel(logLevel),
				Format: logFormat,
			}
			logging.SetDefault(cfg)
		},
		SilenceUsage:  true, // Don't show usage on errors
		SilenceErrors: true, // We'll handle errors ourselves
	}

	// Global flags available to all commands
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"Log level (debug, info, warn, error)")
	cmd.PersistentFlags().StringVar(&logFormat, "log-format", "json",
		"Log format (json, text)")

	// Add subcommands
	cmd.AddCommand(NewVersionCmd())
	cmd.AddCommand(NewServeCmd())
	cmd.AddCommand(NewGreetCmd())
	cmd.AddCommand(NewAuthzCmd())

	return cmd
}

// Execute runs the root command.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
