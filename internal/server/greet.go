package server

import (
	"context"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	greetv1 "github.com/holos-run/tokensmith/api/greet/v1"
)

// GreetServer implements the GreetService.
type GreetServer struct{}

// NewGreetServer creates a new GreetServer.
func NewGreetServer() *GreetServer {
	return &GreetServer{}
}

// Greet implements the Greet RPC method.
func (s *GreetServer) Greet(
	ctx context.Context,
	req *connect.Request[greetv1.GreetRequest],
) (*connect.Response[greetv1.GreetResponse], error) {
	name := req.Msg.Name
	if name == "" {
		name = "World"
	}

	slog.Info("processing greet request",
		slog.String("name", name))

	greeting := fmt.Sprintf("Hello, %s!", name)
	res := connect.NewResponse(&greetv1.GreetResponse{
		Greeting: greeting,
	})

	return res, nil
}
