package config

import (
	"errors"
	"fmt"

	"github.com/go-jose/go-jose/v4"
)

// ClustersConfig contains configuration for multiple workload clusters.
type ClustersConfig struct {
	// Clusters is the list of workload cluster configurations.
	Clusters []ClusterConfig `yaml:"clusters"`
}

// ClusterConfig defines the configuration for a single workload cluster.
type ClusterConfig struct {
	// Name is a human-readable identifier for the cluster.
	Name string `yaml:"name"`

	// Issuer is the OIDC issuer URL for the cluster (e.g., "https://kubernetes.default.svc").
	// This must match the "iss" claim in tokens from this cluster.
	Issuer string `yaml:"issuer"`

	// JWKSURI is the URL to fetch the JSON Web Key Set from.
	// This is optional if JWKSData is provided inline.
	JWKSURI string `yaml:"jwks_uri,omitempty"`

	// JWKSData contains the JSON Web Key Set data inline.
	// This is optional if JWKSURI is provided.
	// Use this to avoid runtime network calls.
	JWKSData *jose.JSONWebKeySet `yaml:"jwks_data,omitempty"`
}

// Validate checks that the configuration is valid.
func (c *ClustersConfig) Validate() error {
	if len(c.Clusters) == 0 {
		return errors.New("at least one cluster must be configured")
	}

	issuers := make(map[string]bool)
	names := make(map[string]bool)

	for i, cluster := range c.Clusters {
		if err := cluster.Validate(); err != nil {
			return fmt.Errorf("cluster[%d]: %w", i, err)
		}

		// Check for duplicate issuers
		if issuers[cluster.Issuer] {
			return fmt.Errorf("cluster[%d]: duplicate issuer %q", i, cluster.Issuer)
		}
		issuers[cluster.Issuer] = true

		// Check for duplicate names
		if names[cluster.Name] {
			return fmt.Errorf("cluster[%d]: duplicate name %q", i, cluster.Name)
		}
		names[cluster.Name] = true
	}

	return nil
}

// FindByIssuer returns the cluster configuration for the given issuer.
// Returns nil if no cluster matches the issuer.
func (c *ClustersConfig) FindByIssuer(issuer string) *ClusterConfig {
	for i := range c.Clusters {
		if c.Clusters[i].Issuer == issuer {
			return &c.Clusters[i]
		}
	}
	return nil
}

// Validate checks that the cluster configuration is valid.
func (c *ClusterConfig) Validate() error {
	if c.Name == "" {
		return errors.New("name is required")
	}

	if c.Issuer == "" {
		return errors.New("issuer is required")
	}

	// At least one of JWKSURI or JWKSData must be provided
	if c.JWKSURI == "" && c.JWKSData == nil {
		return errors.New("either jwks_uri or jwks_data must be provided")
	}

	// If JWKSData is provided, it must contain at least one key
	if c.JWKSData != nil && len(c.JWKSData.Keys) == 0 {
		return errors.New("jwks_data must contain at least one key")
	}

	return nil
}
