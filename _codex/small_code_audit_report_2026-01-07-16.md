# AIBox Security and Code Quality Audit (2026-01-07-16)

## Scope
- Reviewed: cmd, internal/{auth,service,rag,tenant,config,server,validation,redis}, api/proto

## High Severity
### H-1 Missing authorization gates on AdminService and FileService
- Impact: any authenticated key (or unauthenticated in dev mode) can access admin/version and manage file stores, bypassing the permission model.
- Evidence: `internal/service/admin.go` and `internal/service/files.go` do not call `auth.RequirePermission`.
- Fix: require `PermissionAdmin` for Ready/Version and `PermissionFiles` for all file methods; tighten the auth skip list so Version is not bypassed.

Patch-ready diff:
```diff
diff --git a/internal/auth/interceptor.go b/internal/auth/interceptor.go
index 4a7e4c1..2bd3a0f 100644
--- a/internal/auth/interceptor.go
+++ b/internal/auth/interceptor.go
@@ -36,8 +36,7 @@ func NewAuthenticator(keyStore *KeyStore, rateLimiter *RateLimiter) *Authenticat
 		keyStore:    keyStore,
 		rateLimiter: rateLimiter,
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
index 61c63b1..e671021 100644
--- a/internal/service/admin.go
+++ b/internal/service/admin.go
@@ -5,9 +5,10 @@ import (
 	"context"
 	"time"
 
-	"github.com/cliffpyles/aibox/internal/redis"
 	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
+	"github.com/cliffpyles/aibox/internal/auth"
+	"github.com/cliffpyles/aibox/internal/redis"
 )
@@ -48,6 +49,10 @@ func (s *AdminService) Health(ctx context.Context, req *pb.HealthRequest) (*pb.H
 }
 
 // Ready returns readiness status with dependency checks.
 func (s *AdminService) Ready(ctx context.Context, req *pb.ReadyRequest) (*pb.ReadyResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionAdmin); err != nil {
+		return nil, err
+	}
+
 	dependencies := make(map[string]*pb.DependencyStatus)
@@ -86,6 +91,10 @@ func (s *AdminService) Ready(ctx context.Context, req *pb.ReadyRequest) (*pb.Rea
 }
 
 // Version returns detailed version information.
 func (s *AdminService) Version(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionAdmin); err != nil {
+		return nil, err
+	}
+
 	return &pb.VersionResponse{
 		Version:   s.version,
```

```diff
diff --git a/internal/service/files.go b/internal/service/files.go
index 5577ebd..91b61bb 100644
--- a/internal/service/files.go
+++ b/internal/service/files.go
@@ -6,11 +6,13 @@ import (
 	"context"
 	"fmt"
 	"io"
 	"log/slog"
+	"strings"
 	"time"
 
 	pb "github.com/cliffpyles/aibox/gen/go/aibox/v1"
+	"github.com/cliffpyles/aibox/internal/auth"
 	"github.com/cliffpyles/aibox/internal/rag"
 )
@@ -25,6 +27,8 @@ type FileService struct {
 	ragService *rag.Service
 }
 
+const maxUploadBytes int64 = 100 * 1024 * 1024
+
 // NewFileService creates a new file service.
 func NewFileService(ragService *rag.Service) *FileService {
 	return &FileService{
@@ -37,6 +41,10 @@ func NewFileService(ragService *rag.Service) *FileService {
 // CreateFileStore creates a new vector store (Qdrant collection).
 func (s *FileService) CreateFileStore(ctx context.Context, req *pb.CreateFileStoreRequest) (*pb.CreateFileStoreResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return nil, err
+	}
+
 	if req.ClientId == "" {
 		return nil, fmt.Errorf("client_id is required")
 	}
@@ -49,7 +57,8 @@ func (s *FileService) CreateFileStore(ctx context.Context, req *pb.CreateFileSto
 		storeID = fmt.Sprintf("store_%d", time.Now().UnixNano())
 	}
 
-	// Create the Qdrant collection via RAG service
-	if err := s.ragService.CreateStore(ctx, req.ClientId, storeID); err != nil {
+	// Create the Qdrant collection via RAG service
+	tenantID := resolveTenantID(ctx, req.ClientId)
+	if err := s.ragService.CreateStore(ctx, tenantID, storeID); err != nil {
 		slog.Error("failed to create file store",
 			"client_id", req.ClientId,
 			"store_id", storeID,
@@ -79,6 +88,10 @@ func (s *FileService) CreateFileStore(ctx context.Context, req *pb.CreateFileSto
 func (s *FileService) UploadFile(stream pb.FileService_UploadFileServer) error {
 	ctx := stream.Context()
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return err
+	}
 
 	// First message should be metadata
 	firstMsg, err := stream.Recv()
@@ -100,6 +113,9 @@ func (s *FileService) UploadFile(stream pb.FileService_UploadFileServer) error {
 	if metadata.Filename == "" {
 		return fmt.Errorf("filename is required")
 	}
+	if metadata.Size > 0 && metadata.Size > maxUploadBytes {
+		return fmt.Errorf("file exceeds max upload size (%d bytes)", maxUploadBytes)
+	}
@@ -127,6 +143,9 @@ func (s *FileService) UploadFile(stream pb.FileService_UploadFileServer) error {
 		if chunk == nil {
 			continue
 		}
+		if int64(buf.Len())+int64(len(chunk)) > maxUploadBytes {
+			return fmt.Errorf("file exceeds max upload size (%d bytes)", maxUploadBytes)
+		}
 		buf.Write(chunk)
 	}
 
 	// Extract tenant ID from context or use a default
 	// In a real implementation, this would come from the auth interceptor
-	tenantID := "default"
+	tenantID := resolveTenantID(ctx, "")
@@ -177,6 +196,10 @@ func (s *FileService) UploadFile(stream pb.FileService_UploadFileServer) error {
 // DeleteFileStore deletes a store and all its contents.
 func (s *FileService) DeleteFileStore(ctx context.Context, req *pb.DeleteFileStoreRequest) (*pb.DeleteFileStoreResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return nil, err
+	}
+
 	if req.StoreId == "" {
 		return nil, fmt.Errorf("store_id is required")
 	}
 
 	// Extract tenant ID from context
-	tenantID := "default"
+	tenantID := resolveTenantID(ctx, "")
@@ -206,6 +229,10 @@ func (s *FileService) DeleteFileStore(ctx context.Context, req *pb.DeleteFileSto
 // GetFileStore retrieves store information.
 func (s *FileService) GetFileStore(ctx context.Context, req *pb.GetFileStoreRequest) (*pb.GetFileStoreResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return nil, err
+	}
+
 	if req.StoreId == "" {
 		return nil, fmt.Errorf("store_id is required")
 	}
 
 	// Extract tenant ID from context
-	tenantID := "default"
+	tenantID := resolveTenantID(ctx, "")
@@ -242,6 +269,10 @@ func (s *FileService) GetFileStore(ctx context.Context, req *pb.GetFileStoreRequ
 // ListFileStores lists all stores for a client.
 func (s *FileService) ListFileStores(ctx context.Context, req *pb.ListFileStoresRequest) (*pb.ListFileStoresResponse, error) {
+	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
+		return nil, err
+	}
+
 	// For now, return empty list - would need to implement collection listing in Qdrant
 	// This would require storing metadata about stores separately
 	return &pb.ListFileStoresResponse{
 		Stores: []*pb.FileStoreSummary{},
 	}, nil
 }
+
+func resolveTenantID(ctx context.Context, fallback string) string {
+	if tenantID := getTenantID(ctx); tenantID != "" {
+		return tenantID
+	}
+	if client := auth.ClientFromContext(ctx); client != nil && client.ClientID != "" {
+		return client.ClientID
+	}
+	if strings.TrimSpace(fallback) != "" {
+		return fallback
+	}
+	return "default"
+}
```

### H-2 Unbounded UploadFile buffering allows memory exhaustion
- Impact: a client can stream arbitrarily large payloads, consuming memory; gRPC per-message limits do not bound total stream size.
- Fix: enforce a maximum upload size using the metadata size and incremental byte counting (see `files.go` diff above).

## Medium Severity
### M-1 Tenant isolation bypass in FileService
- Impact: file stores are created under `client_id` but read/delete use the hard-coded `default` tenant, risking cross-tenant data leakage and inconsistencies.
- Fix: derive tenant ID from auth/tenant context with a safe fallback (see `files.go` diff above).

### M-2 Qdrant collection name/path injection via tenant/store IDs
- Impact: unvalidated IDs are used in Qdrant URLs, allowing path manipulation or invalid collection names.
- Fix: validate tenant/store IDs before building collection names.

Patch-ready diff:
```diff
diff --git a/internal/rag/service.go b/internal/rag/service.go
index 0f9096b..fd8b3d5 100644
--- a/internal/rag/service.go
+++ b/internal/rag/service.go
@@ -4,8 +4,10 @@ import (
 	"context"
 	"fmt"
 	"io"
+	"regexp"
+	"strings"
 
 	"github.com/cliffpyles/aibox/internal/rag/chunker"
 	"github.com/cliffpyles/aibox/internal/rag/embedder"
@@ -80,6 +82,10 @@ type IngestResult struct {
 // Ingest extracts text from a file, chunks it, embeds the chunks, and stores them.
 func (s *Service) Ingest(ctx context.Context, params IngestParams) (*IngestResult, error) {
+	if err := validateCollectionParts(params.TenantID, params.StoreID); err != nil {
+		return nil, err
+	}
+
 	// Generate collection name
 	collectionName := s.collectionName(params.TenantID, params.StoreID)
@@ -196,6 +202,10 @@ type RetrieveResult struct {
 // Retrieve finds chunks similar to the query text.
 func (s *Service) Retrieve(ctx context.Context, params RetrieveParams) ([]RetrieveResult, error) {
+	if err := validateCollectionParts(params.TenantID, params.StoreID); err != nil {
+		return nil, err
+	}
+
 	collectionName := s.collectionName(params.TenantID, params.StoreID)
@@ -256,6 +266,9 @@ func (s *Service) Retrieve(ctx context.Context, params RetrieveParams) ([]Retrie
 // CreateStore creates a new file store (Qdrant collection).
 func (s *Service) CreateStore(ctx context.Context, tenantID, storeID string) error {
+	if err := validateCollectionParts(tenantID, storeID); err != nil {
+		return err
+	}
 	collectionName := s.collectionName(tenantID, storeID)
 	return s.store.CreateCollection(ctx, collectionName, s.embedder.Dimensions())
 }
@@ -263,6 +276,9 @@ func (s *Service) CreateStore(ctx context.Context, tenantID, storeID string) err
 // DeleteStore removes a file store and all its contents.
 func (s *Service) DeleteStore(ctx context.Context, tenantID, storeID string) error {
+	if err := validateCollectionParts(tenantID, storeID); err != nil {
+		return err
+	}
 	collectionName := s.collectionName(tenantID, storeID)
 	return s.store.DeleteCollection(ctx, collectionName)
 }
@@ -270,6 +286,9 @@ func (s *Service) DeleteStore(ctx context.Context, tenantID, storeID string) err
 // StoreInfo returns information about a file store.
 func (s *Service) StoreInfo(ctx context.Context, tenantID, storeID string) (*vectorstore.CollectionInfo, error) {
+	if err := validateCollectionParts(tenantID, storeID); err != nil {
+		return nil, err
+	}
 	collectionName := s.collectionName(tenantID, storeID)
 	return s.store.CollectionInfo(ctx, collectionName)
 }
@@ -281,6 +300,32 @@ func (s *Service) collectionName(tenantID, storeID string) string {
 	return fmt.Sprintf("%s_%s", tenantID, storeID)
 }
 
+const maxCollectionPartLen = 128
+
+var collectionPartPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)
+
+func validateCollectionParts(tenantID, storeID string) error {
+	tenantID = strings.TrimSpace(tenantID)
+	storeID = strings.TrimSpace(storeID)
+
+	if tenantID == "" {
+		return fmt.Errorf("tenant_id is required")
+	}
+	if storeID == "" {
+		return fmt.Errorf("store_id is required")
+	}
+	if len(tenantID) > maxCollectionPartLen {
+		return fmt.Errorf("tenant_id exceeds %d characters", maxCollectionPartLen)
+	}
+	if len(storeID) > maxCollectionPartLen {
+		return fmt.Errorf("store_id exceeds %d characters", maxCollectionPartLen)
+	}
+	if !collectionPartPattern.MatchString(tenantID) {
+		return fmt.Errorf("tenant_id contains invalid characters")
+	}
+	if !collectionPartPattern.MatchString(storeID) {
+		return fmt.Errorf("store_id contains invalid characters")
+	}
+	return nil
+}
+
 // Helper functions for payload extraction
 func getString(m map[string]any, key string) string {
 	if v, ok := m[key]; ok {
```

### M-3 Token-per-minute defaults are never applied
- Impact: token limits are effectively unlimited unless per-client TPM is set, increasing cost/exposure.
- Fix: apply default TPM in `RecordTokens` when limit is zero.

Patch-ready diff:
```diff
diff --git a/internal/auth/ratelimit.go b/internal/auth/ratelimit.go
index 8c75a25..a65f2a9 100644
--- a/internal/auth/ratelimit.go
+++ b/internal/auth/ratelimit.go
@@ -70,10 +70,17 @@ func (r *RateLimiter) Allow(ctx context.Context, client *ClientKey) error {
 
 // RecordTokens records token usage for TPM limiting
 func (r *RateLimiter) RecordTokens(ctx context.Context, clientID string, tokens int64, limit int) error {
-	if !r.enabled || limit == 0 {
+	if !r.enabled {
+		return nil
+	}
+
+	effectiveLimit := limit
+	if effectiveLimit == 0 {
+		effectiveLimit = r.defaultLimits.TokensPerMinute
+	}
+	if effectiveLimit == 0 {
 		return nil
 	}
@@ -94,7 +101,7 @@ func (r *RateLimiter) RecordTokens(ctx context.Context, clientID string, tokens 
 	}
 
 	// Check if over limit (return error but don't block - already processed)
-	if int(count) > limit {
+	if int(count) > effectiveLimit {
 		return ErrRateLimitExceeded
 	}
```

## Notes / Residual Risk
- `Auth.AdminToken` is configured but never enforced; consider removing it or wiring it into admin auth to avoid a false sense of protection.
