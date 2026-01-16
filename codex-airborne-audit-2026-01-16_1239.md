# Airborne Code Audit

Date Created: 2026-01-16 12:39:21 +0100

## Scope
- Repo root: /Users/cliff/Desktop/_code/airborne
- Reviewed Go services, auth, config, RAG, providers, and deployment artifacts.
- Excluded per agent rules: _studies, _proposals, _codex, _claude, _rcodegen.

## Method
- Manual review of gRPC entrypoints, auth/tenant middleware, provider clients, and RAG services.
- Targeted searches for SSRF, auth bypass, input validation gaps, and resource exhaustion.
- No code changes applied; patch-ready diffs provided below.

## Findings (ordered by severity)

### 1) High: SSRF bypass via hostname resolution in base URL validation
**Location:** `internal/validation/url.go:51`

**What/Why:** `ValidateProviderURL` only blocks private IPs when the hostname is a literal IP. Hostnames that resolve to private/loopback/metadata IPs can bypass the check (DNS rebinding / internal DNS). This undermines SSRF defenses for any feature that accepts `base_url` overrides.

**Impact:** An attacker who can provide a custom base URL could reach internal services (including link-local metadata) by using a hostname that resolves to a private IP.

**Recommendation:** Resolve hostnames during validation and reject any private/loopback/link-local/metadata targets, while still allowing explicit `localhost` for dev.

**Patch-ready diff:**
```diff
diff --git a/internal/validation/url.go b/internal/validation/url.go
index 2e90090..f86b43f 100644
--- a/internal/validation/url.go
+++ b/internal/validation/url.go
@@
-import (
+import (
+	"context"
 	"errors"
 	"fmt"
 	"net"
 	"net/url"
 	"strings"
 )
@@
-	// Parse IP address if it's a direct IP
-	if ip := net.ParseIP(hostname); ip != nil {
-		// Allow localhost IPs for http://
-		if isLocalhost {
-			return nil
-		}
-
-		// Block private IPs
-		if isPrivateIP(ip) {
-			return fmt.Errorf("%w: %s is in a private IP range", ErrPrivateIP, hostname)
-		}
-	}
-
-	return nil
+	// Parse IP address if it's a direct IP
+	if ip := net.ParseIP(hostname); ip != nil {
+		// Allow localhost IPs for http://
+		if isLocalhost {
+			return nil
+		}
+
+		// Block private IPs
+		if isPrivateIP(ip) {
+			return fmt.Errorf("%w: %s is in a private IP range", ErrPrivateIP, hostname)
+		}
+		return nil
+	}
+
+	// Resolve hostnames to prevent SSRF via internal DNS/rebinding.
+	if err := validateResolvedHost(hostname); err != nil {
+		return err
+	}
+
+	return nil
 }
+
+func validateResolvedHost(hostname string) error {
+	ips, err := net.DefaultResolver.LookupIPAddr(context.Background(), hostname)
+	if err != nil {
+		return fmt.Errorf("%w: unable to resolve host %q", ErrInvalidURL, hostname)
+	}
+	for _, ipAddr := range ips {
+		if ipAddr.IP.IsLoopback() && !isLocalhostHost(hostname) {
+			return fmt.Errorf("%w: loopback address is not allowed for %q", ErrPrivateIP, hostname)
+		}
+		if isMetadataEndpoint(ipAddr.IP.String()) {
+			return fmt.Errorf("%w: %s is blocked", ErrMetadataEndpoint, ipAddr.IP.String())
+		}
+		if isPrivateIP(ipAddr.IP) {
+			return fmt.Errorf("%w: %s is in a private IP range", ErrPrivateIP, ipAddr.IP.String())
+		}
+	}
+	return nil
+}
```

### 2) High: Unprivileged base_url overrides in FileService (SSRF pivot)
**Location:** `internal/service/files.go:57`, `internal/service/files.go:175`

**What/Why:** FileService methods accept `ProviderConfig.base_url` from requests without admin gating. This enables untrusted callers with file permissions to redirect outbound requests to arbitrary hosts. Combined with the DNS gap above, this is a direct SSRF risk.

**Impact:** FileService callers can route server-side HTTP calls to internal or sensitive endpoints.

**Recommendation:** Require `PermissionAdmin` for any request that sets a custom base URL and validate it early. Mirror the ChatService behavior.

**Patch-ready diff:**
```diff
diff --git a/internal/service/files.go b/internal/service/files.go
index 2dcbd46..bd1af04 100644
--- a/internal/service/files.go
+++ b/internal/service/files.go
@@
-import (
-	"context"
-	"crypto/rand"
-	"encoding/hex"
-	"fmt"
-	"io"
-	"log/slog"
-	"os"
-	"time"
+import (
+	"context"
+	"crypto/rand"
+	"encoding/hex"
+	"fmt"
+	"io"
+	"log/slog"
+	"os"
+	"strings"
+	"time"
@@
 	"github.com/ai8future/airborne/internal/auth"
 	"github.com/ai8future/airborne/internal/provider/gemini"
 	"github.com/ai8future/airborne/internal/provider/openai"
 	"github.com/ai8future/airborne/internal/rag"
+	"github.com/ai8future/airborne/internal/validation"
 	"google.golang.org/grpc/codes"
 	"google.golang.org/grpc/status"
 )
@@
 func NewFileService(ragService *rag.Service, rateLimiter *auth.RateLimiter) *FileService {
 	return &FileService{
 		ragService:  ragService,
 		rateLimiter: rateLimiter,
 	}
 }
+
+// authorizeCustomBaseURL ensures only admins can set custom provider base URLs.
+func authorizeCustomBaseURL(ctx context.Context, cfg *pb.ProviderConfig) error {
+	if cfg == nil {
+		return nil
+	}
+	baseURL := strings.TrimSpace(cfg.GetBaseUrl())
+	if baseURL == "" {
+		return nil
+	}
+	if err := auth.RequirePermission(ctx, auth.PermissionAdmin); err != nil {
+		return err
+	}
+	if err := validation.ValidateProviderURL(baseURL); err != nil {
+		return status.Error(codes.InvalidArgument, err.Error())
+	}
+	return nil
+}
@@
 func (s *FileService) CreateFileStore(ctx context.Context, req *pb.CreateFileStoreRequest) (*pb.CreateFileStoreResponse, error) {
 	// Check permission
 	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
 		return nil, err
 	}
+	if err := authorizeCustomBaseURL(ctx, req.Config); err != nil {
+		return nil, err
+	}
@@
 func (s *FileService) UploadFile(stream pb.FileService_UploadFileServer) error {
@@
 	metadata := firstMsg.GetMetadata()
 	if metadata == nil {
 		return fmt.Errorf("first message must contain metadata")
 	}
+	if err := authorizeCustomBaseURL(ctx, metadata.Config); err != nil {
+		return err
+	}
@@
 func (s *FileService) DeleteFileStore(ctx context.Context, req *pb.DeleteFileStoreRequest) (*pb.DeleteFileStoreResponse, error) {
 	// Check permission
 	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
 		return nil, err
 	}
+	if err := authorizeCustomBaseURL(ctx, req.Config); err != nil {
+		return nil, err
+	}
@@
 func (s *FileService) GetFileStore(ctx context.Context, req *pb.GetFileStoreRequest) (*pb.GetFileStoreResponse, error) {
 	// Check permission
 	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
 		return nil, err
 	}
+	if err := authorizeCustomBaseURL(ctx, req.Config); err != nil {
+		return nil, err
+	}
@@
 func (s *FileService) ListFileStores(ctx context.Context, req *pb.ListFileStoresRequest) (*pb.ListFileStoresResponse, error) {
 	// Check permission
 	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
 		return nil, err
 	}
+	if err := authorizeCustomBaseURL(ctx, req.Config); err != nil {
+		return nil, err
+	}
```

### 3) Medium: Gemini FileSearch uses http.DefaultClient and buffers entire uploads
**Location:** `internal/provider/gemini/filestore.go:115`, `internal/provider/gemini/filestore.go:168`

**What/Why:** FileSearch operations use `http.DefaultClient` (no timeouts) and read the entire file into memory before upload. With large uploads, this can stall requests indefinitely and spike memory usage.

**Impact:** Potential resource exhaustion and stuck goroutines when the remote endpoint is slow or unresponsive.

**Recommendation:** Use a dedicated HTTP client with timeouts and stream file data directly.

**Patch-ready diff:**
```diff
diff --git a/internal/provider/gemini/filestore.go b/internal/provider/gemini/filestore.go
index 34f5f7b..7b07f8d 100644
--- a/internal/provider/gemini/filestore.go
+++ b/internal/provider/gemini/filestore.go
@@
 const (
 	fileSearchBaseURL         = "https://generativelanguage.googleapis.com/v1beta"
 	fileSearchPollingInterval = 2 * time.Second
 	fileSearchPollingTimeout  = 5 * time.Minute
+	fileSearchHTTPTimeout     = 2 * time.Minute
 )
+
+var fileSearchHTTPClient = &http.Client{
+	Timeout: fileSearchHTTPTimeout,
+}
@@
-	resp, err := http.DefaultClient.Do(req)
+	resp, err := fileSearchHTTPClient.Do(req)
@@
-	// Read the file content
-	fileContent, err := io.ReadAll(content)
-	if err != nil {
-		return nil, fmt.Errorf("read file content: %w", err)
-	}
-
 	// Use the upload endpoint with multipart
 	baseURL := cfg.getBaseURL()
@@
-	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(fileContent))
+	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, content)
@@
-	resp, err := http.DefaultClient.Do(req)
+	resp, err := fileSearchHTTPClient.Do(req)
@@
-		resp2, err := http.DefaultClient.Do(req2)
+		resp2, err := fileSearchHTTPClient.Do(req2)
@@
-			req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, url, nil)
+			req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, url, nil)
@@
-			resp, err := http.DefaultClient.Do(req)
+			resp, err := fileSearchHTTPClient.Do(req)
@@
-	resp, err := http.DefaultClient.Do(req)
+	resp, err := fileSearchHTTPClient.Do(req)
@@
-	resp, err := http.DefaultClient.Do(req)
+	resp, err := fileSearchHTTPClient.Do(req)
@@
-	resp, err := http.DefaultClient.Do(req)
+	resp, err := fileSearchHTTPClient.Do(req)
```

### 4) Medium: API key IDs are short and unchecked for collisions
**Location:** `internal/auth/keys.go:76`, `internal/auth/keys.go:249`

**What/Why:** Key IDs are fixed at 8 hex chars (32 bits) with no collision check. A collision would overwrite an existing key in Redis, potentially granting access to the wrong client.

**Impact:** Low-probability but high-impact key confusion as the system scales.

**Recommendation:** Increase key ID length and check for uniqueness on generation. Allow variable-length key IDs in parsing for backward compatibility.

**Patch-ready diff:**
```diff
diff --git a/internal/auth/keys.go b/internal/auth/keys.go
index 12f5a50..bbaf4b7 100644
--- a/internal/auth/keys.go
+++ b/internal/auth/keys.go
@@
 import (
 	"context"
 	"crypto/rand"
 	"encoding/hex"
 	"encoding/json"
 	"fmt"
+	"strings"
 	"time"
@@
 const (
 	defaultKeyPrefix = "aibox:key:"
+	keyIDLength      = 16
+	minKeyIDLength    = 8
+	maxKeyIDAttempts  = 5
 )
@@
 func (s *KeyStore) GenerateAPIKey(ctx context.Context, clientID, clientName string, permissions []Permission, limits RateLimits) (string, *ClientKey, error) {
 	// Generate key ID and secret
-	keyID, err := generateRandomString(8)
+	keyID, err := s.generateUniqueKeyID(ctx)
 	if err != nil {
 		return "", nil, fmt.Errorf("failed to generate key ID: %w", err)
 	}
@@
 	secret, err := generateRandomString(32)
 	if err != nil {
 		return "", nil, fmt.Errorf("failed to generate secret: %w", err)
 	}
@@
 	fullKey := fmt.Sprintf("aibox_sk_%s_%s", keyID, secret)
 	return fullKey, key, nil
 }
+
+func (s *KeyStore) generateUniqueKeyID(ctx context.Context) (string, error) {
+	for attempt := 0; attempt < maxKeyIDAttempts; attempt++ {
+		keyID, err := generateRandomString(keyIDLength)
+		if err != nil {
+			return "", err
+		}
+		exists, err := s.redis.Exists(ctx, s.keyPrefix+keyID)
+		if err != nil {
+			return "", fmt.Errorf("check key ID uniqueness: %w", err)
+		}
+		if exists == 0 {
+			return keyID, nil
+		}
+	}
+	return "", fmt.Errorf("failed to generate unique key ID")
+}
@@
 func parseAPIKey(apiKey string) (keyID, secret string, err error) {
 	// Expected format: aibox_sk_KEYID_SECRET
 	if len(apiKey) < 20 {
 		return "", "", ErrInvalidKey
 	}
@@
-	// Extract keyID (8 chars) and secret (rest)
-	remainder := apiKey[9:]
-	if len(remainder) < 10 { // keyID(8) + _(1) + secret(1+)
-		return "", "", ErrInvalidKey
-	}
-
-	keyID = remainder[:8]
-	if remainder[8] != '_' {
-		return "", "", ErrInvalidKey
-	}
-	secret = remainder[9:]
+	// Extract keyID and secret (split on first underscore)
+	remainder := apiKey[9:]
+	parts := strings.SplitN(remainder, "_", 2)
+	if len(parts) != 2 {
+		return "", "", ErrInvalidKey
+	}
+	keyID = parts[0]
+	secret = parts[1]
+	if len(keyID) < minKeyIDLength || secret == "" {
+		return "", "", ErrInvalidKey
+	}
 
 	return keyID, secret, nil
 }
@@
 func generateRandomString(length int) (string, error) {
-	bytes := make([]byte, length)
+	bytes := make([]byte, (length+1)/2)
 	if _, err := rand.Read(bytes); err != nil {
 		return "", err
 	}
-	return hex.EncodeToString(bytes)[:length], nil
+	encoded := hex.EncodeToString(bytes)
+	return encoded[:length], nil
 }
```

### 5) Low: Raw provider errors returned to clients on failover
**Location:** `internal/service/chat.go:201`

**What/Why:** When failover succeeds, `OriginalError` returns the raw provider error string. This can leak internal/provider details to clients.

**Impact:** Information disclosure and inconsistent error hygiene (streaming errors are sanitized, failover errors are not).

**Recommendation:** Sanitize the original error before returning it.

**Patch-ready diff:**
```diff
diff --git a/internal/service/chat.go b/internal/service/chat.go
index 25ed5d1..1f3f7b4 100644
--- a/internal/service/chat.go
+++ b/internal/service/chat.go
@@
 				prepared.params.Config = s.buildProviderConfig(ctx, req, fallbackProvider.Name())
 				fallbackResult, fallbackErr := fallbackProvider.GenerateReply(ctx, prepared.params)
 				if fallbackErr == nil {
-					return s.buildResponse(fallbackResult, fallbackProvider.Name(), true, prepared.provider.Name(), err.Error()), nil
+					sanitized := sanitize.SanitizeForClient(err)
+					return s.buildResponse(fallbackResult, fallbackProvider.Name(), true, prepared.provider.Name(), sanitized), nil
 				}
```

### 6) Low: Misleading env var name in static auth error message
**Location:** `internal/server/grpc.go:83`

**What/Why:** Static auth error message refers to `AIBOX_ADMIN_TOKEN`, while config uses `AIRBORNE_ADMIN_TOKEN`. This can cause misconfiguration during incident response.

**Recommendation:** Correct the error string.

**Patch-ready diff:**
```diff
diff --git a/internal/server/grpc.go b/internal/server/grpc.go
index 4e4db5e..3a98cdf 100644
--- a/internal/server/grpc.go
+++ b/internal/server/grpc.go
@@
-		if cfg.Auth.AdminToken == "" {
-			return nil, nil, fmt.Errorf("AIBOX_ADMIN_TOKEN required for static auth mode")
+		if cfg.Auth.AdminToken == "" {
+			return nil, nil, fmt.Errorf("AIRBORNE_ADMIN_TOKEN required for static auth mode")
 		}
```

## Design/Architecture Notes (no code diff)
- **FileService tenant isolation:** Tenant validation is skipped for FileService methods (`internal/auth/tenant_interceptor.go:33`). In multi-tenant setups, internal stores use `TenantIDFromContext`, which falls back to client ID or `default`. This can unintentionally mix file stores across tenants or clients. Consider adding `tenant_id` to FileService request messages and enforcing tenant validation via the interceptor, then regenerating protobufs.

## Additional Observations
- TLS is disabled by default in `configs/airborne.yaml`; ensure TLS is enforced in production deployments.
- `docker-compose.yml` leaves `AIRBORNE_ADMIN_TOKEN` empty by default, which will prevent startup in static auth mode.

## Suggested Validation (not run)
1) `go test ./...`
2) `go test -race ./...`
3) If SSRF hardening is applied, add tests for `ValidateProviderURL` covering DNS resolution and loopback cases.

