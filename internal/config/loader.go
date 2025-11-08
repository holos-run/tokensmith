package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadClustersConfig loads and validates a clusters configuration from a YAML file.
func LoadClustersConfig(path string) (*ClustersConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ClustersConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}
