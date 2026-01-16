# Airborne Refactor Review
Date Created: 2026-01-16 13:20:22 +0100
Date Updated: 2026-01-16 (Claude:Opus 4.5)

## Findings (ordered by severity)

### High/Medium
- ~~High - Tenant `ExtraOptions` maps are assigned directly and then mutated when request overrides are merged, which can leak per-request options across tenants and introduce data races; copy maps before merging and add a guard test.~~ **FIXED** - Deep copy implemented in `internal/service/chat.go:464`
- ~~Medium - Streaming and non-streaming request builders diverge (OpenAI drops service tier/verbosity/prompt cache retention in streaming)~~ **FIXED** - Added missing options to OpenAI streaming path in `internal/provider/openai/client.go:406-429`
- Medium - Gemini streaming drops safety/thinking/structured output compared to non-streaming, causing inconsistent behavior; centralize request building and add parity tests. (`internal/provider/gemini/client.go:162`, `internal/provider/gemini/client.go:403`) **DEFERRED**
- Medium - Compat-based providers expose `WithDebugLogging` and `ClientOption` but the wrapper clients ignore options entirely, so debug logging is effectively unusable; pass options through or remove the no-op API. (`internal/provider/mistral/client.go:22`, `internal/provider/openrouter/client.go:23`, `internal/provider/grok/client.go:22`) **DEFERRED** - Usability issue, not a bug
- Medium - Gemini file upload reads the full content into memory and uses `http.DefaultClient` without timeouts, undermining the temp-file streaming safeguards and risking large memory spikes/hangs; switch to streaming multipart and a configured client. (`internal/provider/gemini/filestore.go:153`, `internal/provider/gemini/filestore.go:217`, `internal/service/files.go:232`) **DEFERRED** - Requires significant refactor for large file support

### Low
- Low - Retry/backoff logic is duplicated and drifting across providers (different error heuristics/strings), making tuning inconsistent; factor into a shared retry helper. (`internal/provider/openai/client.go:721`, `internal/provider/gemini/client.go:859`, `internal/provider/anthropic/client.go:515`, `internal/provider/compat/openai_compat.go:413`) **SKIPPED** - Providers have material differences in error classification; duplication is appropriate polymorphism
- Low - File-store operations repeat similar config validation/logging/response mapping per provider in both the service layer and provider helpers; use an interface or shared helper to reduce copy/paste. (`internal/service/files.go:74`, `internal/service/files.go:108`, `internal/provider/openai/filestore.go:44`, `internal/provider/gemini/filestore.go:45`) **SKIPPED** - Low impact
- Low - Conversation history/message formatting is re-implemented per provider (OpenAI prompt, compat chat messages, Gemini contents, Anthropic messages), which makes behavior diverge over time; consolidate shared policy or enforce consistency with tests. (`internal/provider/openai/client.go:564`, `internal/provider/compat/openai_compat.go:365`, `internal/provider/gemini/client.go:565`, `internal/provider/anthropic/client.go:435`) **SKIPPED** - Providers have different message formats by design
- Low - Provider configuration types are duplicated across runtime config, tenant config, and provider request config, inviting drift and manual mapping errors; consider a shared struct or explicit conversion layer. (`internal/config/config.go:72`, `internal/tenant/config.go:14`, `internal/provider/provider.go:170`) **SKIPPED** - Intentional separation of concerns

## Open Questions / Assumptions
- ~~Are streaming endpoints intentionally unsupported for options like `service_tier`, `verbosity`, `prompt_cache_retention`?~~ **RESOLVED** - No, this was a bug. Options now apply to streaming.
- Safety/thinking/structured output options for Gemini streaming: Still needs investigation.
- Should compat provider wrappers keep `WithDebugLogging` if they do not pass options through, or is debug logging expected to be supported for those providers? **DEFERRED**
- Is it intentional that file-store operations accept per-request API keys while chat requests only use tenant-configured keys? **DEFERRED**
- ~~Should tenant config maps be treated as immutable at runtime (e.g., copy-on-read) to avoid cross-request mutation?~~ **RESOLVED** - Yes. Deep copy now implemented.

## Change Summary
- 2026-01-16: Fixed ExtraOptions map mutation (data race/security)
- 2026-01-16: Fixed OpenAI streaming/non-streaming parity
