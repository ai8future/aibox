# Code Test Report 2026-01-07-16

## Last Updated
- 2026-01-07: Tests added for provider clients (openai, anthropic, gemini) and tenant module

---

## Remaining Untested Areas
- `cmd/aibox` (main entrypoint)
- `internal/redis`
- `internal/rag/testutil`

## Remaining Proposed Unit Test Coverage

### cmd/aibox
- Validate `main` exits non-zero when configuration validation fails (ex: `AIBOX_GRPC_PORT=0`).
- Note: successful startup, gRPC server creation, and Redis-dependent paths are better handled via integration tests with controlled Redis and port binding.

### internal/redis
- Exercise wrapper methods against a local in-memory Redis (`miniredis`):
  - `NewClient` success + failure, `Ping`, `Set/Get/Del/Exists/TTL`, `Incr/IncrBy`, hash ops, `Eval`.
- Validate `IsNil` behavior with `redis.Nil`.
- Adds dev-only dependency on `github.com/alicebob/miniredis/v2`.

### internal/rag/testutil
- `MockEmbedder` default/custom behavior and call tracking.
- `MockStore` create/upsert/search/delete lifecycle + call tracking.
- `MockExtractor` default response, call tracking, supported formats.
- `RandomEmbedding` and `SampleText` helper sanity checks.

---

## Completed Tests (removed from proposals)

The following test areas have been implemented:

### internal/provider/openai (Added v0.5.4)
- `buildUserPrompt`: trims input, formats history with role labeling, includes separators.
- `mapReasoningEffort` and `mapServiceTier`: case-insensitive mappings with defaults.
- `isRetryableError`: OpenAI API status code mapping + string-based fallbacks.
- `waitForCompletion`: error on nil response, short-circuit on completed status or empty ID.
- `extractCitations`: empty response handling.
- Client creation, name, and capabilities.

### internal/provider/anthropic (Added v0.5.4)
- `buildMessages`: conversation history mapping, prepended placeholder when history starts with assistant, input trimming.
- `extractText`: nil handling, empty content.
- `isRetryableError`: string-based retry detection (rate limits, overloads).
- Client creation, name, and capabilities.

### internal/provider/gemini (Added v0.5.4)
- `buildContents`: role mapping (assistant -> model), input trimming, history ordering.
- `extractText`: concatenates candidate content parts; skips nil candidates.
- `extractUsage`: nil-safe conversion from usage metadata.
- `extractCitations`: grounding metadata -> URL citations.
- `buildSafetySettings`: threshold parsing and default behavior.
- `isRetryableError`: string-based retry detection.
- Client creation, name, and capabilities.

### internal/tenant (Added v0.5.4)
- `TenantConfig.GetProvider` and `DefaultProvider` (failover ordering vs fallback).
- `loadSecret` and `resolveSecrets`: ENV/${VAR}/inline handling, path validation.
- `validateTenantConfig`: required fields, bounds checking, failover validation.
- `loadTenants`: JSON/YAML parsing, skipping non-configs, secret resolution, duplicates.
- `Manager` behaviors: sorted tenant codes, default tenant, reload diff results.
