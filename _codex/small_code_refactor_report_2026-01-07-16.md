# AIBox Refactor Opportunities (2026-01-07-16)

## Opportunities
- Consolidate retry/backoff logic in `internal/provider/{openai,gemini,anthropic}` into a shared helper to reduce duplication and keep behavior consistent.
- Extract shared GenerateReply/GenerateReplyStream setup (validation, provider selection, config merge, RAG retrieval) into a common helper to reduce divergence.
- Introduce small interfaces for Redis and vector store clients to improve unit testability without external dependencies.
- Centralize tenant ID and store ID resolution (used in ChatService and FileService) into a single utility to avoid drift.
- Normalize and validate IDs in one place (tenant ID, store ID, client ID), and reuse across config loading and request validation.
- Replace ad hoc logging fields with a structured request context (request_id, tenant_id, client_id) passed through services.
- Add a small config validation layer (limits, required fields, range checks) that runs after Load to keep invariants explicit.
