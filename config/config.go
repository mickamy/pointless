// Package config provides configuration file support for pointless.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the pointless configuration.
type Config struct {
	Threshold int      `yaml:"threshold"`
	Exclude   []string `yaml:"exclude"`
}

// DefaultConfig returns a config with default values.
func DefaultConfig() Config {
	return Config{
		Threshold: 1024,
		Exclude:   nil,
	}
}

// Load loads configuration from .pointless.yaml in the current directory or parent directories.
func Load() (Config, error) {
	cfg := DefaultConfig()

	path, err := findConfigFile()
	if err != nil {
		return cfg, fmt.Errorf("finding config file: %w", err)
	}

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config file: %w", err)
	}

	return cfg, nil
}

// findConfigFile searches for .pointless.yaml or .pointless.yml in current and parent directories.
func findConfigFile() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}

	for {
		path := filepath.Join(dir, ".pointless.yaml")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}

		// Also check .pointless.yml
		path = filepath.Join(dir, ".pointless.yml")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return "", nil
}

// ShouldExclude checks if a file path matches any exclude pattern.
func (c Config) ShouldExclude(path string) bool {
	for _, pattern := range c.Exclude {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		// Also try matching against the base name
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}

	return false
}
