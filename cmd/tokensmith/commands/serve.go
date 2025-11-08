package commands

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	greetv1connect "github.com/holos-run/tokensmith/api/greet/v1/greetv1connect"
	"github.com/holos-run/tokensmith/internal/server"
	"github.com/spf13/cobra"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

var (
	serveAddr string
	servePort int
)

// NewServeCmd creates the serve command.
func NewServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the gRPC server",
		RunE:  runServe,
	}

	cmd.Flags().StringVar(&serveAddr, "addr", "localhost", "Server address")
	cmd.Flags().IntVar(&servePort, "port", 8080, "Server port")

	return cmd
}

func runServe(cmd *cobra.Command, args []string) error {
	// Create the greet service
	greetSvc := server.NewGreetServer()
	path, handler := greetv1connect.NewGreetServiceHandler(greetSvc)

	mux := http.NewServeMux()
	mux.Handle(path, handler)

	addr := fmt.Sprintf("%s:%d", serveAddr, servePort)

	// Use h2c for HTTP/2 without TLS (suitable for development)
	srv := &http.Server{
		Addr:    addr,
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}

	slog.Info("starting grpc server",
		slog.String("addr", addr),
		slog.String("service", "GreetService"))

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
		slog.Info("received signal, shutting down",
			slog.String("signal", sig.String()))
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	slog.Info("server stopped")
	return nil
}
