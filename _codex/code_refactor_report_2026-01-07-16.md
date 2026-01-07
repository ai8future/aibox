# AIBox Code Refactor Review (2026-01-07-16)

## Scope
- Reviewed Go sources under `cmd/` and `internal/` plus supporting config and proto usage.
- Skipped generated code in `gen/` and git metadata.
- No code changes were made.

## High-impact opportunities

### 1) Consolidate request preparation in ChatService
**Why:** `GenerateReply` and `GenerateReplyStream` duplicate validation, request ID handling, provider selection, RAG retrieval, and param construction. The two paths can easily drift (different logs, RAG behavior, provider config overrides).

**Where:** `internal/service/chat.go:46-173` and `internal/service/chat.go:176-331`.

**Suggested refactor:**
- Extract a shared helper (for example `prepareGenerate(ctx, req)`) that returns:
  - selected provider
  - provider config
  - request ID
  - `provider.GenerateParams`
  - any RAG chunks (and formatted instruction override)
- Keep stream and unary paths thin wrappers around this helper.

**Benefits:** less duplication, consistent behavior, and reduced risk when adding new features (new validation, new provider options, or new RAG behavior).

### 2) Unify provider client retry/backoff and default model selection
**Why:** OpenAI, Gemini, and Anthropic clients all implement very similar retry loops, backoff helpers, and request timeout handling. Each client also duplicates default model selection and API key checks.

**Where:**
- OpenAI retry/backoff and defaults: `internal/provider/openai/client.go:21-27`, `internal/provider/openai/client.go:80-201`, `internal/provider/openai/client.go:431-467`.
- Gemini retry/backoff and defaults: `internal/provider/gemini/client.go:17-21`, `internal/provider/gemini/client.go:74-201`, `internal/provider/gemini/client.go:206-231`.
- Anthropic retry/backoff and defaults: `internal/provider/anthropic/client.go:18-23`, `internal/provider/anthropic/client.go:76-195`.

**Suggested refactor:**
- Add a small shared helper in `internal/provider` (or a `provider/retry` package) to run request attempts with backoff.
- Centralize default models in a single location (for example provider package constants or config defaults) and reference that in providers.

**Benefits:** reduces duplicated logic, keeps retry policies aligned, and avoids default model drift across packages.

### 3) Simplify configuration sources and remove unused config fields
**Why:** There are two configuration paths: `internal/config` (file + env) and `internal/tenant` (env + per-tenant YAML/JSON). The `internal/config.Config` includes `Providers` and `Failover` settings that are not referenced anywhere else, while `internal/tenant.EnvConfig` duplicates server/redis/logging settings even though only `ConfigsDir` is used by `tenant.Load`.

**Where:**
- `internal/config/config.go:13-81` and `internal/config/config.go:119-171`.
- `internal/tenant/env.go:9-129`.
- `internal/tenant/manager.go:26-63` (only `ConfigsDir` is used downstream).

**Suggested refactor:**
- Remove unused fields from `internal/config.Config` or wire them into `ChatService` so configuration flows through a single path.
- Narrow `internal/tenant.EnvConfig` to just the settings it actually consumes (likely `ConfigsDir`), or pass in a `config.Config` so tenant loading does not re-parse env variables.

**Benefits:** less drift between config sources, fewer places to change env var handling, and clearer configuration flow.

### 4) Fix tenant ID handling in FileService
**Why:** `CreateFileStore` uses `req.ClientId` as the tenant ID, while `UploadFile`, `DeleteFileStore`, and `GetFileStore` hardcode `tenantID := "default"`. This will create stores under one tenant and then read/write them under another.

**Where:** `internal/service/files.go:30-203`.

**Suggested refactor:**
- Resolve tenant consistently from context (similar to `getTenantID` in chat) or add an explicit tenant ID field to file service requests.
- Centralize tenant ID resolution in a shared helper so all services use the same logic.

**Benefits:** consistent store naming, fewer hidden bugs, and easier multi-tenant support.

### 5) Avoid hardcoded RAG TopK in chat service
**Why:** `retrieveRAGContext` hardcodes `TopK: 5`, which ignores configured defaults (`config.RAG.RetrievalTopK`) and service defaults in `rag.Service`.

**Where:** `internal/service/chat.go:533-553`.

**Suggested refactor:**
- Pass `TopK: 0` to use service defaults, or store the configured value in `ChatService` and use it here.

**Benefits:** single source of truth for retrieval depth and easier tuning.

## Duplication and cleanup opportunities

### A) Remove unused or redundant code paths
- `selectProvider` in `ChatService` appears unused (no references found). Consider removing it or using it inside `selectProviderWithTenant`.
  - `internal/service/chat.go:380-394`.
- Provider client `debug` flags are unused in all three providers. Either wire them to logging or drop them.
  - `internal/provider/openai/client.go:29-41`
  - `internal/provider/gemini/client.go:23-35`
  - `internal/provider/anthropic/client.go:25-37`

### B) Reduce duplication in interceptor skip lists
**Why:** Auth and tenant interceptors carry separate `skipMethods` maps. This can drift (for example, Ready is skipped by tenant but not auth).

**Where:**
- `internal/auth/interceptor.go:22-38`
- `internal/auth/tenant_interceptor.go:19-35`

**Suggested refactor:**
- Consolidate skip lists into a shared constant or config, or add a small helper that merges defaults.

### C) Consolidate env-var expansion utilities
**Why:** `config.expandEnv` and `tenant.loadSecret` both implement env expansion and prefix handling. Having two implementations risks inconsistent behavior.

**Where:**
- `internal/config/config.go:174-205`
- `internal/tenant/secrets.go:22-59`

**Suggested refactor:**
- Move env and secret expansion to a shared utility package and call it from both places.

## Smaller maintainability improvements
- `RateLimiter.GetUsage` uses `fmt.Sscanf` without checking parsing errors. `strconv.ParseInt` would be clearer and easier to audit.
  - `internal/auth/ratelimit.go:126-140`.

## Test and documentation follow-ups
- If tenant handling is aligned for file services, update related tests to include tenant IDs or context-based tenancy.
- If `prepareGenerate` is added, add targeted unit tests for both streaming and unary paths to ensure they remain aligned.

