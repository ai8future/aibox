# Audit Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix security, performance, and reliability issues identified in the Gemini CLI audit reports.

**Architecture:** These are targeted fixes to existing code - no new features or architectural changes. Each fix is isolated and can be committed independently.

**Tech Stack:** Go 1.25, gRPC, Redis

---

## Task 1: Fix DoS via Memory Exhaustion in File Uploads (CRITICAL)

**Files:**
- Modify: `internal/service/files.go:220-261`

**Problem:** File uploads buffer entire content (up to 100MB) in memory via `bytes.Buffer`. 50 concurrent uploads = 5GB RAM, causing OOM.

**Step 1: Update imports**

Add `"os"` to the imports if not already present.

**Step 2: Replace bytes.Buffer with temp file**

Replace lines 220-261 in `UploadFile` method:

```go
	// Collect file chunks with size limit enforcement
	// SECURITY: Use a temporary file instead of bytes.Buffer to prevent memory exhaustion (DoS)
	tmpFile, err := os.CreateTemp("", "airborne-upload-*.tmp")
	if err != nil {
		return status.Error(codes.Internal, "failed to create temporary file for upload")
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	var totalBytes int64
	for {
		// Check for context cancellation (timeout)
		select {
		case <-ctx.Done():
			return status.Error(codes.DeadlineExceeded, "upload timeout exceeded")
		default:
		}

		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("receive chunk: %w", err)
		}

		chunk := msg.GetChunk()
		if chunk == nil {
			continue
		}

		// Enforce size limit
		totalBytes += int64(len(chunk))
		if totalBytes > maxUploadBytes {
			return fmt.Errorf("file exceeds maximum allowed size %d bytes", maxUploadBytes)
		}

		if _, err := tmpFile.Write(chunk); err != nil {
			return fmt.Errorf("write to temp file: %w", err)
		}
	}

	// Reset file pointer to beginning for reading
	if _, err := tmpFile.Seek(0, 0); err != nil {
		return fmt.Errorf("seek temp file: %w", err)
	}

	// Route by provider
	switch metadata.Provider {
	case pb.Provider_PROVIDER_OPENAI:
		return s.uploadToOpenAI(ctx, stream, metadata, tmpFile)
	case pb.Provider_PROVIDER_GEMINI:
		return s.uploadToGemini(ctx, stream, metadata, tmpFile)
	default:
		return s.uploadToInternal(ctx, stream, metadata, tmpFile)
	}
```

**Step 3: Update function signatures**

Change all three upload helper functions to accept `io.Reader` instead of `*bytes.Buffer`:

```go
func (s *FileService) uploadToOpenAI(ctx context.Context, stream pb.FileService_UploadFileServer, metadata *pb.UploadFileMetadata, content io.Reader) error {
```

```go
func (s *FileService) uploadToGemini(ctx context.Context, stream pb.FileService_UploadFileServer, metadata *pb.UploadFileMetadata, content io.Reader) error {
```

```go
func (s *FileService) uploadToInternal(ctx context.Context, stream pb.FileService_UploadFileServer, metadata *pb.UploadFileMetadata, content io.Reader) error {
```

**Step 4: Run tests**

```bash
go test ./internal/service/... -v -race
```

**Step 5: Commit**

```bash
git add internal/service/files.go
git commit -m "fix(security): prevent DoS via memory exhaustion in file uploads

Use temporary file instead of bytes.Buffer to prevent memory exhaustion
when handling large file uploads. 50 concurrent 100MB uploads would
previously consume 5GB RAM, potentially causing OOM.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 2: Fix Unconditional HTTP Capture Performance Issue (HIGH)

**Files:**
- Modify: `internal/provider/openai/client.go:110-117`
- Modify: `internal/provider/anthropic/client.go:118-125, 322-329`
- Modify: `internal/provider/gemini/client.go:103-110`
- Modify: `internal/provider/compat/openai_compat.go:135-142`

**Problem:** `httpcapture.New()` is called for every request regardless of debug mode, causing GC pressure and latency overhead.

**Step 1: Fix openai/client.go**

Replace lines ~110-117:

```go
	// Create capturing transport for debug JSON (only when debug enabled)
	var capture *httpcapture.Transport
	clientOpts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if c.debug {
		capture = httpcapture.New()
		clientOpts = append(clientOpts, option.WithHTTPClient(capture.Client()))
	}
```

Then update the result building (around line 320) to handle nil capture:

```go
	var reqJSON, respJSON []byte
	if capture != nil {
		reqJSON = capture.RequestBody
		respJSON = capture.ResponseBody
	}
	// Use reqJSON and respJSON in the result
```

**Step 2: Fix anthropic/client.go**

Apply the same pattern to both `GenerateReply` (~line 118) and `GenerateReplyStream` (~line 322).

**Step 3: Fix gemini/client.go**

Apply the same pattern around line 103.

**Step 4: Fix compat/openai_compat.go**

Apply the same pattern around line 135.

**Step 5: Run tests**

```bash
go test ./internal/provider/... -v -race
```

**Step 6: Commit**

```bash
git add internal/provider/
git commit -m "perf: only create HTTP capture transport when debug enabled

Previously httpcapture.New() was called for every request, reading
entire request/response bodies into memory even when debug logging
was disabled. This caused unnecessary GC pressure and latency.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 3: Fix Silent Configuration Failures (HIGH)

**Files:**
- Modify: `internal/config/config.go:186-275`

**Problem:** Invalid environment variable values are silently ignored, causing services to start with unexpected defaults.

**Step 1: Add warning logs for failed parses**

For each `strconv.Atoi` or `strconv.ParseBool` call in `applyEnvOverrides`, add an else clause:

```go
	if port := os.Getenv("AIRBORNE_GRPC_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			c.Server.GRPCPort = p
		} else {
			slog.Warn("invalid AIRBORNE_GRPC_PORT, using default", "value", port, "error", err)
		}
	}
```

Apply this pattern to all numeric/boolean env var parses:
- `AIRBORNE_GRPC_PORT`
- `AIRBORNE_TLS_ENABLED`
- `REDIS_DB`
- `RAG_ENABLED`
- `RAG_CHUNK_SIZE`
- `RAG_CHUNK_OVERLAP`
- `RAG_RETRIEVAL_TOP_K`

**Step 2: Add import for slog if needed**

Ensure `"log/slog"` is in the imports.

**Step 3: Run tests**

```bash
go test ./internal/config/... -v -race
```

**Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "fix: log warnings for invalid environment variable values

Previously invalid env vars like AIRBORNE_GRPC_PORT=\"invalid\" were
silently ignored, causing services to start with defaults. Operators
now see warnings to diagnose configuration issues.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 4: Fix Rate Limit Error Suppression (MEDIUM)

**Files:**
- Modify: `internal/service/chat.go:230, 344`

**Problem:** Errors from `RecordTokens` are explicitly ignored with `_ =`, losing visibility when Redis issues occur.

**Step 1: Fix GenerateReply (line 230)**

Replace:
```go
_ = s.rateLimiter.RecordTokens(ctx, client.ClientID, result.Usage.TotalTokens, client.RateLimits.TokensPerMinute)
```

With:
```go
if err := s.rateLimiter.RecordTokens(ctx, client.ClientID, result.Usage.TotalTokens, client.RateLimits.TokensPerMinute); err != nil {
	slog.Warn("failed to record token usage for rate limiting", "client_id", client.ClientID, "error", err)
}
```

**Step 2: Fix GenerateReplyStream (line 344)**

Apply the same pattern:
```go
if err := s.rateLimiter.RecordTokens(ctx, client.ClientID, chunk.Usage.TotalTokens, client.RateLimits.TokensPerMinute); err != nil {
	slog.Warn("failed to record stream token usage for rate limiting", "client_id", client.ClientID, "error", err)
}
```

**Step 3: Run tests**

```bash
go test ./internal/service/... -v -race
```

**Step 4: Commit**

```bash
git add internal/service/chat.go
git commit -m "fix: log rate limit recording errors instead of suppressing

Errors from RecordTokens were silently ignored, making Redis issues
invisible. Now logs warnings so operators can detect when rate limiting
data is not being recorded.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 5: Improve JSON Error Context (LOW)

**Files:**
- Modify: `internal/auth/keys.go:243`

**Problem:** JSON unmarshal errors don't include the key ID, making debugging harder.

**Step 1: Update error message**

Replace line 243:
```go
return nil, fmt.Errorf("failed to unmarshal key: %w", err)
```

With:
```go
return nil, fmt.Errorf("data corruption in key store for %q: %w", keyID, err)
```

**Step 2: Run tests**

```bash
go test ./internal/auth/... -v -race
```

**Step 3: Commit**

```bash
git add internal/auth/keys.go
git commit -m "fix: include key ID in JSON unmarshal error messages

Helps distinguish between transient Redis failures and permanent
data corruption issues.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 6: Add Warning Comment to Dead Code (LOW)

**Files:**
- Modify: `internal/server/grpc.go:325-326`

**Problem:** Development auth interceptors exist but aren't wired up - risk of accidental production use.

**Step 1: Add prominent warning comment**

Update the comment before `developmentAuthInterceptor`:

```go
// developmentAuthInterceptor injects a dev client in non-production mode when Redis is unavailable.
//
// WARNING: This function bypasses authentication entirely. It is intended ONLY for
// local development and testing. NEVER wire this into NewGRPCServer for production builds.
// If you need to use this, ensure it's behind a build tag or explicit development mode check.
func developmentAuthInterceptor() grpc.UnaryServerInterceptor {
```

Apply similar comment to `developmentAuthStreamInterceptor`.

**Step 2: Commit**

```bash
git add internal/server/grpc.go
git commit -m "docs: add prominent warning to development auth interceptors

Makes it clear these functions bypass authentication and should never
be wired into production code.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Final Steps

**Run full test suite:**
```bash
go test ./... -v -race
```

**Run build:**
```bash
go build ./cmd/airborne
```

**Update VERSION and CHANGELOG.md** per project conventions.
