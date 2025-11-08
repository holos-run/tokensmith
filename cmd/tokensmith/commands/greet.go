package commands

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	greetv1 "github.com/holos-run/tokensmith/api/greet/v1"
	greetv1connect "github.com/holos-run/tokensmith/api/greet/v1/greetv1connect"
	"github.com/spf13/cobra"
)

var (
	greetServerURL string
	greetName      string
	greetTimeout   time.Duration
)

// NewGreetCmd creates the greet command.
func NewGreetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "greet",
		Short: "Call the greet service",
		RunE:  runGreet,
	}

	cmd.Flags().StringVar(&greetServerURL, "server", "http://localhost:8080",
		"Server URL")
	cmd.Flags().StringVar(&greetName, "name", "",
		"Name to greet (default: World)")
	cmd.Flags().DurationVar(&greetTimeout, "timeout", 5*time.Second,
		"Request timeout")

	return cmd
}

func runGreet(cmd *cobra.Command, args []string) error {
	// Create client
	client := greetv1connect.NewGreetServiceClient(
		http.DefaultClient,
		greetServerURL,
	)

	// Create request
	req := connect.NewRequest(&greetv1.GreetRequest{
		Name: greetName,
	})

	ctx, cancel := context.WithTimeout(cmd.Context(), greetTimeout)
	defer cancel()

	slog.Debug("calling greet service",
		slog.String("server", greetServerURL),
		slog.String("name", greetName))

	// Call the service
	res, err := client.Greet(ctx, req)
	if err != nil {
		return fmt.Errorf("greet failed: %w", err)
	}

	// Print the result to stdout
	fmt.Println(res.Msg.Greeting)

	return nil
}
