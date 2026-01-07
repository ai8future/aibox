# AIBox Unit Test Proposal Report (2026-01-07-16)

## Gaps covered
- Auth parsing and permission checks (no Redis dependency)
- Tenant config loading/secret resolution
- Config Load() behavior with env overrides
- ChatService helper logic (provider selection, config merging, RAG formatting)

## Patch-ready diffs
### Add auth parsing and permission tests
```diff
diff --git a/internal/auth/keys_test.go b/internal/auth/keys_test.go
new file mode 100644
index 0000000..fe9da35
--- /dev/null
+++ b/internal/auth/keys_test.go
@@ -0,0 +1,112 @@
+package auth
+
+import (
+	"errors"
+	"strings"
+	"testing"
+)
+
+func TestParseAPIKey(t *testing.T) {
+	tests := []struct {
+		name       string
+		apiKey     string
+		wantID     string
+		wantSecret string
+		wantErr    error
+	}{
+		{
+			name:       "valid key",
+			apiKey:     "aibox_sk_12345678_deadbeef",
+			wantID:     "12345678",
+			wantSecret: "deadbeef",
+		},
+		{
+			name:    "invalid prefix",
+			apiKey:  "bad_sk_12345678_deadbeef",
+			wantErr: ErrInvalidKey,
+		},
+		{
+			name:    "missing underscore",
+			apiKey:  "aibox_sk_12345678deadbeef",
+			wantErr: ErrInvalidKey,
+		},
+		{
+			name:    "too short",
+			apiKey:  "aibox_sk_1",
+			wantErr: ErrInvalidKey,
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			keyID, secret, err := parseAPIKey(tt.apiKey)
+			if tt.wantErr != nil {
+				if err == nil || !errors.Is(err, tt.wantErr) {
+					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
+				}
+				return
+			}
+			if err != nil {
+				t.Fatalf("unexpected error: %v", err)
+			}
+			if keyID != tt.wantID {
+				t.Errorf("keyID=%q, want %q", keyID, tt.wantID)
+			}
+			if secret != tt.wantSecret {
+				t.Errorf("secret=%q, want %q", secret, tt.wantSecret)
+			}
+		})
+	}
+}
+
+func TestGenerateRandomString_LengthAndCharset(t *testing.T) {
+	got, err := generateRandomString(16)
+	if err != nil {
+		t.Fatalf("generateRandomString failed: %v", err)
+	}
+	if len(got) != 16 {
+		t.Fatalf("length=%d, want 16", len(got))
+	}
+	if strings.Trim(got, "0123456789abcdef") != "" {
+		t.Fatalf("expected lowercase hex, got %q", got)
+	}
+}
+
+func TestClientKey_HasPermission(t *testing.T) {
+	key := &ClientKey{Permissions: []Permission{PermissionChat}}
+	if !key.HasPermission(PermissionChat) {
+		t.Error("expected chat permission")
+	}
+	if key.HasPermission(PermissionFiles) {
+		t.Error("did not expect files permission")
+	}
+
+	admin := &ClientKey{Permissions: []Permission{PermissionAdmin}}
+	if !admin.HasPermission(PermissionFiles) {
+		t.Error("admin should have files permission")
+	}
+}
```

```diff
diff --git a/internal/auth/interceptor_test.go b/internal/auth/interceptor_test.go
new file mode 100644
index 0000000..31e25ad
--- /dev/null
+++ b/internal/auth/interceptor_test.go
@@ -0,0 +1,78 @@
+package auth
+
+import (
+	"context"
+	"testing"
+
+	"google.golang.org/grpc/codes"
+	"google.golang.org/grpc/metadata"
+	"google.golang.org/grpc/status"
+)
+
+func TestExtractAPIKey(t *testing.T) {
+	md := metadata.Pairs("authorization", "Bearer token")
+	if got := extractAPIKey(md); got != "token" {
+		t.Fatalf("got %q, want %q", got, "token")
+	}
+
+	md = metadata.Pairs("authorization", "rawtoken")
+	if got := extractAPIKey(md); got != "rawtoken" {
+		t.Fatalf("got %q, want %q", got, "rawtoken")
+	}
+
+	md = metadata.Pairs("x-api-key", "apikey")
+	if got := extractAPIKey(md); got != "apikey" {
+		t.Fatalf("got %q, want %q", got, "apikey")
+	}
+
+	md = metadata.MD{}
+	if got := extractAPIKey(md); got != "" {
+		t.Fatalf("got %q, want empty", got)
+	}
+}
+
+func TestRequirePermission(t *testing.T) {
+	if err := RequirePermission(context.Background(), PermissionChat); status.Code(err) != codes.Unauthenticated {
+		t.Fatalf("expected unauthenticated, got %v", err)
+	}
+
+	ctx := context.WithValue(context.Background(), ClientContextKey, &ClientKey{
+		Permissions: []Permission{PermissionChat},
+	})
+	if err := RequirePermission(ctx, PermissionChat); err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+
+	ctx = context.WithValue(context.Background(), ClientContextKey, &ClientKey{
+		Permissions: []Permission{PermissionFiles},
+	})
+	if err := RequirePermission(ctx, PermissionChat); status.Code(err) != codes.PermissionDenied {
+		t.Fatalf("expected permission denied, got %v", err)
+	}
+}
```

### Add ChatService helper coverage
```diff
diff --git a/internal/service/chat_helpers_test.go b/internal/service/chat_helpers_test.go
new file mode 100644
index 0000000..c8e0c0d
--- /dev/null
+++ b/internal/service/chat_helpers_test.go
@@ -0,0 +1,151 @@
+package service
+
+import (
+	"context"
+	"strings"
+	"testing"
+
+	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
+	"github.com/cliffpyles/aibox/internal/auth"
+	"github.com/cliffpyles/aibox/internal/rag"
+	"github.com/cliffpyles/aibox/internal/tenant"
+)
+
+func TestFormatRAGContext(t *testing.T) {
+	chunks := []rag.RetrieveResult{{Filename: "doc.txt", Text: "hello"}}
+	result := formatRAGContext(chunks)
+	if !strings.Contains(result, "Relevant context") {
+		t.Fatal("expected context header")
+	}
+	if !strings.Contains(result, "doc.txt") {
+		t.Fatal("expected filename in context")
+	}
+}
+
+func TestRAGChunksToCitations_TruncatesSnippet(t *testing.T) {
+	longText := strings.Repeat("a", 250)
+	citations := ragChunksToCitations([]rag.RetrieveResult{{Filename: "doc.txt", Text: longText}})
+	if len(citations) != 1 {
+		t.Fatalf("expected 1 citation, got %d", len(citations))
+	}
+	if !strings.HasSuffix(citations[0].Snippet, "...") {
+		t.Fatalf("expected truncated snippet to end with ...")
+	}
+}
+
+func TestBuildProviderConfig_Overrides(t *testing.T) {
+	svc := NewChatService(nil, nil)
+	tenantTemp := 0.2
+	tenantTopP := 0.9
+	tenantMax := 1000
+	tenantCfg := tenant.TenantConfig{
+		TenantID: "tenant1",
+		Providers: map[string]tenant.ProviderConfig{
+			"openai": {
+				Enabled:         true,
+				APIKey:          "tenant-key",
+				Model:           "gpt-4",
+				Temperature:     &tenantTemp,
+				TopP:            &tenantTopP,
+				MaxOutputTokens: &tenantMax,
+				BaseURL:         "https://tenant.example",
+				ExtraOptions:    map[string]string{"reasoning_effort": "low"},
+			},
+		},
+	}
+	ctx := context.WithValue(context.Background(), auth.TenantContextKey, &tenantCfg)
+
+	overrideTemp := 0.7
+	overrideMax := int32(2048)
+	req := &pb.GenerateReplyRequest{
+		ProviderConfigs: map[string]*pb.ProviderConfig{
+			"openai": {
+				ApiKey:          "override-key",
+				Model:           "gpt-4o",
+				Temperature:     &overrideTemp,
+				MaxOutputTokens: &overrideMax,
+				ExtraOptions:    map[string]string{"service_tier": "priority"},
+			},
+		},
+	}
+
+	cfg := svc.buildProviderConfig(ctx, req, "openai")
+	if cfg.APIKey != "override-key" {
+		t.Fatalf("APIKey=%q, want override-key", cfg.APIKey)
+	}
+	if cfg.Model != "gpt-4o" {
+		t.Fatalf("Model=%q, want gpt-4o", cfg.Model)
+	}
+	if cfg.Temperature == nil || *cfg.Temperature != overrideTemp {
+		t.Fatalf("Temperature=%v, want %v", cfg.Temperature, overrideTemp)
+	}
+	if cfg.MaxOutputTokens == nil || *cfg.MaxOutputTokens != int(overrideMax) {
+		t.Fatalf("MaxOutputTokens=%v, want %d", cfg.MaxOutputTokens, overrideMax)
+	}
+	if cfg.ExtraOptions["reasoning_effort"] != "low" {
+		t.Fatalf("expected tenant extra option to persist")
+	}
+	if cfg.ExtraOptions["service_tier"] != "priority" {
+		t.Fatalf("expected override extra option to be set")
+	}
+}
+
+func TestSelectProviderWithTenant_DefaultProvider(t *testing.T) {
+	svc := NewChatService(nil, nil)
+	tenantCfg := tenant.TenantConfig{
+		TenantID: "tenant1",
+		Providers: map[string]tenant.ProviderConfig{
+			"openai": {Enabled: true, APIKey: "k", Model: "m"},
+		},
+		Failover: tenant.FailoverConfig{Enabled: true, Order: []string{"openai"}},
+	}
+	ctx := context.WithValue(context.Background(), auth.TenantContextKey, &tenantCfg)
+
+	provider, err := svc.selectProviderWithTenant(ctx, &pb.GenerateReplyRequest{})
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+	if provider.Name() != "openai" {
+		t.Fatalf("provider=%q, want openai", provider.Name())
+	}
+}
+
+func TestSelectProviderWithTenant_DisabledProvider(t *testing.T) {
+	svc := NewChatService(nil, nil)
+	tenantCfg := tenant.TenantConfig{
+		TenantID: "tenant1",
+		Providers: map[string]tenant.ProviderConfig{
+			"openai": {Enabled: true, APIKey: "k", Model: "m"},
+		},
+	}
+	ctx := context.WithValue(context.Background(), auth.TenantContextKey, &tenantCfg)
+
+	_, err := svc.selectProviderWithTenant(ctx, &pb.GenerateReplyRequest{
+		PreferredProvider: pb.Provider_PROVIDER_GEMINI,
+	})
+	if err == nil {
+		t.Fatal("expected error for disabled provider")
+	}
+}
```

### Add tenant secret and manager coverage
```diff
diff --git a/internal/tenant/secrets_test.go b/internal/tenant/secrets_test.go
new file mode 100644
index 0000000..2618d7a
--- /dev/null
+++ b/internal/tenant/secrets_test.go
@@ -0,0 +1,70 @@
+package tenant
+
+import (
+	"os"
+	"path/filepath"
+	"testing"
+)
+
+func TestLoadSecret(t *testing.T) {
+	t.Setenv("TEST_SECRET", "env-value")
+	got, err := loadSecret("ENV=TEST_SECRET")
+	if err != nil {
+		t.Fatalf("ENV secret failed: %v", err)
+	}
+	if got != "env-value" {
+		t.Fatalf("got %q, want env-value", got)
+	}
+
+	filePath := filepath.Join(t.TempDir(), "secret.txt")
+	if err := os.WriteFile(filePath, []byte("file-value\n"), 0o600); err != nil {
+		t.Fatalf("write file: %v", err)
+	}
+	got, err = loadSecret("FILE=" + filePath)
+	if err != nil {
+		t.Fatalf("FILE secret failed: %v", err)
+	}
+	if got != "file-value" {
+		t.Fatalf("got %q, want file-value", got)
+	}
+
+	t.Setenv("TEST_VAR", "var-value")
+	got, err = loadSecret("${TEST_VAR}")
+	if err != nil {
+		t.Fatalf("VAR secret failed: %v", err)
+	}
+	if got != "var-value" {
+		t.Fatalf("got %q, want var-value", got)
+	}
+
+	got, err = loadSecret("inline")
+	if err != nil {
+		t.Fatalf("inline secret failed: %v", err)
+	}
+	if got != "inline" {
+		t.Fatalf("got %q, want inline", got)
+	}
+
+	_, err = loadSecret("ENV=MISSING_SECRET")
+	if err == nil {
+		t.Fatal("expected error for missing env secret")
+	}
+}
```

```diff
diff --git a/internal/tenant/manager_test.go b/internal/tenant/manager_test.go
new file mode 100644
index 0000000..a39a4b8
--- /dev/null
+++ b/internal/tenant/manager_test.go
@@ -0,0 +1,80 @@
+package tenant
+
+import (
+	"os"
+	"path/filepath"
+	"testing"
+)
+
+func TestLoadAndReload(t *testing.T) {
+	dir := t.TempDir()
+	writeTenantConfig(t, dir, "tenant1.json", "tenant1")
+
+	t.Setenv("AIBOX_CONFIGS_DIR", dir)
+
+	mgr, err := Load("")
+	if err != nil {
+		t.Fatalf("Load failed: %v", err)
+	}
+	if mgr.TenantCount() != 1 {
+		t.Fatalf("TenantCount=%d, want 1", mgr.TenantCount())
+	}
+	if _, ok := mgr.Tenant("tenant1"); !ok {
+		t.Fatal("expected tenant1 to exist")
+	}
+
+	writeTenantConfig(t, dir, "tenant2.json", "tenant2")
+	diff, err := mgr.Reload()
+	if err != nil {
+		t.Fatalf("Reload failed: %v", err)
+	}
+	if len(diff.Added) != 1 || diff.Added[0] != "tenant2" {
+		t.Fatalf("unexpected diff.Added: %v", diff.Added)
+	}
+}
+
+func writeTenantConfig(t *testing.T, dir, filename, tenantID string) {
+	t.Helper()
+	content := []byte("{" +
+		"\"tenant_id\":\"" + tenantID + "\"," +
+		"\"providers\":{\"openai\":{\"enabled\":true,\"api_key\":\"k\",\"model\":\"m\"}}" +
+	"}")
+	if err := os.WriteFile(filepath.Join(dir, filename), content, 0o600); err != nil {
+		t.Fatalf("writeTenantConfig: %v", err)
+	}
+}
```

### Add config Load() coverage
```diff
diff --git a/internal/config/config_test.go b/internal/config/config_test.go
new file mode 100644
index 0000000..ba6df09
--- /dev/null
+++ b/internal/config/config_test.go
@@ -0,0 +1,70 @@
+package config
+
+import (
+	"os"
+	"path/filepath"
+	"testing"
+)
+
+func TestLoad_ConfigFileAndEnvOverrides(t *testing.T) {
+	dir := t.TempDir()
+	path := filepath.Join(dir, "config.yaml")
+	content := []byte("server:\n  grpc_port: 6000\n  host: 127.0.0.1\n" +
+		"tls:\n  enabled: true\n  cert_file: \"${TEST_CERT_FILE}\"\n  key_file: \"${TEST_KEY_FILE}\"\n" +
+		"auth:\n  admin_token: \"${TEST_ADMIN_TOKEN}\"\n")
+	if err := os.WriteFile(path, content, 0o600); err != nil {
+		t.Fatalf("write config: %v", err)
+	}
+
+	t.Setenv("AIBOX_CONFIG", path)
+	t.Setenv("AIBOX_GRPC_PORT", "7000")
+	t.Setenv("TEST_CERT_FILE", "/tmp/cert.pem")
+	t.Setenv("TEST_KEY_FILE", "/tmp/key.pem")
+	t.Setenv("TEST_ADMIN_TOKEN", "secret")
+
+	cfg, err := Load()
+	if err != nil {
+		t.Fatalf("Load failed: %v", err)
+	}
+	if cfg.Server.GRPCPort != 7000 {
+		t.Fatalf("GRPCPort=%d, want 7000", cfg.Server.GRPCPort)
+	}
+	if cfg.TLS.CertFile != "/tmp/cert.pem" {
+		t.Fatalf("CertFile=%q, want /tmp/cert.pem", cfg.TLS.CertFile)
+	}
+	if cfg.Auth.AdminToken != "secret" {
+		t.Fatalf("AdminToken=%q, want secret", cfg.Auth.AdminToken)
+	}
+}
+
+func TestLoad_MissingConfigUsesDefaults(t *testing.T) {
+	path := filepath.Join(t.TempDir(), "missing.yaml")
+	t.Setenv("AIBOX_CONFIG", path)
+
+	cfg, err := Load()
+	if err != nil {
+		t.Fatalf("Load failed: %v", err)
+	}
+	if cfg.Server.GRPCPort != 50051 {
+		t.Fatalf("GRPCPort=%d, want 50051", cfg.Server.GRPCPort)
+	}
+}
```
