package tenant

import "testing"

func TestTenantConfigGetProvider(t *testing.T) {
	cfg := TenantConfig{
		Providers: map[string]ProviderConfig{
			"openai": {Enabled: true, APIKey: "key", Model: "model"},
			"gemini": {Enabled: false},
		},
	}

	t.Run("enabled provider", func(t *testing.T) {
		providerCfg, ok := cfg.GetProvider("openai")
		if !ok {
			t.Fatal("expected enabled provider to return true")
		}
		if providerCfg.APIKey != "key" {
			t.Fatalf("APIKey = %q, want %q", providerCfg.APIKey, "key")
		}
		if providerCfg.Model != "model" {
			t.Fatalf("Model = %q, want %q", providerCfg.Model, "model")
		}
	})

	t.Run("disabled provider", func(t *testing.T) {
		if _, ok := cfg.GetProvider("gemini"); ok {
			t.Fatal("expected disabled provider to return false")
		}
	})

	t.Run("unknown provider", func(t *testing.T) {
		if _, ok := cfg.GetProvider("unknown"); ok {
			t.Fatal("expected unknown provider to return false")
		}
	})
}

func TestTenantConfigDefaultProvider_FailoverOrder(t *testing.T) {
	cfg := TenantConfig{
		Failover: FailoverConfig{Enabled: true, Order: []string{"gemini", "openai"}},
		Providers: map[string]ProviderConfig{
			"openai": {Enabled: true, APIKey: "key", Model: "model"},
			"gemini": {Enabled: true, APIKey: "key2", Model: "model2"},
		},
	}

	name, providerCfg, ok := cfg.DefaultProvider()
	if !ok {
		t.Fatal("expected default provider to exist")
	}
	if name != "gemini" {
		t.Fatalf("expected failover order provider (gemini), got %q", name)
	}
	if providerCfg.APIKey != "key2" {
		t.Fatalf("APIKey = %q, want %q", providerCfg.APIKey, "key2")
	}
}

func TestTenantConfigDefaultProvider_FailoverSkipsDisabled(t *testing.T) {
	cfg := TenantConfig{
		Failover: FailoverConfig{Enabled: true, Order: []string{"gemini", "openai"}},
		Providers: map[string]ProviderConfig{
			"openai": {Enabled: true, APIKey: "key", Model: "model"},
			"gemini": {Enabled: false, APIKey: "key2", Model: "model2"},
		},
	}

	name, _, ok := cfg.DefaultProvider()
	if !ok {
		t.Fatal("expected default provider to exist")
	}
	if name != "openai" {
		t.Fatalf("expected openai (gemini disabled), got %q", name)
	}
}

func TestTenantConfigDefaultProvider_FallbackNoFailover(t *testing.T) {
	cfg := TenantConfig{
		Providers: map[string]ProviderConfig{
			"openai": {Enabled: true, APIKey: "key", Model: "model"},
			"gemini": {Enabled: false},
		},
	}

	name, _, ok := cfg.DefaultProvider()
	if !ok {
		t.Fatal("expected fallback provider")
	}
	if name != "openai" {
		t.Fatalf("expected fallback provider (openai), got %q", name)
	}
}

func TestTenantConfigDefaultProvider_NoEnabledProviders(t *testing.T) {
	cfg := TenantConfig{
		Providers: map[string]ProviderConfig{
			"openai": {Enabled: false},
			"gemini": {Enabled: false},
		},
	}

	_, _, ok := cfg.DefaultProvider()
	if ok {
		t.Fatal("expected no default provider when all disabled")
	}
}
