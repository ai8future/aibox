# AIBox Code Audit Report
Date Created: 2026-01-07 23:21:06 +0100
Date Updated: 2026-01-08

## Scope
- Reviewed Go services in `cmd`, `internal`, API protos, configs, and Docker assets.
- Excluded `_studies`/`_proposals` per agent rules.
- Tests not executed for this audit.

## Findings (ordered by severity)
### High
1) Tenant boundary not enforced for API keys; tenant discovery before auth enables cross-tenant access.
- Evidence: `internal/auth/keys.go:36`, `internal/auth/tenant_interceptor.go:83`, `internal/server/grpc.go:91`.
- Impact: any valid API key can access any tenant by setting `tenant_id`; unauthenticated callers can probe tenant IDs.
- Recommendation: bind keys to tenant IDs and enforce match after authentication; move auth interceptors before tenant resolution; allow tenant ID via metadata for non-tenant-aware RPCs.

2) Default deployment variables and configuration are miswired; provider settings in `configs/aibox.yaml` are unused.
- Evidence: `configs/aibox.yaml:18`, `configs/email4ai.json:1`, `docker-compose.yml:13`, `internal/service/chat.go:454`.
- Impact: docker-compose sets `OPENAI_API_KEY`/`GEMINI_API_KEY`/`ANTHROPIC_API_KEY`, but tenant config expects `EMAIL4AI_*` vars; if tenant config fails to load the server falls back to legacy mode with no provider API keys and all requests fail.
- Recommendation: align env var names, or fail fast in production when tenant configs cannot load; if legacy mode is intended, wire `config.Providers` into provider defaults.

### Medium
3) Failover is not applied consistently; streaming fallback and tenant failover order are ignored.
- Evidence: `internal/service/chat.go:149`, `internal/service/chat.go:281`, `internal/service/chat.go:428`, `internal/tenant/config.go:25`.
- Impact: failover settings are only partially honored (unary only, hard-coded order), causing avoidable outages for streaming clients.
- Recommendation: use tenant failover order when present and attempt fallback on stream initialization failures.

### Low/Medium
4) RAG context is appended after size validation.
- Evidence: `internal/service/chat.go:71`, `internal/service/chat.go:105`, `internal/service/chat.go:244`.
- Impact: with larger RAG settings, the effective prompt can exceed configured size limits; provider requests then fail or inflate costs.
- Recommendation: revalidate after RAG context injection.

5) FileService provider fields are ignored and `ListFileStores` is unimplemented.
- Evidence: `internal/service/files.go:33`, `api/proto/aibox/v1/files.proto:23`.
- Impact: client SDKs expect provider-backed file stores that are not supported in this implementation.
- Recommendation: reject unsupported provider settings up front, or update the proto/docs to describe self-hosted Qdrant-only behavior.

## Fixed in v0.5.7-0.5.13
- Config file read errors silently ignored - FIXED (v0.5.7): Now fails on read errors, allows missing files
- Tenant ID case normalization inconsistent - FIXED (v0.5.12): Normalized to lowercase on load
- Logging config ignored - FIXED (v0.5.8): Now applies cfg.Logging level/format
- FileService blocked by tenant interceptor - FIXED (v0.5.10): Added to skipMethods
- Go toolchain version 1.25 - NOT A BUG: Go 1.25 is valid (report was incorrect)

## Remaining Patch Recommendations

### Patch 1: Enforce tenant scoping (for remaining high-severity issues)
Consider binding API keys to tenant IDs and enforcing match after authentication.

### Patch 2: Align docker-compose env vars with tenant config
Add EMAIL4AI_* env mappings to docker-compose.yml or update tenant config to read standard OPENAI_API_KEY etc.

## Suggested follow-ups
- Add a documented key-management flow so `TenantID` is set for keys (e.g., admin RPC or provisioning script).
- Decide whether admin keys should be tenant-scoped or global, then enforce consistently.
- Either implement provider-backed file stores or update `api/proto/aibox/v1/files.proto` and docs to define Qdrant-only semantics.

## Suggested verification
1) `go test ./...`
2) `go test -race ./...`
3) `docker build .`
4) `docker-compose up` and verify `/aibox.v1.AdminService/Ready` and a sample `GenerateReply` for the configured tenant.
