package token

import (
	"context"
	"fmt"
	"strings"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ServiceAccountIdentity represents a Kubernetes service account identity.
type ServiceAccountIdentity struct {
	// Namespace is the namespace of the service account.
	Namespace string

	// Name is the name of the service account.
	Name string

	// UID is the unique identifier of the service account.
	UID string

	// Username is the full username (e.g., "system:serviceaccount:namespace:name").
	Username string
}

// TokenValidator is the interface for validating tokens.
type TokenValidator interface {
	Validate(ctx context.Context, bearerToken string) (*ServiceAccountIdentity, error)
}

// Validator validates tokens using the Kubernetes TokenReview API.
// This is the legacy method that makes network calls to the workload cluster.
type Validator struct {
	client kubernetes.Interface
}

// NewValidator creates a new token validator using the TokenReview API.
func NewValidator(client kubernetes.Interface) *Validator {
	return &Validator{
		client: client,
	}
}

// Validate validates a bearer token using the Kubernetes TokenReview API.
// It returns the service account identity if the token is valid.
func (v *Validator) Validate(ctx context.Context, bearerToken string) (*ServiceAccountIdentity, error) {
	// Create TokenReview request
	tokenReview := &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token: bearerToken,
		},
	}

	// Call Kubernetes API to validate the token
	result, err := v.client.AuthenticationV1().TokenReviews().Create(ctx, tokenReview, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create token review: %w", err)
	}

	// Check if token is authenticated
	if !result.Status.Authenticated {
		if result.Status.Error != "" {
			return nil, fmt.Errorf("token validation failed: %s", result.Status.Error)
		}
		return nil, fmt.Errorf("token is not authenticated")
	}

	// Extract and parse service account identity
	username := result.Status.User.Username
	identity, err := parseServiceAccountIdentity(username, result.Status.User.UID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse service account identity: %w", err)
	}

	return identity, nil
}

// parseServiceAccountIdentity parses a Kubernetes service account username
// into its component parts.
//
// Expected format: "system:serviceaccount:<namespace>:<name>"
func parseServiceAccountIdentity(username, uid string) (*ServiceAccountIdentity, error) {
	const prefix = "system:serviceaccount:"

	if !strings.HasPrefix(username, prefix) {
		return nil, fmt.Errorf("username %q is not a service account", username)
	}

	// Remove prefix
	remainder := strings.TrimPrefix(username, prefix)

	// Split into namespace and name
	parts := strings.SplitN(remainder, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid service account username format: %q", username)
	}

	namespace := parts[0]
	name := parts[1]

	if namespace == "" || name == "" {
		return nil, fmt.Errorf("namespace or name is empty in username: %q", username)
	}

	return &ServiceAccountIdentity{
		Namespace: namespace,
		Name:      name,
		UID:       uid,
		Username:  username,
	}, nil
}
