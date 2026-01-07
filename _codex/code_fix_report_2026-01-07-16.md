# Code Fix Report (2026-01-07-16)

## Overview
- Scope: manual review of core runtime, tenant, RAG, and file service code paths.
- Tests: not run (changes are provided as patch-ready diffs only).
- Note: No code was modified in the workspace per instruction; patches are provided for application.

## Findings (ordered by severity)

### 1) FileService uses inconsistent namespaces and skips permission checks
**Impact**: File stores are created under one namespace (req.ClientId) while upload/delete/get always use "default". This makes stores unreachable when `client_id` != "default" and creates cross-client data exposure in authenticated environments. Additionally, `PermissionFiles` is defined but never enforced, so any authenticated key can access file operations.

**Fix**: Derive a stable store namespace from the authenticated client ID (when present) and use it consistently across Create/Upload/Delete/Get. Enforce `PermissionFiles` on all FileService endpoints. If no auth context exists, fall back to "default" to keep dev behavior consistent.

**Patch**:
```diff
diff --git a/internal/service/files.go b/internal/service/files.go
--- a/internal/service/files.go
+++ b/internal/service/files.go
@@
 	"log/slog"
 	"time"
 
 	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
+	"github.com/cliffpyles/aibox/internal/auth"
 	"github.com/cliffpyles/aibox/internal/rag"
 )
@@
 func (s *FileService) CreateFileStore(ctx context.Context, req *pb.CreateFileStoreRequest) (*pb.CreateFileStoreResponse, error) {
-	if req.ClientId == "" {
-		return nil, fmt.Errorf("client_id is required")
-	}
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return nil, err
+	}
+
+	tenantID := fileStoreTenantID(ctx)
@@
-	if err := s.ragService.CreateStore(ctx, req.ClientId, storeID); err != nil {
+	if err := s.ragService.CreateStore(ctx, tenantID, storeID); err != nil {
 		slog.Error("failed to create file store",
-			"client_id", req.ClientId,
+			"tenant_id", tenantID,
 			"store_id", storeID,
 			"error", err,
 		)
 		return nil, fmt.Errorf("create store: %w", err)
 	}
@@
 	slog.Info("file store created",
-		"client_id", req.ClientId,
+		"tenant_id", tenantID,
 		"store_id", storeID,
 	)
@@
 func (s *FileService) UploadFile(stream pb.FileService_UploadFileServer) error {
 	ctx := stream.Context()
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return err
+	}
@@
-	// Extract tenant ID from context or use a default
-	// In a real implementation, this would come from the auth interceptor
-	tenantID := "default"
+	tenantID := fileStoreTenantID(ctx)
@@
 func (s *FileService) DeleteFileStore(ctx context.Context, req *pb.DeleteFileStoreRequest) (*pb.DeleteFileStoreResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return nil, err
+	}
@@
-	// Extract tenant ID from context
-	tenantID := "default"
+	tenantID := fileStoreTenantID(ctx)
@@
 func (s *FileService) GetFileStore(ctx context.Context, req *pb.GetFileStoreRequest) (*pb.GetFileStoreResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return nil, err
+	}
@@
-	// Extract tenant ID from context
-	tenantID := "default"
+	tenantID := fileStoreTenantID(ctx)
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
+
+func fileStoreTenantID(ctx context.Context) string {
+	if client := auth.ClientFromContext(ctx); client != nil && client.ClientID != "" {
+		return client.ClientID
+	}
+	return "default"
+}
```

**Test updates** (permission context + client scoping):
```diff
diff --git a/internal/service/files_test.go b/internal/service/files_test.go
--- a/internal/service/files_test.go
+++ b/internal/service/files_test.go
@@
 	"context"
 	"fmt"
 	"io"
 	"testing"
 
 	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
+	"github.com/cliffpyles/aibox/internal/auth"
 	"github.com/cliffpyles/aibox/internal/rag"
 	"github.com/cliffpyles/aibox/internal/rag/extractor"
 	"github.com/cliffpyles/aibox/internal/rag/testutil"
 	"github.com/cliffpyles/aibox/internal/rag/vectorstore"
 )
@@
-	resp, err := svc.CreateFileStore(context.Background(), req)
+	resp, err := svc.CreateFileStore(ctxWithFilePermission("default"), req)
@@
-	resp, err := svc.CreateFileStore(context.Background(), req)
+	resp, err := svc.CreateFileStore(ctxWithFilePermission("default"), req)
@@
-func TestFileService_CreateFileStore_MissingClientID(t *testing.T) {
+func TestFileService_CreateFileStore_MissingClientID(t *testing.T) {
 	mockRAG := createMockRAGService()
 	svc := NewFileService(mockRAG)
@@
-	_, err := svc.CreateFileStore(context.Background(), req)
-
-	if err == nil {
-		t.Fatal("expected error for missing client_id")
-	}
+	_, err := svc.CreateFileStore(ctxWithFilePermission("default"), req)
+	if err != nil {
+		t.Fatalf("unexpected error for missing client_id: %v", err)
+	}
 }
@@
-	_, err := svc.CreateFileStore(context.Background(), req)
+	_, err := svc.CreateFileStore(ctxWithFilePermission("default"), req)
@@
-	resp, err := svc.DeleteFileStore(context.Background(), req)
+	resp, err := svc.DeleteFileStore(ctxWithFilePermission("default"), req)
@@
-	_, err := svc.DeleteFileStore(context.Background(), req)
+	_, err := svc.DeleteFileStore(ctxWithFilePermission("default"), req)
@@
-	resp, err := svc.DeleteFileStore(context.Background(), req)
+	resp, err := svc.DeleteFileStore(ctxWithFilePermission("default"), req)
@@
-	resp, err := svc.GetFileStore(context.Background(), req)
+	resp, err := svc.GetFileStore(ctxWithFilePermission("default"), req)
@@
-	_, err := svc.GetFileStore(context.Background(), req)
+	_, err := svc.GetFileStore(ctxWithFilePermission("default"), req)
@@
-	_, err := svc.GetFileStore(context.Background(), req)
+	_, err := svc.GetFileStore(ctxWithFilePermission("default"), req)
@@
-	resp, err := svc.ListFileStores(context.Background(), req)
+	resp, err := svc.ListFileStores(ctxWithFilePermission("default"), req)
@@
-		ctx: context.Background(),
+		ctx: ctxWithFilePermission("default"),
@@
-		ctx: context.Background(),
+		ctx: ctxWithFilePermission("default"),
@@
-		ctx: context.Background(),
+		ctx: ctxWithFilePermission("default"),
@@
-		ctx: context.Background(),
+		ctx: ctxWithFilePermission("default"),
@@
-		ctx: context.Background(),
+		ctx: ctxWithFilePermission("default"),
@@
-		ctx:      context.Background(),
+		ctx:      ctxWithFilePermission("default"),
@@
 func createRAGServiceWithMocks(
 	store *testutil.MockStore,
 	embedder *testutil.MockEmbedder,
 	extractor *testutil.MockExtractor,
 ) *rag.Service {
@@
 }
+
+func ctxWithFilePermission(clientID string) context.Context {
+	return context.WithValue(context.Background(), auth.ClientContextKey, &auth.ClientKey{
+		ClientID:    clientID,
+		Permissions: []auth.Permission{auth.PermissionFiles},
+	})
+}
```

### 2) Tenant ID normalization mismatch breaks lookups
**Impact**: `TenantInterceptor.resolveTenant` lowercases incoming `tenant_id`, but `loadTenants` stores tenant IDs as-is. Mixed-case IDs in config will never match, leading to `tenant not found` even when configured.

**Fix**: Normalize tenant IDs to lowercase and trimmed form during load so lookups and duplicate detection are consistent.

**Patch**:
```diff
diff --git a/internal/tenant/loader.go b/internal/tenant/loader.go
--- a/internal/tenant/loader.go
+++ b/internal/tenant/loader.go
@@
-		// Skip files without tenant_id (e.g., shared config files)
-		if cfg.TenantID == "" {
+		cfg.TenantID = strings.TrimSpace(cfg.TenantID)
+		// Skip files without tenant_id (e.g., shared config files)
+		if cfg.TenantID == "" {
 			continue
 		}
+		cfg.TenantID = strings.ToLower(cfg.TenantID)
```

### 3) Chunker can panic when no chunk is appended
**Impact**: If `chunkText` is smaller than `MinChunkSize` and no chunk has been appended yet, `chunks[len(chunks)-1]` panics. This can happen on whitespace-heavy segments or unusual text layouts, crashing ingestion.

**Fix**: Guard the overlap backtracking logic when no chunks exist.

**Patch**:
```diff
diff --git a/internal/rag/chunker/chunker.go b/internal/rag/chunker/chunker.go
--- a/internal/rag/chunker/chunker.go
+++ b/internal/rag/chunker/chunker.go
@@
 	// Move start forward, accounting for overlap
 	start = end - opts.Overlap
-	if start <= chunks[len(chunks)-1].Start {
+	if len(chunks) > 0 && start <= chunks[len(chunks)-1].Start {
 		// Prevent infinite loop if overlap is too large
 		start = end
 	}
```

### 4) RAG_ENABLED env var cannot disable a true config
**Impact**: If a config file enables RAG, setting `RAG_ENABLED=false` has no effect because the env override only turns the feature on. This makes disabling via env impossible in deployments.

**Fix**: Parse `RAG_ENABLED` with `strconv.ParseBool` and set the boolean when present.

**Patch**:
```diff
diff --git a/internal/config/config.go b/internal/config/config.go
--- a/internal/config/config.go
+++ b/internal/config/config.go
@@
-	// RAG configuration
-	if enabled := os.Getenv("RAG_ENABLED"); enabled == "true" || enabled == "1" {
-		c.RAG.Enabled = true
-	}
+	// RAG configuration
+	if enabled := os.Getenv("RAG_ENABLED"); enabled != "" {
+		if v, err := strconv.ParseBool(enabled); err == nil {
+			c.RAG.Enabled = v
+		}
+	}
```

## Notes / Follow-ups
- FileService still does not support multi-tenant selection because the proto lacks `tenant_id`; the fixes above use authenticated client ID as a namespace to reduce cross-client leakage.
- If you want `client_id` to remain required for CreateFileStore, consider adding `client_id` to Upload/Delete/Get requests (proto change) so the namespace stays consistent.
