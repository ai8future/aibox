# AIBox Bug and Code Smell Fix Report (2026-01-07-16)

## Fixes
### F-1 Development mode becomes unusable when Redis is down
- Impact: in development mode with Redis unavailable, `auth.RequirePermission` always fails because no client is injected, so ChatService is effectively locked out.
- Fix: inject a development client when `authenticator` is nil and `StartupMode` is not production.

Patch-ready diff:
```diff
diff --git a/internal/server/grpc.go b/internal/server/grpc.go
index 7d9a7f5..a4b0b0a 100644
--- a/internal/server/grpc.go
+++ b/internal/server/grpc.go
@@ -102,6 +102,12 @@ func NewGRPCServer(cfg *config.Config, version VersionInfo) (*grpc.Server, error
 		unaryInterceptors = append(unaryInterceptors, tenantInterceptor.UnaryInterceptor())
 		streamInterceptors = append(streamInterceptors, tenantInterceptor.StreamInterceptor())
 	}
+
+	// Inject a dev client when auth is disabled in development mode.
+	if authenticator == nil && !cfg.StartupMode.IsProduction() {
+		unaryInterceptors = append(unaryInterceptors, developmentAuthInterceptor())
+		streamInterceptors = append(streamInterceptors, developmentAuthStreamInterceptor())
+	}
 
 	// Add auth interceptors if Redis is available
 	if authenticator != nil {
 		unaryInterceptors = append(unaryInterceptors, authenticator.UnaryInterceptor())
@@ -323,6 +329,48 @@ func streamLoggingInterceptor() grpc.StreamServerInterceptor {
 	}
 }
+
+func developmentAuthInterceptor() grpc.UnaryServerInterceptor {
+	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
+		client := &auth.ClientKey{
+			ClientID:   "dev",
+			ClientName: "development",
+			Permissions: []auth.Permission{
+				auth.PermissionAdmin,
+				auth.PermissionChat,
+				auth.PermissionChatStream,
+				auth.PermissionFiles,
+			},
+			RateLimits: auth.RateLimits{},
+		}
+		ctx = context.WithValue(ctx, auth.ClientContextKey, client)
+		return handler(ctx, req)
+	}
+}
+
+func developmentAuthStreamInterceptor() grpc.StreamServerInterceptor {
+	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
+		client := &auth.ClientKey{
+			ClientID:   "dev",
+			ClientName: "development",
+			Permissions: []auth.Permission{
+				auth.PermissionAdmin,
+				auth.PermissionChat,
+				auth.PermissionChatStream,
+				auth.PermissionFiles,
+			},
+			RateLimits: auth.RateLimits{},
+		}
+		ctx := context.WithValue(ss.Context(), auth.ClientContextKey, client)
+		wrapped := &authenticatedStream{ServerStream: ss, ctx: ctx}
+		return handler(srv, wrapped)
+	}
+}
+
+type authenticatedStream struct {
+	grpc.ServerStream
+	ctx context.Context
+}
+
+func (s *authenticatedStream) Context() context.Context {
+	return s.ctx
+}
```

### F-2 Tenant ID normalization mismatch breaks lookups
- Impact: tenant IDs are lowercased in the interceptor but stored as-is at load time, so `Tenant("Acme")` can never be found when the request passes `acme`.
- Fix: normalize tenant IDs when loading configs.

Patch-ready diff:
```diff
diff --git a/internal/tenant/loader.go b/internal/tenant/loader.go
index 7c58c52..8b20d51 100644
--- a/internal/tenant/loader.go
+++ b/internal/tenant/loader.go
@@ -45,6 +45,9 @@ func loadTenants(dir string) (map[string]TenantConfig, error) {
 		case ".yaml", ".yml":
 			if err := yaml.Unmarshal(raw, &cfg); err != nil {
 				return nil, fmt.Errorf("decoding %s: %w", path, err)
 			}
 		}
+
+		cfg.TenantID = strings.ToLower(strings.TrimSpace(cfg.TenantID))
 
 		// Skip files without tenant_id (e.g., shared config files)
 		if cfg.TenantID == "" {
 			continue
 		}
```

### F-3 Config reload ignores the caller-provided configDir override
- Impact: `Load("/custom")` works once, but `Reload()` reverts to the env-configured directory.
- Fix: persist the override into `Env.ConfigsDir` during `Load`.

Patch-ready diff:
```diff
diff --git a/internal/tenant/manager.go b/internal/tenant/manager.go
index 9a6317b..63c1747 100644
--- a/internal/tenant/manager.go
+++ b/internal/tenant/manager.go
@@ -31,8 +31,10 @@ func Load(configDir string) (*Manager, error) {
 	}
 
 	// Use configDir if provided, otherwise use env config
 	if configDir == "" {
 		configDir = envCfg.ConfigsDir
+	} else {
+		envCfg.ConfigsDir = configDir
 	}
 
 	tenantCfgs, err := loadTenants(configDir)
```

### F-4 Streaming replies never record token usage
- Impact: TPM accounting is skipped for streaming responses, so token-based limits are ineffective.
- Fix: record tokens on stream completion.

Patch-ready diff:
```diff
diff --git a/internal/service/chat.go b/internal/service/chat.go
index 706e220..6c5b1d0 100644
--- a/internal/service/chat.go
+++ b/internal/service/chat.go
@@ -338,6 +338,13 @@ func (s *ChatService) GenerateReplyStream(req *pb.GenerateReplyRequest, stream p
 		case provider.ChunkTypeComplete:
+			if s.rateLimiter != nil && chunk.Usage != nil {
+				client := auth.ClientFromContext(ctx)
+				if client != nil {
+					_ = s.rateLimiter.RecordTokens(ctx, client.ClientID, chunk.Usage.TotalTokens, client.RateLimits.TokensPerMinute)
+				}
+			}
 			pbChunk = &pb.GenerateReplyChunk{
 				Chunk: &pb.GenerateReplyChunk_Complete{
 					Complete: &pb.StreamComplete{
 						ResponseId: chunk.ResponseID,
```

### F-5 RAG ingestion can silently overwrite chunk IDs and accepts malformed embeddings
- Impact: repeated uploads of the same filename can overwrite existing vectors; malformed embedder output can panic or corrupt stores.
- Fix: add embedding length validation and generate unique point IDs.

Patch-ready diff:
```diff
diff --git a/internal/rag/service.go b/internal/rag/service.go
index 0f9096b..9a13ab9 100644
--- a/internal/rag/service.go
+++ b/internal/rag/service.go
@@ -4,8 +4,11 @@ import (
 	"context"
+	"crypto/rand"
+	"encoding/hex"
 	"fmt"
 	"io"
+	"time"
 
 	"github.com/cliffpyles/aibox/internal/rag/chunker"
 	"github.com/cliffpyles/aibox/internal/rag/embedder"
@@ -134,6 +137,15 @@ func (s *Service) Ingest(ctx context.Context, params IngestParams) (*IngestResul
 	if err != nil {
 		return nil, fmt.Errorf("generate embeddings: %w", err)
 	}
+	if len(embeddings) != len(chunks) {
+		return nil, fmt.Errorf("embedding count mismatch: got %d want %d", len(embeddings), len(chunks))
+	}
+	expectedDims := s.embedder.Dimensions()
+	for i, embedding := range embeddings {
+		if len(embedding) != expectedDims {
+			return nil, fmt.Errorf("embedding %d dimensions mismatch: got %d want %d", i, len(embedding), expectedDims)
+		}
+	}
@@ -142,8 +154,9 @@ func (s *Service) Ingest(ctx context.Context, params IngestParams) (*IngestResul
 	// Create points for vector store
 	points := make([]vectorstore.Point, len(chunks))
 	for i, chunk := range chunks {
 		points[i] = vectorstore.Point{
-			ID:     fmt.Sprintf("%s_%s_%d", params.Filename, params.StoreID, chunk.Index),
+			ID:     generatePointID(params.Filename, params.StoreID, chunk.Index),
 			Vector: embeddings[i],
 			Payload: map[string]any{
 				"tenant_id":   params.TenantID,
@@ -242,6 +255,20 @@ func (s *Service) collectionName(tenantID, storeID string) string {
 	return fmt.Sprintf("%s_%s", tenantID, storeID)
 }
+
+func generatePointID(filename, storeID string, index int) string {
+	buf := make([]byte, 8)
+	if _, err := rand.Read(buf); err != nil {
+		return fmt.Sprintf("%s_%s_%d_%d", filename, storeID, index, time.Now().UnixNano())
+	}
+	return fmt.Sprintf("%s_%s_%d_%s", filename, storeID, index, hex.EncodeToString(buf))
+}
```
