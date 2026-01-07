package tenant

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func floatPtr(v float64) *float64 {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func TestValidateTenantConfig(t *testing.T) {
	base := TenantConfig{
		TenantID: "tenant",
		Providers: map[string]ProviderConfig{
			"openai": {Enabled: true, APIKey: "key", Model: "model"},
		},
	}

	tests := []struct {
		name    string
		mutate  func(*TenantConfig)
		wantErr bool
	}{
		{"valid config", func(c *TenantConfig) {}, false},
		{"missing tenant id", func(c *TenantConfig) { c.TenantID = "" }, true},
		{"long tenant id", func(c *TenantConfig) { c.TenantID = strings.Repeat("x", 65) }, true},
		{"no enabled provider", func(c *TenantConfig) {
			c.Providers["openai"] = ProviderConfig{Enabled: false}
		}, true},
		{"missing api key", func(c *TenantConfig) {
			c.Providers["openai"] = ProviderConfig{Enabled: true, Model: "model"}
		}, true},
		{"missing model", func(c *TenantConfig) {
			c.Providers["openai"] = ProviderConfig{Enabled: true, APIKey: "key"}
		}, true},
		{"temperature too high", func(c *TenantConfig) {
			p := c.Providers["openai"]
			p.Temperature = floatPtr(3.0)
			c.Providers["openai"] = p
		}, true},
		{"temperature too low", func(c *TenantConfig) {
			p := c.Providers["openai"]
			p.Temperature = floatPtr(-0.5)
			c.Providers["openai"] = p
		}, true},
		{"top_p too high", func(c *TenantConfig) {
			p := c.Providers["openai"]
			p.TopP = floatPtr(1.5)
			c.Providers["openai"] = p
		}, true},
		{"top_p too low", func(c *TenantConfig) {
			p := c.Providers["openai"]
			p.TopP = floatPtr(-0.1)
			c.Providers["openai"] = p
		}, true},
		{"max_output_tokens too low", func(c *TenantConfig) {
			p := c.Providers["openai"]
			p.MaxOutputTokens = intPtr(0)
			c.Providers["openai"] = p
		}, true},
		{"max_output_tokens too high", func(c *TenantConfig) {
			p := c.Providers["openai"]
			p.MaxOutputTokens = intPtr(200000)
			c.Providers["openai"] = p
		}, true},
		{"invalid failover provider", func(c *TenantConfig) {
			c.Failover = FailoverConfig{Enabled: true, Order: []string{"missing"}}
		}, true},
		{"valid temperature", func(c *TenantConfig) {
			p := c.Providers["openai"]
			p.Temperature = floatPtr(0.7)
			c.Providers["openai"] = p
		}, false},
		{"valid top_p", func(c *TenantConfig) {
			p := c.Providers["openai"]
			p.TopP = floatPtr(0.9)
			c.Providers["openai"] = p
		}, false},
		{"valid max_output_tokens", func(c *TenantConfig) {
			p := c.Providers["openai"]
			p.MaxOutputTokens = intPtr(4096)
			c.Providers["openai"] = p
		}, false},
		{"valid failover", func(c *TenantConfig) {
			c.Failover = FailoverConfig{Enabled: true, Order: []string{"openai"}}
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Copy base config
			cfg := base
			cfg.Providers = map[string]ProviderConfig{"openai": base.Providers["openai"]}
			tt.mutate(&cfg)
			err := validateTenantConfig(&cfg)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadTenants_JSONAndYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TEST_API_KEY", "env-value")

	// Create JSON config
	jsonCfg := `{"tenant_id":"t1","providers":{"openai":{"enabled":true,"api_key":"ENV=TEST_API_KEY","model":"gpt-4o"}}}`
	if err := os.WriteFile(filepath.Join(dir, "tenant1.json"), []byte(jsonCfg), 0o600); err != nil {
		t.Fatalf("write json config: %v", err)
	}

	// Create YAML config
	yamlCfg := `tenant_id: t2
providers:
  openai:
    enabled: true
    api_key: inline-key
    model: gpt-4o
`
	if err := os.WriteFile(filepath.Join(dir, "tenant2.yaml"), []byte(yamlCfg), 0o600); err != nil {
		t.Fatalf("write yaml config: %v", err)
	}

	// Create non-config file (should be skipped)
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("skip"), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	configs, err := loadTenants(dir)
	if err != nil {
		t.Fatalf("loadTenants failed: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("expected 2 tenants, got %d", len(configs))
	}

	// Verify JSON config with env resolution
	if configs["t1"].Providers["openai"].APIKey != "env-value" {
		t.Fatalf("expected env-resolved API key, got %q", configs["t1"].Providers["openai"].APIKey)
	}

	// Verify YAML config
	if configs["t2"].Providers["openai"].APIKey != "inline-key" {
		t.Fatalf("expected inline API key, got %q", configs["t2"].Providers["openai"].APIKey)
	}
}

func TestLoadTenants_SkipsEmptyTenantID(t *testing.T) {
	dir := t.TempDir()

	// Config without tenant_id
	noID := `{"providers":{"openai":{"enabled":true,"api_key":"key","model":"gpt-4o"}}}`
	if err := os.WriteFile(filepath.Join(dir, "noid.json"), []byte(noID), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Valid config
	valid := `{"tenant_id":"valid","providers":{"openai":{"enabled":true,"api_key":"key","model":"gpt-4o"}}}`
	if err := os.WriteFile(filepath.Join(dir, "valid.json"), []byte(valid), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	configs, err := loadTenants(dir)
	if err != nil {
		t.Fatalf("loadTenants failed: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 tenant (empty tenant_id skipped), got %d", len(configs))
	}
	if _, ok := configs["valid"]; !ok {
		t.Fatal("expected 'valid' tenant")
	}
}

func TestLoadTenants_DuplicateTenantID(t *testing.T) {
	dir := t.TempDir()

	cfg := `{"tenant_id":"dup","providers":{"openai":{"enabled":true,"api_key":"key","model":"gpt-4o"}}}`
	if err := os.WriteFile(filepath.Join(dir, "tenant1.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write json config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tenant2.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write json config: %v", err)
	}

	if _, err := loadTenants(dir); err == nil {
		t.Fatal("expected duplicate tenant_id error")
	}
}

func TestLoadTenants_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	_, err := loadTenants(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func TestLoadTenants_InvalidJSON(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{invalid json}"), 0o600); err != nil {
		t.Fatalf("write bad json: %v", err)
	}

	_, err := loadTenants(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadTenants_InvalidYAML(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("invalid: yaml: missing: colons"), 0o600); err != nil {
		t.Fatalf("write bad yaml: %v", err)
	}

	_, err := loadTenants(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadTenants_ValidationError(t *testing.T) {
	dir := t.TempDir()

	// Config with missing API key
	cfg := `{"tenant_id":"t1","providers":{"openai":{"enabled":true,"model":"gpt-4o"}}}`
	if err := os.WriteFile(filepath.Join(dir, "tenant.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := loadTenants(dir)
	if err == nil {
		t.Fatal("expected validation error")
	}
}
