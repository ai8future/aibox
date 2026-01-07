# AIBox Code Audit Report (2026-01-07-15)

## Scope
- Reviewed Go services, auth, tenancy, providers, RAG, configs, Docker assets, and proto definitions.
- Skipped `_studies` and `_proposals` per AGENTS instructions.

## Method
- Static review only; no tests or builds executed.

## Summary of findings
- High: 1
- Medium: 6
- Low: 3

## Findings

### F-01 (High) FileService tenancy and size enforcement gaps break isolation and enable DoS
- Evidence: `internal/service/files.go`, `internal/auth/tenant_interceptor.go`.
- Impact:
  - CreateFileStore uses `client_id` as tenant, while Upload/Get/Delete use a hard-coded `default` tenant. Stores created via CreateFileStore are never used by Upload/Get/Delete, and Delete never targets the created store. This breaks basic functionality and multi-tenant isolation.
  - UploadFile streams are unbounded; a client can stream arbitrarily large payloads across many chunks, causing memory exhaustion.
  - File endpoints lack explicit permission checks, so any authenticated key can call file operations.
- Recommendation:
  - Resolve tenant consistently from context and allow tenant selection via gRPC metadata headers for FileService (until proto is extended).
  - Enforce upload size caps using metadata.Size and a hard upper bound.
  - Require `PermissionFiles` on all file endpoints.
- Patch-ready diff:
```diff
diff --git a/internal/service/files.go b/internal/service/files.go
index 574f8ab..5b427f7 100644
--- a/internal/service/files.go
+++ b/internal/service/files.go
@@
 import (
 	"bytes"
 	"context"
 	"fmt"
 	"io"
 	"log/slog"
 	"time"
 
 	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
+	"github.com/cliffpyles/aibox/internal/auth"
 	"github.com/cliffpyles/aibox/internal/rag"
 )
+
+const maxUploadBytes int64 = 100 * 1024 * 1024
@@
 func NewFileService(ragService *rag.Service) *FileService {
 	return &FileService{
 		ragService: ragService,
 	}
 }
+
+func tenantIDFromContext(ctx context.Context) string {
+	if cfg := auth.TenantFromContext(ctx); cfg != nil && cfg.TenantID != "" {
+		return cfg.TenantID
+	}
+	return "default"
+}
@@
 func (s *FileService) CreateFileStore(ctx context.Context, req *pb.CreateFileStoreRequest) (*pb.CreateFileStoreResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return nil, err
+	}
 	if req.ClientId == "" {
 		return nil, fmt.Errorf("client_id is required")
 	}
+
+	tenantID := tenantIDFromContext(ctx)
@@
-	if err := s.ragService.CreateStore(ctx, req.ClientId, storeID); err != nil {
+	if err := s.ragService.CreateStore(ctx, tenantID, storeID); err != nil {
 		slog.Error("failed to create file store",
 			"client_id", req.ClientId,
 			"store_id", storeID,
 			"error", err,
@@
 func (s *FileService) UploadFile(stream pb.FileService_UploadFileServer) error {
 	ctx := stream.Context()
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return err
+	}
@@
 	if metadata.Filename == "" {
 		return fmt.Errorf("filename is required")
 	}
+	if metadata.Size < 0 {
+		return fmt.Errorf("size must be >= 0")
+	}
+	if metadata.Size > maxUploadBytes {
+		return fmt.Errorf("file exceeds max size (%d bytes)", maxUploadBytes)
+	}
@@
 	// Collect file chunks
 	var buf bytes.Buffer
+	var received int64
 	for {
 		msg, err := stream.Recv()
 		if err == io.EOF {
 			break
 		}
@@
 		chunk := msg.GetChunk()
 		if chunk == nil {
 			continue
 		}
+		received += int64(len(chunk))
+		if received > maxUploadBytes {
+			return fmt.Errorf("file exceeds max size (%d bytes)", maxUploadBytes)
+		}
+		if metadata.Size > 0 && received > metadata.Size {
+			return fmt.Errorf("uploaded bytes exceed declared size")
+		}
 		buf.Write(chunk)
 	}
@@
-	tenantID := "default"
+	tenantID := tenantIDFromContext(ctx)
@@
 func (s *FileService) DeleteFileStore(ctx context.Context, req *pb.DeleteFileStoreRequest) (*pb.DeleteFileStoreResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return nil, err
+	}
 	if req.StoreId == "" {
 		return nil, fmt.Errorf("store_id is required")
 	}
 
 	// Extract tenant ID from context
-	tenantID := "default"
+	tenantID := tenantIDFromContext(ctx)
@@
 func (s *FileService) GetFileStore(ctx context.Context, req *pb.GetFileStoreRequest) (*pb.GetFileStoreResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return nil, err
+	}
 	if req.StoreId == "" {
 		return nil, fmt.Errorf("store_id is required")
 	}
 
 	// Extract tenant ID from context
-	tenantID := "default"
+	tenantID := tenantIDFromContext(ctx)
@@
 func (s *FileService) ListFileStores(ctx context.Context, req *pb.ListFileStoresRequest) (*pb.ListFileStoresResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return nil, err
+	}
 	// For now, return empty list - would need to implement collection listing in Qdrant
 	// This would require storing metadata about stores separately
 	return &pb.ListFileStoresResponse{
 		Stores: []*pb.FileStoreSummary{},
 	}, nil
 }
```
```diff
diff --git a/internal/auth/tenant_interceptor.go b/internal/auth/tenant_interceptor.go
index 234f84c..9b9e2c6 100644
--- a/internal/auth/tenant_interceptor.go
+++ b/internal/auth/tenant_interceptor.go
@@
 import (
 	"context"
 	"strings"
 
 	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
 	"github.com/cliffpyles/aibox/internal/tenant"
 	"google.golang.org/grpc"
 	"google.golang.org/grpc/codes"
+	"google.golang.org/grpc/metadata"
 	"google.golang.org/grpc/status"
 )
@@
-		tenantID := extractTenantID(req)
+		tenantID := extractTenantID(ctx, req)
@@
-		tenantID := extractTenantID(m)
+		tenantID := extractTenantID(s.ServerStream.Context(), m)
@@
-func extractTenantID(req interface{}) string {
+func extractTenantID(ctx context.Context, req interface{}) string {
 	switch r := req.(type) {
 	case *pb.GenerateReplyRequest:
 		return r.TenantId
 	case *pb.SelectProviderRequest:
 		return r.TenantId
 	default:
-		return ""
+		if md, ok := metadata.FromIncomingContext(ctx); ok {
+			if values := md.Get("x-tenant-id"); len(values) > 0 {
+				return values[0]
+			}
+			if values := md.Get("tenant-id"); len(values) > 0 {
+				return values[0]
+			}
+		}
+		return ""
 	}
 }
```

### F-02 (Medium) Admin endpoints are overly permissive
- Evidence: `internal/auth/interceptor.go`, `internal/service/admin.go`.
- Impact: AdminService Version is currently unauthenticated, and Ready is authenticated but not authorization-checked. Any API key can access readiness details and build metadata.
- Recommendation: Require `PermissionAdmin` for Version and Ready; keep Health unauthenticated if desired for liveness.
- Patch-ready diff:
```diff
diff --git a/internal/auth/interceptor.go b/internal/auth/interceptor.go
index 47d9abc..e539b52 100644
--- a/internal/auth/interceptor.go
+++ b/internal/auth/interceptor.go
@@
 		skipMethods: map[string]bool{
-			"/aibox.v1.AdminService/Health":  true,
-			"/aibox.v1.AdminService/Version": true,
+			"/aibox.v1.AdminService/Health": true,
 		},
 	}
 }
```
```diff
diff --git a/internal/service/admin.go b/internal/service/admin.go
index 9f1fb0f..0d361d6 100644
--- a/internal/service/admin.go
+++ b/internal/service/admin.go
@@
 import (
 	"context"
 	"time"
 
+	"github.com/cliffpyles/aibox/internal/auth"
 	"github.com/cliffpyles/aibox/internal/redis"
 	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
 )
@@
 func (s *AdminService) Ready(ctx context.Context, req *pb.ReadyRequest) (*pb.ReadyResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionAdmin); err != nil {
+		return nil, err
+	}
 	dependencies := make(map[string]*pb.DependencyStatus)
@@
 func (s *AdminService) Version(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionAdmin); err != nil {
+		return nil, err
+	}
 	return &pb.VersionResponse{
 		Version:   s.version,
 		GitCommit: s.gitCommit,
 		BuildTime: s.buildTime,
 		GoVersion: s.goVersion,
 	}, nil
 }
```

### F-03 (Medium) Tenant config load failures silently fall back to legacy mode
- Evidence: `internal/server/grpc.go`.
- Impact: In production, an invalid or missing tenant config can cause the server to start without tenancy enforcement, potentially bypassing isolation expectations.
- Recommendation: Fail fast in production when tenant config loading fails.
- Patch-ready diff:
```diff
diff --git a/internal/server/grpc.go b/internal/server/grpc.go
index 4c0eb5e..f3c6a1a 100644
--- a/internal/server/grpc.go
+++ b/internal/server/grpc.go
@@
 	tenantMgr, err := tenant.Load("")
 	if err != nil {
-		slog.Warn("tenant config not loaded - running in single-tenant legacy mode", "error", err)
-		// Create an empty manager for legacy mode
-		tenantMgr = nil
+		if cfg.StartupMode.IsProduction() {
+			return nil, fmt.Errorf("tenant config required in production: %w", err)
+		}
+		slog.Warn("tenant config not loaded - running in single-tenant legacy mode", "error", err)
+		tenantMgr = nil
 	} else {
 		slog.Info("tenant configurations loaded",
 			"tenant_count", tenantMgr.TenantCount(),
 			"tenants", tenantMgr.TenantCodes(),
 		)
 	}
```

### F-04 (Medium) Token-per-minute defaults are never applied
- Evidence: `internal/auth/ratelimit.go`.
- Impact: If client keys omit `tpm`, token usage is effectively unlimited even if defaults are configured.
- Recommendation: Apply default TPM in RecordTokens when per-client limit is not set.
- Patch-ready diff:
```diff
diff --git a/internal/auth/ratelimit.go b/internal/auth/ratelimit.go
index d3a9150..7c88c3e 100644
--- a/internal/auth/ratelimit.go
+++ b/internal/auth/ratelimit.go
@@
 func (r *RateLimiter) RecordTokens(ctx context.Context, clientID string, tokens int64, limit int) error {
-	if !r.enabled || limit == 0 {
+	if !r.enabled {
 		return nil
 	}
+	if limit == 0 {
+		limit = r.defaultLimits.TokensPerMinute
+	}
+	if limit == 0 {
+		return nil
+	}
@@
 	// Check if over limit (return error but don't block - already processed)
 	if int(count) > limit {
 		return ErrRateLimitExceeded
 	}
```

### F-05 (Medium) Request-supplied `base_url` enables SSRF-like outbound calls
- Evidence: `internal/service/chat.go`.
- Impact: If untrusted clients can set `provider_configs.base_url`, the server can be coerced into contacting arbitrary endpoints.
- Recommendation: Restrict `base_url` overrides to admin keys or an explicit allowlist.
- Patch-ready diff (also fixes F-09):
```diff
diff --git a/internal/service/chat.go b/internal/service/chat.go
index 52a7c84..e4e5c2b 100644
--- a/internal/service/chat.go
+++ b/internal/service/chat.go
@@
 func (s *ChatService) GenerateReply(ctx context.Context, req *pb.GenerateReplyRequest) (*pb.GenerateReplyResponse, error) {
 	// Check permission
 	if err := auth.RequirePermission(ctx, auth.PermissionChat); err != nil {
 		return nil, err
 	}
+	if hasCustomBaseURL(req) {
+		if err := auth.RequirePermission(ctx, auth.PermissionAdmin); err != nil {
+			return nil, err
+		}
+	}
@@
 func (s *ChatService) GenerateReplyStream(req *pb.GenerateReplyRequest, stream pb.AIBoxService_GenerateReplyStreamServer) error {
 	ctx := stream.Context()
 
 	// Check permission
 	if err := auth.RequirePermission(ctx, auth.PermissionChatStream); err != nil {
 		return err
 	}
+	if hasCustomBaseURL(req) {
+		if err := auth.RequirePermission(ctx, auth.PermissionAdmin); err != nil {
+			return err
+		}
+	}
@@
 	return s.ragService.Retrieve(ctx, rag.RetrieveParams{
 		StoreID:  storeID,
 		TenantID: tenantID,
 		Query:    query,
-		TopK:     5,
+		TopK:     0,
 	})
 }
@@
 func mapProviderFromString(name string) pb.Provider {
 	return mapProviderToProto(name)
 }
+
+func hasCustomBaseURL(req *pb.GenerateReplyRequest) bool {
+	for _, cfg := range req.ProviderConfigs {
+		if cfg != nil && strings.TrimSpace(cfg.GetBaseUrl()) != "" {
+			return true
+		}
+	}
+	return false
+}
```

### F-06 (Medium) OpenAI polling can hang without a deadline
- Evidence: `internal/provider/openai/client.go`.
- Impact: If OpenAI never transitions to a terminal status, requests can hang indefinitely (no timeout on poll loop).
- Recommendation: Use a bounded context for polling.
- Patch-ready diff:
```diff
diff --git a/internal/provider/openai/client.go b/internal/provider/openai/client.go
index b9b6e2e..b6f2d0b 100644
--- a/internal/provider/openai/client.go
+++ b/internal/provider/openai/client.go
@@
-		// Wait for completion
-		resp, err = waitForCompletion(ctx, client, resp)
+		// Wait for completion
+		pollCtx, pollCancel := context.WithTimeout(ctx, requestTimeout)
+		resp, err = waitForCompletion(pollCtx, client, resp)
+		pollCancel()
 		if err != nil {
 			lastErr = err
 			slog.Warn("openai wait error", "attempt", attempt, "error", err)
 			continue
 		}
```

### F-07 (Medium) Docker health checks are invalid
- Evidence: `Dockerfile`, `docker-compose.yml`.
- Impact: Container health checks always fail (HTTP check against gRPC port; unsupported `--health-check` flag), leading to unhealthy containers and restarts.
- Recommendation: Replace with a simple TCP check or implement a proper gRPC health endpoint.
- Patch-ready diff (TCP check):
```diff
diff --git a/Dockerfile b/Dockerfile
index 8d7ad9d..eb4a3c2 100644
--- a/Dockerfile
+++ b/Dockerfile
@@
-RUN apk add --no-cache ca-certificates tzdata curl
+RUN apk add --no-cache ca-certificates tzdata curl netcat-openbsd
@@
-HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
-    CMD curl -f http://localhost:50051/health || exit 1
+HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
+    CMD nc -z 127.0.0.1 50051 || exit 1
```
```diff
diff --git a/docker-compose.yml b/docker-compose.yml
index edcff87..cbbcc52 100644
--- a/docker-compose.yml
+++ b/docker-compose.yml
@@
     healthcheck:
-      test: ["CMD", "/app/aibox", "--health-check"]
+      test: ["CMD", "sh", "-c", "nc -z 127.0.0.1 50051"]
       interval: 30s
       timeout: 10s
       retries: 3
       start_period: 10s
```

### F-08 (Low) Go directive uses a patch version
- Evidence: `go.mod`.
- Impact: The Go toolchain expects `go 1.xx` format; patch values can break tooling and IDEs.
- Recommendation: Use `go 1.25` (or the intended major.minor) to match the toolchain and Docker image.
- Patch-ready diff:
```diff
diff --git a/go.mod b/go.mod
index bce12c8..06be784 100644
--- a/go.mod
+++ b/go.mod
@@
-go 1.25.5
+go 1.25
```

### F-09 (Low) RAG retrieval TopK ignores configuration
- Evidence: `internal/service/chat.go`.
- Impact: `cfg.RAG.RetrievalTopK` is set but never used; retrieval always uses 5.
- Recommendation: Let `rag.Service` default TopK apply by passing 0. (Included in F-05 diff.)

### F-10 (Low) API key generation wastes entropy
- Evidence: `internal/auth/keys.go`.
- Impact: `generateRandomString` allocates more random bytes than needed, then truncates, which is misleading and inefficient.
- Recommendation: Generate only the bytes needed for the requested hex length.
- Patch-ready diff:
```diff
diff --git a/internal/auth/keys.go b/internal/auth/keys.go
index 9b79a9f..1a4d9c4 100644
--- a/internal/auth/keys.go
+++ b/internal/auth/keys.go
@@
 func generateRandomString(length int) (string, error) {
-	bytes := make([]byte, length)
+	if length <= 0 {
+		return "", nil
+	}
+	byteLen := (length + 1) / 2
+	bytes := make([]byte, byteLen)
 	if _, err := rand.Read(bytes); err != nil {
 		return "", err
 	}
 	return hex.EncodeToString(bytes)[:length], nil
 }
```

## Testing gaps
- No tests cover admin authorization on `AdminService.Ready` and `AdminService.Version`.
- No tests cover FileService tenant scoping or upload size enforcement.
- No tests cover `base_url` override restrictions or related security checks.
- No tests cover OpenAI poll timeout behavior.

## Suggested validation steps
1) `go test ./...`
2) Exercise file upload with oversized payloads to confirm enforcement.
3) Verify admin endpoints require admin permission after changes.
4) Validate Docker health checks transition to healthy with a running server.
