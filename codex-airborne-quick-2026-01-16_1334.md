Date Created: 2026-01-16 13:34:05 +0100

## AUDIT
- SSRF guard only checks literal host/IP and can be bypassed by DNS rebinding; resolve hostnames and block private/metadata IPs in `internal/validation/url.go`.
```diff
diff --git a/internal/validation/url.go b/internal/validation/url.go
index 7b08e2d..5fa3c9a 100644
--- a/internal/validation/url.go
+++ b/internal/validation/url.go
@@ -97,6 +97,7 @@ func ValidateProviderURL(rawURL string) error {
 	if ip := net.ParseIP(hostname); ip != nil {
 		// Allow localhost IPs for http://
 		if isLocalhost {
 			return nil
 		}
@@ -106,8 +107,26 @@ func ValidateProviderURL(rawURL string) error {
 		if isPrivateIP(ip) {
 			return fmt.Errorf("%w: %s is in a private IP range", ErrPrivateIP, hostname)
 		}
+
+		// Block metadata IPs explicitly
+		if isMetadataEndpoint(ip.String()) {
+			return fmt.Errorf("%w: %s is blocked", ErrMetadataEndpoint, hostname)
+		}
 
 		return nil
 	}
+
+	// Resolve DNS to prevent hostnames pointing at private networks (DNS rebinding).
+	ips, err := net.LookupIP(hostname)
+	if err != nil {
+		return fmt.Errorf("%w: failed to resolve hostname %q", ErrInvalidURL, hostname)
+	}
+	for _, ip := range ips {
+		if isPrivateIP(ip) {
+			return fmt.Errorf("%w: %s resolves to a private IP", ErrPrivateIP, hostname)
+		}
+		if isMetadataEndpoint(ip.String()) {
+			return fmt.Errorf("%w: %s resolves to metadata IP %s", ErrMetadataEndpoint, hostname, ip.String())
+		}
+	}
 
 	return nil
 }
```
- `ListKeys` claims to return keys "without secrets" but includes the `SecretHash`; redact before returning in `internal/auth/keys.go`.
```diff
diff --git a/internal/auth/keys.go b/internal/auth/keys.go
index 0a6db3d..e7f11de 100644
--- a/internal/auth/keys.go
+++ b/internal/auth/keys.go
@@ -177,6 +177,7 @@ func (s *KeyStore) ListKeys(ctx context.Context) ([]*ClientKey, error) {
 		if err != nil {
 			// Skip keys that can't be loaded (may have expired)
 			continue
 		}
+		key.SecretHash = ""
 		keys = append(keys, key)
 	}
 
 	return keys, nil
 }
```
- Gemini FileSearch uses `http.DefaultClient` without timeouts; add a client with timeouts to prevent hung requests in `internal/provider/gemini/filestore.go`.
```diff
diff --git a/internal/provider/gemini/filestore.go b/internal/provider/gemini/filestore.go
index e55a3a1..08acb0c 100644
--- a/internal/provider/gemini/filestore.go
+++ b/internal/provider/gemini/filestore.go
@@ -19,9 +19,12 @@ import (
 const (
 	fileSearchBaseURL         = "https://generativelanguage.googleapis.com/v1beta"
 	fileSearchPollingInterval = 2 * time.Second
 	fileSearchPollingTimeout  = 5 * time.Minute
+	fileSearchHTTPTimeout     = 60 * time.Second
 )
+
+var fileSearchHTTPClient = &http.Client{Timeout: fileSearchHTTPTimeout}
@@ -112,7 +115,7 @@ func CreateFileSearchStore(ctx context.Context, cfg FileStoreConfig, name string)
 	req.Header.Set("Content-Type", "application/json")
 
-	resp, err := http.DefaultClient.Do(req)
+	resp, err := fileSearchHTTPClient.Do(req)
 	if err != nil {
 		return nil, fmt.Errorf("execute request: %w", err)
 	}
@@ -214,7 +217,7 @@ func UploadFileToFileSearchStore(ctx context.Context, cfg FileStoreConfig, storeI
 	}
 	req.Header.Set("X-Goog-Upload-Protocol", "raw")
 
-	resp, err := http.DefaultClient.Do(req)
+	resp, err := fileSearchHTTPClient.Do(req)
 	if err != nil {
 		return nil, fmt.Errorf("execute upload request: %w", err)
 	}
@@ -229,7 +232,7 @@ func UploadFileToFileSearchStore(ctx context.Context, cfg FileStoreConfig, storeI
 		}
 		req2.Header.Set("Content-Type", "application/json")
 
-		resp2, err := http.DefaultClient.Do(req2)
+		resp2, err := fileSearchHTTPClient.Do(req2)
 		if err != nil {
 			return nil, fmt.Errorf("execute metadata request: %w", err)
 		}
@@ -308,7 +311,7 @@ func waitForOperation(ctx context.Context, cfg FileStoreConfig, operationName str
 		case <-ticker.C:
 			req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, url, nil)
 			if err != nil {
 				return "unknown", fmt.Errorf("create request: %w", err)
 			}
 
-			resp, err := http.DefaultClient.Do(req)
+			resp, err := fileSearchHTTPClient.Do(req)
 			if err != nil {
 				return "unknown", fmt.Errorf("execute request: %w", err)
 			}
@@ -357,7 +360,7 @@ func DeleteFileSearchStore(ctx context.Context, cfg FileStoreConfig, storeID stri
 	if err != nil {
 		return fmt.Errorf("create request: %w", err)
 	}
 
-	resp, err := http.DefaultClient.Do(req)
+	resp, err := fileSearchHTTPClient.Do(req)
 	if err != nil {
 		return fmt.Errorf("execute request: %w", err)
 	}
@@ -394,7 +397,7 @@ func GetFileSearchStore(ctx context.Context, cfg FileStoreConfig, storeID string)
 	if err != nil {
 		return nil, fmt.Errorf("create request: %w", err)
 	}
 
-	resp, err := http.DefaultClient.Do(req)
+	resp, err := fileSearchHTTPClient.Do(req)
 	if err != nil {
 		return nil, fmt.Errorf("execute request: %w", err)
 	}
@@ -452,7 +455,7 @@ func ListFileSearchStores(ctx context.Context, cfg FileStoreConfig, limit int) ([
 	if err != nil {
 		return nil, fmt.Errorf("create request: %w", err)
 	}
 
-	resp, err := http.DefaultClient.Do(req)
+	resp, err := fileSearchHTTPClient.Do(req)
 	if err != nil {
 		return nil, fmt.Errorf("execute request: %w", err)
 	}
```

## TESTS
- Add coverage for HTTP capture transport behavior in `internal/httpcapture/transport.go`.
```diff
diff --git a/internal/httpcapture/transport_test.go b/internal/httpcapture/transport_test.go
new file mode 100644
index 0000000..e934a2c
--- /dev/null
+++ b/internal/httpcapture/transport_test.go
@@ -0,0 +1,68 @@
+package httpcapture
+
+import (
+	"bytes"
+	"io"
+	"net/http"
+	"net/http/httptest"
+	"testing"
+)
+
+func TestTransport_CapturesAndPreservesBodies(t *testing.T) {
+	var received []byte
+	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		body, err := io.ReadAll(r.Body)
+		if err != nil {
+			t.Fatalf("read request body: %v", err)
+		}
+		received = body
+		w.WriteHeader(http.StatusOK)
+		_, _ = w.Write([]byte("response body"))
+	}))
+	defer server.Close()
+
+	transport := New()
+	client := transport.Client()
+
+	reqBody := []byte("request body")
+	req, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(reqBody))
+	if err != nil {
+		t.Fatalf("new request: %v", err)
+	}
+
+	resp, err := client.Do(req)
+	if err != nil {
+		t.Fatalf("do request: %v", err)
+	}
+	respBody, err := io.ReadAll(resp.Body)
+	resp.Body.Close()
+	if err != nil {
+		t.Fatalf("read response body: %v", err)
+	}
+
+	if got := string(received); got != string(reqBody) {
+		t.Fatalf("server received %q, want %q", got, reqBody)
+	}
+	if got := string(transport.RequestBody); got != string(reqBody) {
+		t.Fatalf("captured request %q, want %q", got, reqBody)
+	}
+	if got := string(respBody); got != "response body" {
+		t.Fatalf("client response %q, want %q", got, "response body")
+	}
+	if got := string(transport.ResponseBody); got != "response body" {
+		t.Fatalf("captured response %q, want %q", got, "response body")
+	}
+}
```
- Add a regression test ensuring tenant-disabled fallback providers are skipped in `internal/service/chat_test.go`.
```diff
diff --git a/internal/service/chat_test.go b/internal/service/chat_test.go
index 5c2f128..2b1d82e 100644
--- a/internal/service/chat_test.go
+++ b/internal/service/chat_test.go
@@ -3,6 +3,7 @@ package service
 import (
 	"context"
+	"errors"
 	"strings"
 	"testing"
@@ -1140,6 +1141,28 @@ func TestGetFallbackProvider_DefaultFallbackFromAnthropic(t *testing.T) {
 	if fallback.Name() != "openai" {
 		t.Errorf("expected openai as default fallback from anthropic, got %s", fallback.Name())
 	}
 }
+
+func TestGenerateReply_FallbackSkippedWhenProviderDisabled(t *testing.T) {
+	primary := newMockProvider("openai")
+	primary.generateErr = errors.New("primary failed")
+	fallback := newMockProvider("gemini")
+	svc := createChatServiceWithMocks(primary, fallback, newMockProvider("anthropic"), nil)
+
+	tenantCfg := createTestTenantConfig("openai")
+	ctx := ctxWithChatPermissionAndTenant("test-client", tenantCfg)
+
+	req := &pb.GenerateReplyRequest{
+		UserInput:        "hello",
+		EnableFailover:   true,
+		FallbackProvider: pb.Provider_PROVIDER_GEMINI,
+	}
+
+	_, err := svc.GenerateReply(ctx, req)
+	if err == nil {
+		t.Fatal("expected error when primary fails and fallback is disabled")
+	}
+	if len(fallback.generateCalls) != 0 {
+		t.Fatalf("expected fallback not called, got %d calls", len(fallback.generateCalls))
+	}
+}
```

## FIXES
- Skip failover when the tenant does not enable the fallback provider; prevents futile retries and inconsistent behavior in `internal/service/chat.go`.
```diff
diff --git a/internal/service/chat.go b/internal/service/chat.go
index 3cdf2d7..2e4f8c9 100644
--- a/internal/service/chat.go
+++ b/internal/service/chat.go
@@ -201,8 +201,20 @@ func (s *ChatService) GenerateReply(ctx context.Context, req *pb.GenerateReplyReq
 	if err != nil {
 		// Try failover if enabled
 		if req.EnableFailover {
 			fallbackProvider := s.getFallbackProvider(prepared.provider.Name(), req.FallbackProvider)
-			if fallbackProvider != nil {
+			if fallbackProvider != nil {
+				if tenantCfg := auth.TenantFromContext(ctx); tenantCfg != nil {
+					if _, ok := tenantCfg.GetProvider(fallbackProvider.Name()); !ok {
+						slog.Warn("fallback provider not enabled for tenant, skipping",
+							"fallback", fallbackProvider.Name(),
+							"tenant_id", tenantCfg.TenantID,
+						)
+						fallbackProvider = nil
+					}
+				}
+			}
+			if fallbackProvider != nil {
 				slog.Warn("primary provider failed, trying fallback",
 					"primary", prepared.provider.Name(),
 					"fallback", fallbackProvider.Name(),
 					"error", err,
 				)
```
- Return proper gRPC InvalidArgument errors for bad upload metadata in `internal/service/files.go`.
```diff
diff --git a/internal/service/files.go b/internal/service/files.go
index 2a6a0a4..7b6fda2 100644
--- a/internal/service/files.go
+++ b/internal/service/files.go
@@ -150,14 +150,14 @@ func (s *FileService) UploadFile(stream pb.FileService_UploadFileServer) error {
 	// First message should be metadata
 	firstMsg, err := stream.Recv()
 	if err != nil {
-		return fmt.Errorf("receive metadata: %w", err)
+		return status.Errorf(codes.InvalidArgument, "receive metadata: %v", err)
 	}
 
 	metadata := firstMsg.GetMetadata()
 	if metadata == nil {
-		return fmt.Errorf("first message must contain metadata")
+		return status.Error(codes.InvalidArgument, "first message must contain metadata")
 	}
 
 	if metadata.StoreId == "" {
-		return fmt.Errorf("store_id is required")
+		return status.Error(codes.InvalidArgument, "store_id is required")
 	}
 	if metadata.Filename == "" {
-		return fmt.Errorf("filename is required")
+		return status.Error(codes.InvalidArgument, "filename is required")
 	}
 
 	// Validate declared size if provided
 	if metadata.Size > 0 && metadata.Size > maxUploadBytes {
-		return fmt.Errorf("file size %d exceeds maximum allowed size %d bytes", metadata.Size, maxUploadBytes)
+		return status.Errorf(codes.InvalidArgument, "file size %d exceeds maximum allowed size %d bytes", metadata.Size, maxUploadBytes)
 	}
```

## REFACTOR
- Extract provider retry/backoff logic into a shared helper to reduce duplication across `internal/provider/openai`, `internal/provider/anthropic`, and `internal/provider/compat`.
- Add an injectable HTTP client interface for provider filestore operations to make behavior testable without touching package globals.
- Split `ChatService.prepareRequest` into validation, provider selection, and RAG augmentation helpers to simplify unit testing and error handling.
