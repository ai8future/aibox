# Audit Fixes Round 2 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix the 3 medium-priority security/reliability issues identified in the second audit round.

**Architecture:** These are targeted fixes to existing code - atomic Lua script for rate limiting, rate limit check for file uploads, and generic error message for tenant lookup. Each fix is isolated and can be committed independently.

**Tech Stack:** Go 1.25, Redis Lua scripting, gRPC

---

## Task 1: Fix Race Condition in Token Rate Limiter

**Files:**
- Modify: `internal/auth/ratelimit.go:79-118`
- Test: `internal/auth/ratelimit_test.go`

**Problem:** The `RecordTokens` function has a race condition where `Expire` is only called when `count == tokens`. If two requests increment simultaneously, neither may see this condition, leaving the key without an expiry.

**Step 1: Add Lua script for atomic token recording**

Add this script constant after `rateLimitScript` (around line 30):

```go
// tokenRecordScript is a Lua script for atomically recording tokens with TTL
// It increments by the token count and ensures TTL is set
const tokenRecordScript = `
local key = KEYS[1]
local tokens = tonumber(ARGV[1])
local window = tonumber(ARGV[2])

local current = redis.call('INCRBY', key, tokens)
local ttl = redis.call('TTL', key)
if ttl == -1 then
    redis.call('EXPIRE', key, window)
end

return current
`
```

**Step 2: Update RecordTokens to use the Lua script**

Replace the `RecordTokens` function (lines 79-118):

```go
// RecordTokens records token usage for TPM limiting
func (r *RateLimiter) RecordTokens(ctx context.Context, clientID string, tokens int64, limit int) error {
	if !r.enabled {
		return nil
	}

	if tokens <= 0 {
		return nil // Ignore non-positive token counts
	}

	// Apply default TPM limit if client-specific limit is 0
	if limit == 0 {
		limit = r.defaultLimits.TokensPerMinute
	}

	// Only skip if both client limit and default are 0 (unlimited)
	if limit == 0 {
		return nil
	}

	key := fmt.Sprintf("%s%s:tpm", rateLimitPrefix, clientID)

	// Use Lua script for atomic increment + TTL setting
	result, err := r.redis.Eval(ctx, tokenRecordScript, []string{key}, tokens, 60)
	if err != nil {
		return fmt.Errorf("failed to record tokens: %w", err)
	}

	// Parse result (same handling as checkLimit)
	var count int64
	switch v := result.(type) {
	case int64:
		count = v
	case int:
		count = int64(v)
	case float64:
		count = int64(v)
	default:
		return fmt.Errorf("unexpected result type %T from token record script", result)
	}

	// Check if over limit (return error but don't block - already processed)
	if int(count) > limit {
		return ErrRateLimitExceeded
	}

	return nil
}
```

**Step 3: Run tests**

```bash
go test ./internal/auth/... -v -race -run TestRateLimiter
```

**Step 4: Commit**

```bash
git add internal/auth/ratelimit.go
git commit -m "fix: use atomic Lua script for token rate limiting

Fixes race condition where concurrent requests could both miss the
condition to set TTL (count == tokens), leaving keys without expiry.
Now uses TTL check in Lua script to ensure expiry is always set.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 2: Add Rate Limiting for File Upload Operations

**Files:**
- Modify: `internal/service/files.go:37-49, 175-220`
- Test: `internal/service/files_test.go`

**Problem:** File uploads lack rate limiting, allowing resource exhaustion through unlimited uploads.

**Step 1: Add RateLimiter field to FileService**

Update the FileService struct (around line 37):

```go
// FileService implements the FileService gRPC service for RAG file management.
type FileService struct {
	pb.UnimplementedFileServiceServer

	ragService  *rag.Service
	rateLimiter *auth.RateLimiter
}

// NewFileService creates a new file service.
func NewFileService(ragService *rag.Service, rateLimiter *auth.RateLimiter) *FileService {
	return &FileService{
		ragService:  ragService,
		rateLimiter: rateLimiter,
	}
}
```

**Step 2: Add rate limit check in UploadFile**

Add rate limit check at the beginning of `UploadFile` (around line 180), after the permission check:

```go
// UploadFile handles streaming file uploads.
func (s *FileService) UploadFile(stream pb.FileService_UploadFileServer) error {
	ctx := stream.Context()

	// Check permission
	if err := auth.RequirePermission(ctx, auth.PermissionFiles); err != nil {
		return err
	}

	// Check rate limit for file uploads
	if s.rateLimiter != nil {
		client := auth.ClientFromContext(ctx)
		if client != nil {
			if err := s.rateLimiter.Allow(ctx, client); err != nil {
				return status.Error(codes.ResourceExhausted, "file upload rate limit exceeded")
			}
		}
	}

	// Set upload timeout
	// ... rest of function unchanged
```

**Step 3: Update NewFileService call in grpc.go**

Find where `NewFileService` is called in `internal/server/grpc.go` and add the rateLimiter parameter:

```go
fileService := service.NewFileService(ragService, rateLimiter)
```

**Step 4: Run tests**

```bash
go test ./internal/service/... -v -race
go test ./internal/server/... -v -race
```

**Step 5: Commit**

```bash
git add internal/service/files.go internal/server/grpc.go
git commit -m "feat: add rate limiting for file upload operations

Prevents resource exhaustion through unlimited file uploads by
checking rate limits before processing upload streams.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 3: Fix Tenant ID Disclosure in Error Messages

**Files:**
- Modify: `internal/auth/tenant_interceptor.go:105`

**Problem:** Error message echoes back the tenant ID, enabling enumeration attacks.

**Step 1: Update error message**

Replace line 105:

```go
// Before:
return nil, status.Errorf(codes.NotFound, "tenant %q not found", tenantID)

// After:
return nil, status.Error(codes.NotFound, "tenant not found")
```

**Step 2: Run tests**

```bash
go test ./internal/auth/... -v -race
```

**Step 3: Commit**

```bash
git add internal/auth/tenant_interceptor.go
git commit -m "fix(security): remove tenant ID from error message

Prevents tenant enumeration attacks by returning generic error
message without echoing back the requested tenant ID.

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
