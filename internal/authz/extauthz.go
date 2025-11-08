package authz

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	envoy_auth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	envoy_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"

	"github.com/holos-run/tokensmith/internal/token"
)

// Server implements the Envoy External Authorization gRPC API.
type Server struct {
	envoy_auth.UnimplementedAuthorizationServer

	validator *token.Validator
	exchanger *token.Exchanger
	logger    *slog.Logger
}

// NewServer creates a new external authorization server.
func NewServer(validator *token.Validator, exchanger *token.Exchanger, logger *slog.Logger) *Server {
	return &Server{
		validator: validator,
		exchanger: exchanger,
		logger:    logger,
	}
}

// Check implements the ext_authz Check RPC.
func (s *Server) Check(ctx context.Context, req *envoy_auth.CheckRequest) (*envoy_auth.CheckResponse, error) {
	s.logger.Info("received authorization check request",
		slog.String("path", getPath(req)),
		slog.String("method", getMethod(req)),
	)

	// Extract bearer token from Authorization header
	bearerToken, err := extractBearerToken(req)
	if err != nil {
		s.logger.Warn("failed to extract bearer token",
			slog.String("error", err.Error()),
		)
		return s.denyResponse(codes.Unauthenticated, "Missing or invalid Authorization header"), nil
	}

	// Validate token using workload cluster TokenReview API
	identity, err := s.validator.Validate(ctx, bearerToken)
	if err != nil {
		s.logger.Warn("token validation failed",
			slog.String("error", err.Error()),
		)
		return s.denyResponse(codes.Unauthenticated, "Token validation failed"), nil
	}

	s.logger.Info("token validated successfully",
		slog.String("namespace", identity.Namespace),
		slog.String("service_account", identity.Name),
		slog.String("uid", identity.UID),
	)

	// Exchange for management cluster token
	managementToken, err := s.exchanger.Exchange(ctx, identity)
	if err != nil {
		s.logger.Error("token exchange failed",
			slog.String("error", err.Error()),
			slog.String("namespace", identity.Namespace),
			slog.String("service_account", identity.Name),
		)
		return s.denyResponse(codes.PermissionDenied, "Token exchange failed"), nil
	}

	s.logger.Info("token exchanged successfully",
		slog.String("namespace", identity.Namespace),
		slog.String("service_account", identity.Name),
	)

	// Return OK response with modified Authorization header
	return s.okResponseWithToken(managementToken), nil
}

// extractBearerToken extracts the bearer token from the Authorization header.
func extractBearerToken(req *envoy_auth.CheckRequest) (string, error) {
	headers := req.GetAttributes().GetRequest().GetHttp().GetHeaders()
	if headers == nil {
		return "", fmt.Errorf("no headers in request")
	}

	authHeader, ok := headers["authorization"]
	if !ok {
		return "", fmt.Errorf("authorization header not found")
	}

	// Expected format: "Bearer <token>"
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return "", fmt.Errorf("authorization header is not a Bearer token")
	}

	token := strings.TrimPrefix(authHeader, bearerPrefix)
	if token == "" {
		return "", fmt.Errorf("bearer token is empty")
	}

	return token, nil
}

// getPath extracts the request path from the check request.
func getPath(req *envoy_auth.CheckRequest) string {
	if req.GetAttributes() == nil || req.GetAttributes().GetRequest() == nil ||
		req.GetAttributes().GetRequest().GetHttp() == nil {
		return ""
	}
	return req.GetAttributes().GetRequest().GetHttp().GetPath()
}

// getMethod extracts the HTTP method from the check request.
func getMethod(req *envoy_auth.CheckRequest) string {
	if req.GetAttributes() == nil || req.GetAttributes().GetRequest() == nil ||
		req.GetAttributes().GetRequest().GetHttp() == nil {
		return ""
	}
	return req.GetAttributes().GetRequest().GetHttp().GetMethod()
}

// okResponseWithToken returns an OK response with a modified Authorization header.
func (s *Server) okResponseWithToken(token string) *envoy_auth.CheckResponse {
	return &envoy_auth.CheckResponse{
		Status: &status.Status{
			Code: int32(codes.OK),
		},
		HttpResponse: &envoy_auth.CheckResponse_OkResponse{
			OkResponse: &envoy_auth.OkHttpResponse{
				Headers: []*envoy_core.HeaderValueOption{
					{
						Header: &envoy_core.HeaderValue{
							Key:   "authorization",
							Value: "Bearer " + token,
						},
						AppendAction: envoy_core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
					},
				},
			},
		},
	}
}

// denyResponse returns a DENY response with the given status code and message.
func (s *Server) denyResponse(code codes.Code, message string) *envoy_auth.CheckResponse {
	return &envoy_auth.CheckResponse{
		Status: &status.Status{
			Code:    int32(code),
			Message: message,
		},
		HttpResponse: &envoy_auth.CheckResponse_DeniedResponse{
			DeniedResponse: &envoy_auth.DeniedHttpResponse{
				Status: &envoy_type.HttpStatus{
					Code: envoy_type.StatusCode(httpStatusFromGRPCCode(code)),
				},
				Body: message,
			},
		},
	}
}

// httpStatusFromGRPCCode converts a gRPC status code to an HTTP status code.
func httpStatusFromGRPCCode(code codes.Code) int32 {
	switch code {
	case codes.OK:
		return 200
	case codes.Unauthenticated:
		return 401
	case codes.PermissionDenied:
		return 403
	case codes.NotFound:
		return 404
	case codes.Internal:
		return 500
	case codes.Unavailable:
		return 503
	default:
		return 500
	}
}
