package tenant

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AllowedSecretDirs contains the allowed directories for FILE= secret paths.
// Paths outside these directories will be rejected to prevent path traversal.
var AllowedSecretDirs = []string{
	"/etc/aibox/secrets",
	"/run/secrets",
	"/var/run/secrets",
}

// validateSecretPath validates that the path is within allowed directories
// and doesn't contain path traversal sequences.
func validateSecretPath(path string) error {
	// Check for path traversal sequences
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal not allowed: %s", path)
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check if path is within allowed directories
	for _, allowed := range AllowedSecretDirs {
		if strings.HasPrefix(absPath, allowed+string(filepath.Separator)) || absPath == allowed {
			return nil
		}
	}

	return fmt.Errorf("path %s not in allowed directories", absPath)
}

// resolveSecrets loads API keys from ENV=, FILE=, or inline values.
func resolveSecrets(cfg *TenantConfig) error {
	for name, pCfg := range cfg.Providers {
		resolved, err := loadSecret(pCfg.APIKey)
		if err != nil {
			return fmt.Errorf("%s api_key: %w", name, err)
		}
		pCfg.APIKey = resolved
		cfg.Providers[name] = pCfg
	}
	return nil
}

// loadSecret resolves a secret value from ENV=, FILE=, or inline.
func loadSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	// Handle ENV= prefix
	if strings.HasPrefix(value, "ENV=") {
		envVar := strings.TrimPrefix(value, "ENV=")
		v := os.Getenv(envVar)
		if v == "" {
			return "", fmt.Errorf("environment variable %s not set", envVar)
		}
		return v, nil
	}

	// Handle FILE= prefix
	if strings.HasPrefix(value, "FILE=") {
		path := strings.TrimSpace(strings.TrimPrefix(value, "FILE="))

		// Validate path to prevent traversal attacks
		if err := validateSecretPath(path); err != nil {
			return "", fmt.Errorf("secret path validation failed: %w", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", path, err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	// Handle ${VAR} expansion
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		varName := value[2 : len(value)-1]
		v := os.Getenv(varName)
		if v == "" {
			return "", fmt.Errorf("environment variable %s not set", varName)
		}
		return v, nil
	}

	// Return as-is (inline value)
	return value, nil
}
