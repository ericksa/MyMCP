package config

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Validate checks the configuration for errors
func (c *Config) Validate() error {
	// Validate server configuration
	if c.MCP.Server.Addr == "" {
		return errors.New("server address cannot be empty")
	}

	// Validate address format and port
	if _, err := net.ResolveTCPAddr("tcp", c.MCP.Server.Addr); err != nil {
		return fmt.Errorf("invalid server address: %v", err)
	}

	// Validate auth configuration
	if c.MCP.Auth.Token == "" {
		return errors.New("auth token cannot be empty")
	}

	// Validate workers base path
	if c.MCP.Workers.BasePath == "" {
		return errors.New("workers base path cannot be empty")
	}

	// Validate shell configuration
	if c.MCP.Workers.Shell.Enabled {
		if len(c.MCP.Workers.Shell.AllowedCommands) == 0 {
			return errors.New("shell allowed_commands cannot be empty when shell is enabled")
		}
		for _, cmd := range c.MCP.Workers.Shell.AllowedCommands {
			if cmd == "" {
				return errors.New("shell allowed_commands contains empty command")
			}
		}
	}

	// Validate MinIO configuration
	if c.MCP.Workers.MinIO.Enabled {
		if c.MCP.Workers.MinIO.Endpoint == "" {
			return errors.New("minio endpoint cannot be empty when minio is enabled")
		}
		if c.MCP.Workers.MinIO.AccessKey == "" {
			return errors.New("minio access key cannot be empty when minio is enabled")
		}
		if c.MCP.Workers.MinIO.SecretKey == "" {
			return errors.New("minio secret key cannot be empty when minio is enabled")
		}
		if c.MCP.Workers.MinIO.DefaultBucket == "" {
			return errors.New("minio default bucket cannot be empty when minio is enabled")
		}
		if !isValidBucketName(c.MCP.Workers.MinIO.DefaultBucket) {
			return fmt.Errorf("invalid minio default bucket name: %s", c.MCP.Workers.MinIO.DefaultBucket)
		}
	}

	// Validate vector configuration
	if c.MCP.Workers.Vector.Enabled {
		if c.MCP.Workers.Vector.Endpoint == "" {
			return errors.New("vector endpoint cannot be empty when vector is enabled")
		}
		if c.MCP.Workers.Vector.DefaultCollection == "" {
			return errors.New("vector default collection cannot be empty when vector is enabled")
		}
		if c.MCP.Workers.Vector.DefaultDimension <= 0 {
			return errors.New("vector default dimension must be positive")
		}
		if c.MCP.Workers.Vector.MaxTopK <= 0 {
			return errors.New("vector max top_k must be positive")
		}
	}

	return nil
}

// isValidBucketName checks if a bucket name is valid according to MinIO/S3 rules
func isValidBucketName(name string) bool {
	if name == "*" {
		return true
	}
	if len(name) < 3 || len(name) > 63 {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`).MatchString(name) {
		return false
	}
	return true
}
