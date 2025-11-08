package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	greetv1 "github.com/holos-run/tokensmith/api/greet/v1"
)

func TestGreetServer_Greet(t *testing.T) {
	tests := []struct {
		name     string
		reqName  string
		wantText string
	}{
		{
			name:     "with name",
			reqName:  "Alice",
			wantText: "Hello, Alice!",
		},
		{
			name:     "empty name defaults to World",
			reqName:  "",
			wantText: "Hello, World!",
		},
		{
			name:     "with different name",
			reqName:  "Bob",
			wantText: "Hello, Bob!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewGreetServer()
			req := connect.NewRequest(&greetv1.GreetRequest{
				Name: tt.reqName,
			})

			res, err := srv.Greet(context.Background(), req)
			if err != nil {
				t.Fatalf("Greet() error = %v", err)
			}

			if got := res.Msg.Greeting; got != tt.wantText {
				t.Errorf("Greet() = %v, want %v", got, tt.wantText)
			}
		})
	}
}
