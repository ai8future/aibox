# Codebase Refactoring Report: Airborne Project
**Date Created:** 2026-01-16_1430
**Date Updated:** 2026-01-16 (Claude:Opus 4.5)

## Executive Summary

The `airborne` codebase exhibits a solid architectural foundation with a clear separation of concerns between services, providers, and API definitions. However, as the number of AI providers has grown (OpenAI, Anthropic, Gemini), a significant amount of code duplication has accumulated in the `internal/provider` package. Additionally, the `ChatService` has become tightly coupled to specific provider implementations, making it difficult to extend without modifying core service logic.

This report outlines key areas for refactoring to improve maintainability, reduce code duplication, and adhere to the Open/Closed Principle.

## Status Update (2026-01-16)

**Related fixes applied from Codex review:**
- ExtraOptions map mutation (data race) - **FIXED** in `internal/service/chat.go`
- OpenAI streaming/non-streaming parity - **FIXED** in `internal/provider/openai/client.go`

**Recommendations evaluated:**
- Provider registry pattern - **SKIPPED** - Only 3 providers actively used; coupling is in proto schema, not service layer
- Extract shared provider base - **SKIPPED** - Retry logic has material differences between providers
- Extract converters from chat.go - **SKIPPED** - Pure code organization, doesn't reduce complexity

## Detailed Findings

### 1. High Duplication in AI Provider Clients

**Affected Files:**
- `internal/provider/openai/client.go`
- `internal/provider/anthropic/client.go`
- `internal/provider/gemini/client.go`

**Observation:**
The client implementations for OpenAI, Anthropic, and Gemini are nearly identical in structure and logic flow. Specifically:
- **Retry Logic:** All three clients implement their own `for` loop with `maxAttempts` and `sleepWithBackoff` logic.
- **Error Handling:** Identical `isRetryableError` functions exist in each package (checking for 429, 5xx, timeout, etc.).
- **Method Signatures:** `GenerateReply` and `GenerateReplyStream` share almost the same boilerplate for context timeouts, configuration reading, and logging.
- **HTTP Client Setup:** Debug logging (`httpcapture`) and Base URL validation logic is repeated.

**Assessment:** While the structure is similar, the `isRetryableError` functions have provider-specific error semantics (OpenAI uses structured error types, Anthropic detects HTTP 529, Gemini checks specific strings). Consolidating would lose these semantics or require complex parameterization.

**Status:** **SKIPPED** - Appropriate polymorphism, not harmful duplication.

### 2. Tight Coupling in Chat Service

**Affected File:**
- `internal/service/chat.go`

**Observation:**
The `ChatService` struct explicitly defines fields for each provider.

**Assessment:** A registry pattern adds abstraction without solving the real coupling (proto definitions). Adding a new provider requires ~15-20 code changes regardless, primarily in proto enums and FileService switches.

**Status:** **SKIPPED** - Premature abstraction.

### 3. Service Layer Clutter

**Affected File:**
- `internal/service/chat.go`

**Observation:**
The bottom of `chat.go` is filled with purely functional conversion helpers.

**Assessment:** Moving converters to a separate file is pure code organization, not functional refactoring. The converters are tightly coupled to the service.

**Status:** **SKIPPED** - Low impact.

## Refactoring Recommendations

### ~~Recommendation 1: Extract Shared Provider Logic~~ SKIPPED

### ~~Recommendation 2: Implement a Provider Registry~~ SKIPPED

### ~~Recommendation 3: Clean Up Chat Service~~ SKIPPED

### ~~Recommendation 4: Standardize Configuration~~ SKIPPED

## Conclusion

After code review, the proposed architectural refactorings were evaluated and determined to be premature abstractions that wouldn't provide sufficient value for their complexity. The "duplication" across providers is largely appropriate polymorphism - each provider legitimately has different error handling, API quirks, and configuration needs.

The actual bugs fixed were:
1. **ExtraOptions data race** - Direct map assignment caused shared mutable state across tenants
2. **Streaming parity** - Missing options in OpenAI streaming path caused inconsistent behavior
