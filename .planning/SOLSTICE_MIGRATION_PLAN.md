# Solstice → Airborne Migration Plan

**Created:** 2026-01-14
**Goal:** Replace Solstice's internal LLM provider code with Airborne gRPC service calls
**Estimated Effort:** 8-11 weeks

---

## Executive Summary

Solstice currently has 24 LLM provider implementations embedded in its codebase (~73 files, ~3,000+ lines of provider code). This plan outlines migrating all LLM interactions to use Airborne as a centralized AI gateway.

**Benefits:**
- Single source of truth for LLM provider management
- Easier provider updates (change once, affects all consumers)
- Better observability and rate limiting
- Reduced code duplication across projects
- Simplified Solstice codebase

---

## Current State Analysis

### Solstice LLM Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Solstice Codebase                       │
├─────────────────────────────────────────────────────────────┤
│  HTTP Webhook                                               │
│       ↓                                                     │
│  Pipeline Service (ProcessEmail)                            │
│       ↓                                                     │
│  Provider Selector (24-way routing)                         │
│       ↓                                                     │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              internal/llm package                    │   │
│  │  ┌─────────┐ ┌─────────┐ ┌───────────┐             │   │
│  │  │ OpenAI  │ │ Gemini  │ │ Anthropic │  + 21 more  │   │
│  │  │ client  │ │ client  │ │  client   │             │   │
│  │  └────┬────┘ └────┬────┘ └─────┬─────┘             │   │
│  │       └───────────┴────────────┘                    │   │
│  │                   ↓                                 │   │
│  │           External LLM APIs                         │   │
│  └─────────────────────────────────────────────────────┘   │
│       ↓                                                     │
│  Response Processor (citations, LaTeX, costs)               │
│       ↓                                                     │
│  Email Sender                                               │
└─────────────────────────────────────────────────────────────┘
```

### Target State

```
┌─────────────────────────────────────────────────────────────┐
│                     Solstice Codebase                       │
├─────────────────────────────────────────────────────────────┤
│  HTTP Webhook                                               │
│       ↓                                                     │
│  Pipeline Service (ProcessEmail)                            │
│       ↓                                                     │
│  ┌─────────────────────────────────────────────────────┐   │
│  │           Airborne gRPC Client                       │   │
│  │  (implements llm.Provider interface)                 │   │
│  └─────────────────────┬───────────────────────────────┘   │
│                        ↓ gRPC                               │
└────────────────────────┼────────────────────────────────────┘
                         ↓
┌────────────────────────┼────────────────────────────────────┐
│                    Airborne Service                         │
├────────────────────────┴────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────┐   │
│  │              Provider Implementations                │   │
│  │  ┌─────────┐ ┌─────────┐ ┌───────────┐             │   │
│  │  │ OpenAI  │ │ Gemini  │ │ Anthropic │  + 21 more  │   │
│  │  └────┬────┘ └────┬────┘ └─────┬─────┘             │   │
│  │       └───────────┴────────────┘                    │   │
│  │                   ↓                                 │   │
│  │           External LLM APIs                         │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  + Rate Limiting, Auth, Observability, Multi-tenant        │
└─────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Core Provider Parity (Weeks 1-3)

**Goal:** Expand Airborne from 3 providers to support the 3 most-used providers with full feature parity.

### 1.1 OpenAI Provider Enhancement

**Current Airborne:** Basic chat completion
**Solstice Features to Add:**

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| Responses API (background polling) | P0 | High | Required for conversation continuity |
| File Search / Vector Stores | P0 | High | RAG functionality |
| Web Search | P1 | Medium | `WebSearchEnabled` flag |
| Reasoning Effort | P1 | Low | `reasoning_effort` parameter |
| Service Tier | P2 | Low | `service_tier` parameter |
| Prompt Cache Retention | P2 | Low | `prompt_cache_retention` |
| Verbosity Level | P2 | Low | Custom parameter |

**Files to Reference:**
- `/Users/cliff/Desktop/_code/solstice/internal/llm/openai/client.go` (575 lines)
- `/Users/cliff/Desktop/_code/solstice/internal/llm/client.go` (legacy, 530 lines)

**Tasks:**
- [ ] Implement OpenAI Responses API support
- [ ] Add vector store creation/management
- [ ] Add file upload to vector stores
- [ ] Add web search toggle
- [ ] Add reasoning effort parameter
- [ ] Add retry logic with exponential backoff
- [ ] Port citation extraction logic

### 1.2 Gemini Provider Enhancement

**Current Airborne:** Basic chat completion
**Solstice Features to Add:**

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| File Search Store | P0 | High | Different from OpenAI vector stores |
| Inline Images | P0 | Medium | Multimodal support |
| Structured Output (JSON mode) | P0 | Medium | Entity/intent extraction |
| Web Search | P1 | Medium | Grounding with Google Search |
| Thinking Level/Budget | P1 | Low | Gemini 2.0 feature |
| Safety Thresholds | P2 | Low | Content filtering |

**Files to Reference:**
- `/Users/cliff/Desktop/_code/solstice/internal/llm/gemini/client.go`
- `/Users/cliff/Desktop/_code/solstice/internal/ingest/gemini/ingestor.go` (998 lines)

**Tasks:**
- [ ] Implement Gemini file search store API
- [ ] Add inline image support in messages
- [ ] Implement structured output with schema
- [ ] Add web search (grounding) support
- [ ] Add thinking level/budget parameters
- [ ] Port file ingestion logic

### 1.3 Anthropic Provider Enhancement

**Current Airborne:** Basic chat completion
**Solstice Features to Add:**

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| Extended Thinking | P0 | Medium | `thinking` block support |
| Thinking Budget | P1 | Low | Token budget for thinking |
| PDF Support | P1 | Medium | Native PDF in messages |

**Files to Reference:**
- `/Users/cliff/Desktop/_code/solstice/internal/llm/anthropic/client.go` (442 lines)

**Tasks:**
- [ ] Implement extended thinking support
- [ ] Add thinking budget parameter
- [ ] Add PDF document support
- [ ] Update message format for thinking blocks

### 1.4 Proto/API Updates

**New Proto Fields for GenerateRequest:**

```protobuf
message GenerateRequest {
  // Existing fields...

  // Phase 1 additions
  string previous_response_id = 20;      // OpenAI conversation continuity
  string vector_store_id = 21;           // OpenAI file search
  string file_search_store_id = 22;      // Gemini file search
  repeated InlineImage inline_images = 23;
  bool web_search_enabled = 24;
  string reasoning_effort = 25;          // "low", "medium", "high"
  int32 thinking_budget = 26;            // Max tokens for thinking
  bool structured_output_enabled = 27;
  string structured_output_schema = 28;  // JSON schema
}

message InlineImage {
  string uri = 1;
  string mime_type = 2;
  string filename = 3;
}

message GenerateResponse {
  // Existing fields...

  // Phase 1 additions
  string response_id = 10;               // For continuity
  repeated Citation citations = 11;
  repeated GeneratedImage images = 12;
  StructuredMetadata structured_metadata = 13;
  string request_json = 14;              // Debug
  string response_json = 15;             // Debug
}
```

### 1.5 Deliverables

- [ ] Updated proto definitions
- [ ] OpenAI provider with Responses API
- [ ] Gemini provider with file search + structured output
- [ ] Anthropic provider with thinking support
- [ ] Integration tests for all three providers
- [ ] Documentation for new API fields

---

## Phase 2: File Handling API (Weeks 4-5)

**Goal:** Add file upload and management APIs to Airborne for RAG functionality.

### 2.1 New gRPC Services

```protobuf
service FileService {
  // Upload file to provider-specific store
  rpc UploadFile(UploadFileRequest) returns (UploadFileResponse);

  // Create a new file store (vector store / file search store)
  rpc CreateFileStore(CreateFileStoreRequest) returns (CreateFileStoreResponse);

  // Delete a file store
  rpc DeleteFileStore(DeleteFileStoreRequest) returns (DeleteFileStoreResponse);

  // List files in a store
  rpc ListFiles(ListFilesRequest) returns (ListFilesResponse);
}

message UploadFileRequest {
  string provider = 1;           // "openai", "gemini"
  string store_id = 2;           // Target store (optional, creates if empty)
  bytes content = 3;
  string filename = 4;
  string mime_type = 5;
}

message UploadFileResponse {
  string file_id = 1;
  string store_id = 2;           // Returns store ID (new or existing)
  string provider = 3;
}
```

### 2.2 Provider-Specific Implementation

**OpenAI Vector Stores:**
- Create vector store via API
- Upload files with chunking
- Poll for processing completion
- Return vector store ID for use in chat

**Gemini File Search:**
- Upload to Gemini Files API
- Create corpus/document structure
- Return file search store ID

### 2.3 Tasks

- [ ] Define FileService proto
- [ ] Implement OpenAI vector store operations
- [ ] Implement Gemini file upload operations
- [ ] Add store ID caching/reuse logic
- [ ] Handle file processing status polling
- [ ] Add cleanup/expiration handling

---

## Phase 3: Additional Providers (Weeks 6-9)

**Goal:** Port remaining 21 providers from Solstice to Airborne.

### Provider Priority Tiers

**Tier 1 - High Usage (Week 6):**
| Provider | Complexity | Notes |
|----------|------------|-------|
| DeepSeek | Medium | Reasoning model, popular |
| Grok | Low | xAI, straightforward |
| Mistral | Low | Standard OpenAI-compatible |
| Perplexity | Low | Search-focused |

**Tier 2 - Enterprise (Week 7):**
| Provider | Complexity | Notes |
|----------|------------|-------|
| Bedrock (AWS) | High | Multiple models, AWS auth |
| Watsonx (IBM) | Medium | IBM Cloud auth |
| Databricks | Medium | Workspace auth |
| Cohere | Low | Standard API |

**Tier 3 - Inference Platforms (Week 8):**
| Provider | Complexity | Notes |
|----------|------------|-------|
| Together | Low | OpenAI-compatible |
| Fireworks | Low | OpenAI-compatible |
| OpenRouter | Low | Multi-provider routing |
| DeepInfra | Low | OpenAI-compatible |
| Baseten | Medium | Custom deployment |
| Hyperbolic | Low | OpenAI-compatible |

**Tier 4 - Specialized (Week 9):**
| Provider | Complexity | Notes |
|----------|------------|-------|
| HuggingFace | Medium | Inference API |
| Predibase | Medium | Fine-tuned models |
| Parasail | Low | OpenAI-compatible |
| Upstage | Low | Solar models |
| Nebius | Low | OpenAI-compatible |
| Cerebras | Low | Fast inference |
| MiniMax | Medium | Chinese provider |

### 3.1 Provider Implementation Template

Each provider needs:
1. Client struct with configuration
2. `GenerateReply` implementation
3. Error classification (retryable vs fatal)
4. Feature flags (file search, web search, etc.)
5. Unit tests with mocked responses

### 3.2 Tasks

- [ ] Create provider implementation template
- [ ] Port Tier 1 providers (4 providers)
- [ ] Port Tier 2 providers (4 providers)
- [ ] Port Tier 3 providers (6 providers)
- [ ] Port Tier 4 providers (7 providers)
- [ ] Integration tests for each provider
- [ ] Update proto with all provider names

---

## Phase 4: Advanced Features (Weeks 10-11)

**Goal:** Achieve full feature parity with Solstice's LLM capabilities.

### 4.1 Provider Selection & Failover

**Implement in Airborne:**
```protobuf
message GenerateRequest {
  // Provider selection options
  string preferred_provider = 30;
  string fallback_provider = 31;
  repeated ProviderTrigger triggers = 32;  // Phrase-based routing
}

message ProviderTrigger {
  string phrase = 1;
  string provider = 2;
}
```

**Or keep in Solstice:**
- Provider selection logic stays in Solstice
- Airborne receives explicit provider choice
- Simpler Airborne, smarter Solstice

**Recommendation:** Keep provider selection in Solstice (domain-specific), Airborne just executes.

### 4.2 Streaming Support

Solstice doesn't use streaming currently, but Airborne already supports it. Ensure:
- [ ] All 24 providers support streaming in Airborne
- [ ] Streaming works with thinking/reasoning modes
- [ ] Citation extraction works with streamed responses

### 4.3 Conversation Continuity

Two modes:
1. **Native (OpenAI):** Use `previous_response_id`
2. **Cross-provider:** Pass full message history

Airborne should support both:
- [ ] Accept `previous_response_id` for OpenAI
- [ ] Accept message history for all providers
- [ ] Handle provider switching mid-conversation

### 4.4 Structured Output

Gemini-specific currently, but could expand:
- [ ] Gemini structured output with JSON schema
- [ ] OpenAI function calling for structured data
- [ ] Anthropic tool use for structured extraction
- [ ] Unified structured output interface

### 4.5 Tasks

- [ ] Decide provider selection location (Airborne vs Solstice)
- [ ] Implement failover wrapper if in Airborne
- [ ] Verify streaming for all providers
- [ ] Implement cross-provider continuity
- [ ] Unify structured output interface
- [ ] Add request/response JSON capture for debugging

---

## Phase 5: Solstice Integration (Week 11+)

**Goal:** Replace Solstice's internal LLM code with Airborne client.

### 5.1 Create Airborne Client in Solstice

```go
// internal/llm/airborne/client.go

type Client struct {
    conn   *grpc.ClientConn
    client airbornev1.AIBoxServiceClient
}

// Implements llm.Provider interface
func (c *Client) GenerateReply(ctx context.Context, params llm.GenerateParams) (llm.GenerateResult, error) {
    req := &airbornev1.GenerateRequest{
        Provider:           params.Provider,
        Model:              params.Model,
        Messages:           convertMessages(params.Messages),
        Temperature:        params.Temperature,
        MaxTokens:          params.MaxTokens,
        // ... map all fields
    }

    resp, err := c.client.GenerateReply(ctx, req)
    if err != nil {
        return llm.GenerateResult{}, mapGRPCError(err)
    }

    return llm.GenerateResult{
        Text:       resp.Content,
        ResponseID: resp.ResponseId,
        Citations:  convertCitations(resp.Citations),
        // ... map all fields
    }, nil
}
```

### 5.2 Migration Steps

1. **Deploy Airborne** with all 24 providers
2. **Add Airborne client** to Solstice as new provider option
3. **Feature flag** to route traffic: `USE_AIRBORNE=true`
4. **Shadow mode:** Call both, compare results, log differences
5. **Gradual rollout:** 10% → 50% → 100% traffic
6. **Remove old code:** Delete `internal/llm/*` providers after stable

### 5.3 Rollback Plan

- Feature flag `USE_AIRBORNE=false` instantly reverts
- Keep old provider code for 2 releases
- Monitor error rates, latency, costs during rollout

### 5.4 Tasks

- [ ] Create Airborne gRPC client in Solstice
- [ ] Implement llm.Provider interface wrapper
- [ ] Add feature flag for traffic routing
- [ ] Implement shadow mode comparison
- [ ] Create monitoring dashboard
- [ ] Document rollout procedure
- [ ] Plan code removal timeline

---

## Risk Mitigation

### Technical Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Feature gap discovered late | Medium | High | Shadow mode testing before cutover |
| Provider API changes | Low | Medium | Pin SDK versions, monitor changelogs |
| Latency increase from extra hop | Medium | Low | Deploy Airborne close to Solstice, measure |
| Auth/rate limit complexity | Low | Medium | Test multi-tenant scenarios early |

### Operational Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Deployment coordination | Medium | Medium | Feature flags, gradual rollout |
| Debugging complexity | Medium | Low | Request/response JSON logging |
| Team knowledge gap | Low | Medium | Document Airborne thoroughly |

---

## Success Metrics

### Phase Completion Criteria

- **Phase 1:** 3 core providers pass integration tests with Solstice params
- **Phase 2:** File upload/download works for OpenAI and Gemini
- **Phase 3:** All 24 providers implemented and tested
- **Phase 4:** Failover and continuity work across providers
- **Phase 5:** 100% Solstice traffic through Airborne for 2 weeks

### Performance Targets

- Latency: < 50ms overhead from Airborne hop
- Error rate: No increase from direct provider calls
- Cost: No change (same tokens, same providers)

---

## Appendix A: Solstice Provider Feature Matrix

| Provider | File Search | Web Search | Streaming | Thinking | Structured |
|----------|-------------|------------|-----------|----------|------------|
| OpenAI | Yes | Yes | Yes | No | No |
| Gemini | Yes | Yes | Yes | Yes | Yes |
| Anthropic | No | No | Yes | Yes | No |
| DeepSeek | No | No | Yes | Yes | No |
| Grok | No | No | Yes | No | No |
| Perplexity | No | Yes | Yes | No | No |
| Bedrock | No | No | Yes | Varies | No |
| Others | No | No | Yes | No | No |

---

## Appendix B: File References

**Solstice LLM Code:**
- `internal/llm/provider.go` - Interface definitions (279 lines)
- `internal/llm/selector.go` - Provider selection logic
- `internal/llm/failover.go` - Failover wrapper
- `internal/llm/openai/client.go` - OpenAI implementation (575 lines)
- `internal/llm/gemini/client.go` - Gemini implementation
- `internal/llm/anthropic/client.go` - Anthropic implementation (442 lines)
- `internal/ingest/gemini/ingestor.go` - File ingestion (998 lines)
- `internal/pipeline/service.go` - Main orchestration

**Airborne Current Code:**
- `internal/provider/provider.go` - Interface definitions (162 lines)
- `internal/provider/openai/client.go` - OpenAI implementation
- `internal/provider/gemini/client.go` - Gemini implementation
- `internal/provider/anthropic/client.go` - Anthropic implementation

---

*Plan created: 2026-01-14*
*Last updated: 2026-01-14*
