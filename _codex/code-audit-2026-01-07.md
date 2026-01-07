# AIBox Code Audit Report (2026-01-07)

## Scope
- Reviewed: cmd/, internal/, api/proto/, configs/, Dockerfile, docker-compose.yml, docs/plans/2026-01-02-security-hardening.md.
- Tests: not run.

## Findings (ordered by severity)
1) High - Permission checks missing on FileService/AdminService
Evidence:
- internal/service/files.go:30-213 (no auth.RequirePermission)
- internal/service/admin.go:54-96 (no PermissionAdmin checks)
- internal/auth/interceptor.go:34-37 (AdminService/Version skipped entirely)

Impact: any authenticated API key can call file management and readiness/version endpoints regardless of permissions.

Recommendation: enforce PermissionFiles and PermissionAdmin; remove Version from auth skip list.

Patch: Patch A + Patch D

2) High - FileService tenant isolation broken; multi-tenant file APIs unusable
Evidence:
- internal/auth/tenant_interceptor.go:106-115 (tenant_id not extracted for file RPCs)
- internal/service/files.go:110-187 (tenant hardcoded to "default"; CreateStore uses req.ClientId)

Impact: tenant_id validation fails for file RPCs in multi-tenant mode; in legacy mode, stores are created under "default" or client_id, leading to cross-tenant collisions/leakage.

Recommendation: add tenant_id fields to file proto messages, update tenant interceptor, use tenant context in file service.

Patch: Patch B + Patch C + Patch A

3) High - Unbounded file uploads allow memory exhaustion
Evidence: internal/service/files.go:92-108 (stream buffered with bytes.Buffer, no limits)

Impact: DoS via large uploads.

Recommendation: enforce size limits using metadata.Size and chunk counter; optionally stream to temp file.

Patch: Patch A

4) High - Token-per-minute limits can be bypassed (streaming + defaults)
Evidence:
- internal/auth/ratelimit.go:77-81 (limit=0 returns; default TPM ignored)
- internal/service/chat.go:286-346 (no RecordTokens on stream completion)

Impact: TPM limits are not enforced for streaming or for keys without explicit TPM.

Recommendation: apply default TPM in RecordTokens and record usage on stream completion.

Patch: Patch E

5) Medium - Container health checks are broken
Evidence:
- Dockerfile:50-52 uses curl on gRPC port
- docker-compose.yml:36-41 runs `--health-check` flag not implemented in cmd/aibox/main.go

Impact: containers report unhealthy or healthcheck never returns.

Recommendation: implement `--health-check` mode to call AdminService/Health and update Dockerfile.

Patch: Patch F

6) Medium - RAG retrieval ignores configured TopK
Evidence: internal/service/chat.go:548-553 hardcodes TopK=5

Impact: config `rag.retrieval_top_k` has no effect.

Recommendation: pass TopK=0 (use service default) or wire config through.

Patch: Patch G

## Additional notes (low)
- Provider defaults, failover order, logging level/format, and admin_token in configs are not wired into runtime (cmd/aibox/main.go always uses JSON info logging; ChatService ignores config providers). Docker compose exports OPENAI/GEMINI/ANTHROPIC env vars that are not consumed unless tenant configs reference them (configs/aibox.yaml, internal/service/chat.go).
- File store IDs are not validated; consider restricting to a safe pattern before creating Qdrant collections.
- RAG context is appended directly to system instructions; for untrusted documents, consider prompt-injection safeguards.

## Testing gaps
- No tests for file-service authz or tenant_id extraction.
- No tests for upload size limits.
- No tests for streaming token accounting.

## Patch-ready diffs

### Patch A - FileService authz, tenant context, upload limits (internal/service/files.go)
```diff
diff --git a/internal/service/files.go b/internal/service/files.go
--- a/internal/service/files.go
+++ b/internal/service/files.go
@@
-import (
-	"bytes"
-	"context"
-	"fmt"
-	"io"
-	"log/slog"
-	"time"
-
-	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
-	"github.com/cliffpyles/aibox/internal/rag"
-)
+import (
+	"bytes"
+	"context"
+	"fmt"
+	"io"
+	"log/slog"
+	"time"
+
+	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
+	"github.com/cliffpyles/aibox/internal/auth"
+	"github.com/cliffpyles/aibox/internal/rag"
+)
@@
 type FileService struct {
 	pb.UnimplementedFileServiceServer
 
 	ragService *rag.Service
 }
+
+const maxUploadBytes = 100 * 1024 * 1024
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
@@
-	if err := s.ragService.CreateStore(ctx, req.ClientId, storeID); err != nil {
+	tenantID := tenantIDFromContext(ctx)
+	if err := s.ragService.CreateStore(ctx, tenantID, storeID); err != nil {
 		slog.Error("failed to create file store",
-			"client_id", req.ClientId,
+			"tenant_id", tenantID,
+			"client_id", req.ClientId,
 			"store_id", storeID,
 			"error", err,
 		)
@@
 	slog.Info("file store created",
-		"client_id", req.ClientId,
+		"tenant_id", tenantID,
+		"client_id", req.ClientId,
 		"store_id", storeID,
 	)
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
+	if metadata.Size > maxUploadBytes {
+		return fmt.Errorf("file too large: %d bytes (max %d)", metadata.Size, maxUploadBytes)
+	}
@@
-	var buf bytes.Buffer
+	var buf bytes.Buffer
+	var totalBytes int64
 	for {
@@
 		chunk := msg.GetChunk()
 		if chunk == nil {
 			continue
 		}
-		buf.Write(chunk)
+		totalBytes += int64(len(chunk))
+		if totalBytes > maxUploadBytes {
+			return fmt.Errorf("file too large: exceeded %d bytes", maxUploadBytes)
+		}
+		buf.Write(chunk)
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

### Patch B - Add tenant_id to file service proto (api/proto/aibox/v1/files.proto)
```diff
diff --git a/api/proto/aibox/v1/files.proto b/api/proto/aibox/v1/files.proto
--- a/api/proto/aibox/v1/files.proto
+++ b/api/proto/aibox/v1/files.proto
@@
 message CreateFileStoreRequest {
+  // Tenant identification (required for multitenant mode)
+  string tenant_id = 6;
+
   Provider provider = 1;          // Which provider's store to create
   string name = 2;                // Human-readable name
   string client_id = 3;           // Client identifier
   ProviderConfig config = 4;      // Provider configuration (including API key)
@@
 message UploadFileMetadata {
+  // Tenant identification (required for multitenant mode)
+  string tenant_id = 7;
+
   string store_id = 1;            // Target store ID
   string filename = 2;            // Original filename
   string mime_type = 3;           // MIME type (e.g., "application/pdf")
@@
 message DeleteFileStoreRequest {
+  string tenant_id = 5;
   string store_id = 1;
   Provider provider = 2;
   ProviderConfig config = 3;
   bool force = 4;                 // Delete even if files are still being processed
 }
@@
 message GetFileStoreRequest {
+  string tenant_id = 4;
   string store_id = 1;
   Provider provider = 2;
   ProviderConfig config = 3;
 }
@@
 message ListFileStoresRequest {
+  string tenant_id = 6;
   string client_id = 1;
   Provider provider = 2;          // Optional: filter by provider
   ProviderConfig config = 3;
   int32 limit = 4;                // Max results (default 100)
   string page_token = 5;          // Pagination token
 }
```

### Patch C - Tenant interceptor extracts tenant_id for file RPCs (internal/auth/tenant_interceptor.go)
```diff
diff --git a/internal/auth/tenant_interceptor.go b/internal/auth/tenant_interceptor.go
--- a/internal/auth/tenant_interceptor.go
+++ b/internal/auth/tenant_interceptor.go
@@
 func extractTenantID(req interface{}) string {
 	switch r := req.(type) {
 	case *pb.GenerateReplyRequest:
 		return r.TenantId
 	case *pb.SelectProviderRequest:
 		return r.TenantId
+	case *pb.CreateFileStoreRequest:
+		return r.TenantId
+	case *pb.DeleteFileStoreRequest:
+		return r.TenantId
+	case *pb.GetFileStoreRequest:
+		return r.TenantId
+	case *pb.ListFileStoresRequest:
+		return r.TenantId
+	case *pb.UploadFileRequest:
+		if md := r.GetMetadata(); md != nil {
+			return md.TenantId
+		}
 	default:
 		return ""
 	}
 }
```

### Patch D - Enforce admin permissions (internal/service/admin.go, internal/auth/interceptor.go)
```diff
diff --git a/internal/auth/interceptor.go b/internal/auth/interceptor.go
--- a/internal/auth/interceptor.go
+++ b/internal/auth/interceptor.go
@@
 		skipMethods: map[string]bool{
 			"/aibox.v1.AdminService/Health":  true,
-			"/aibox.v1.AdminService/Version": true,
 		},
 	}
 }
```

```diff
diff --git a/internal/service/admin.go b/internal/service/admin.go
--- a/internal/service/admin.go
+++ b/internal/service/admin.go
@@
-import (
-	"context"
-	"time"
-
-	"github.com/cliffpyles/aibox/internal/redis"
-	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
-)
+import (
+	"context"
+	"time"
+
+	"github.com/cliffpyles/aibox/internal/auth"
+	"github.com/cliffpyles/aibox/internal/redis"
+	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
+)
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

### Patch E - TPM default + streaming accounting (internal/auth/ratelimit.go, internal/service/chat.go)
```diff
diff --git a/internal/auth/ratelimit.go b/internal/auth/ratelimit.go
--- a/internal/auth/ratelimit.go
+++ b/internal/auth/ratelimit.go
@@
 func (r *RateLimiter) RecordTokens(ctx context.Context, clientID string, tokens int64, limit int) error {
-	if !r.enabled || limit == 0 {
-		return nil
-	}
+	if !r.enabled {
+		return nil
+	}
+	if limit == 0 {
+		limit = r.defaultLimits.TokensPerMinute
+	}
+	if limit == 0 {
+		return nil
+	}
```

```diff
diff --git a/internal/service/chat.go b/internal/service/chat.go
--- a/internal/service/chat.go
+++ b/internal/service/chat.go
@@
 	case provider.ChunkTypeComplete:
+		if s.rateLimiter != nil && chunk.Usage != nil {
+			if client := auth.ClientFromContext(ctx); client != nil {
+				_ = s.rateLimiter.RecordTokens(ctx, client.ClientID, chunk.Usage.TotalTokens, client.RateLimits.TokensPerMinute)
+			}
+		}
 		pbChunk = &pb.GenerateReplyChunk{
 			Chunk: &pb.GenerateReplyChunk_Complete{
 				Complete: &pb.StreamComplete{
 					ResponseId: chunk.ResponseID,
 					Model:      chunk.Model,
 					Provider:   mapProviderToProto(selectedProvider.Name()),
 					FinalUsage: convertUsage(chunk.Usage),
 				},
 			},
 		}
```

### Patch F - Health-check mode for container probes (cmd/aibox/main.go, Dockerfile)
```diff
diff --git a/cmd/aibox/main.go b/cmd/aibox/main.go
--- a/cmd/aibox/main.go
+++ b/cmd/aibox/main.go
@@
 import (
 	"context"
+	"flag"
 	"fmt"
 	"log/slog"
 	"net"
 	"os"
 	"os/signal"
 	"syscall"
+	"time"
 
+	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
 	"github.com/cliffpyles/aibox/internal/config"
 	"github.com/cliffpyles/aibox/internal/server"
 	"google.golang.org/grpc"
+	"google.golang.org/grpc/credentials"
+	"google.golang.org/grpc/credentials/insecure"
 )
@@
 func main() {
+	healthCheck := flag.Bool("health-check", false, "Check gRPC health and exit")
+	flag.Parse()
+
 	// Set up structured logging
 	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
 		Level: slog.LevelInfo,
 	}))
 	slog.SetDefault(logger)
@@
 	cfg, err := config.Load()
 	if err != nil {
 		slog.Error("failed to load configuration", "error", err)
 		os.Exit(1)
 	}
+
+	if *healthCheck {
+		if err := runHealthCheck(cfg); err != nil {
+			slog.Error("health check failed", "error", err)
+			os.Exit(1)
+		}
+		return
+	}
@@
 }
+
+func runHealthCheck(cfg *config.Config) error {
+	host := cfg.Server.Host
+	if host == "" || host == "0.0.0.0" {
+		host = "127.0.0.1"
+	}
+	addr := fmt.Sprintf("%s:%d", host, cfg.Server.GRPCPort)
+
+	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
+	defer cancel()
+
+	var creds credentials.TransportCredentials
+	if cfg.TLS.Enabled {
+		tlsCreds, err := credentials.NewClientTLSFromFile(cfg.TLS.CertFile, "")
+		if err != nil {
+			return fmt.Errorf("load tls cert: %w", err)
+		}
+		creds = tlsCreds
+	} else {
+		creds = insecure.NewCredentials()
+	}
+
+	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(creds), grpc.WithBlock())
+	if err != nil {
+		return fmt.Errorf("dial %s: %w", addr, err)
+	}
+	defer conn.Close()
+
+	client := pb.NewAdminServiceClient(conn)
+	resp, err := client.Health(ctx, &pb.HealthRequest{})
+	if err != nil {
+		return fmt.Errorf("health rpc: %w", err)
+	}
+	if resp.GetStatus() != "healthy" {
+		return fmt.Errorf("health status %q", resp.GetStatus())
+	}
+	return nil
+}
```

```diff
diff --git a/Dockerfile b/Dockerfile
--- a/Dockerfile
+++ b/Dockerfile
@@
-HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
-    CMD curl -f http://localhost:50051/health || exit 1
+HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
+    CMD /app/aibox --health-check
```

### Patch G - Respect configured RAG TopK (internal/service/chat.go)
```diff
diff --git a/internal/service/chat.go b/internal/service/chat.go
--- a/internal/service/chat.go
+++ b/internal/service/chat.go
@@
 	return s.ragService.Retrieve(ctx, rag.RetrieveParams{
 		StoreID:  storeID,
 		TenantID: tenantID,
 		Query:    query,
-		TopK:     5,
+		TopK:     0,
 	})
 }
```
