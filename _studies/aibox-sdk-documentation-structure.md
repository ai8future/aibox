# AIBox SDK Documentation Structure

**Date:** January 3, 2026

## Summary

Planning documentation for the AIBox SDK targeting internal developers. The SDK will support Go and Python, enabling integration with projects like email4ai.

## Target Audience

- **Primary:** Internal developers integrating AIBox into projects like email4ai
- **Familiarity:** Assumes knowledge of the codebase patterns and stack

## SDK Languages

- **Go** - Matches server implementation
- **Python** - Common for AI/ML integrations and scripting

## Documentation Structure

```
docs/
├── README.md                    # Overview + quick links
├── quickstart.md                # 5-minute getting started
├── api-reference.md             # Complete proto/RPC documentation
├── integration-guide.md         # Tenant setup, auth, patterns
├── sdk/
│   ├── go.md                    # Go SDK usage
│   └── python.md                # Python SDK usage
└── examples/
    ├── basic-completion.md
    ├── streaming.md
    ├── multitenancy.md
    └── failover.md
```

## README.md Contents

- One-paragraph description of AIBox
- Feature highlights (3 providers, multitenancy, streaming, failover)
- Links to quickstart and API reference
- Version compatibility table

## AIBox Features to Document

### Core Services (from proto)

1. **AIBoxService**
   - `GenerateReply` - Unary completion
   - `GenerateReplyStream` - Streaming completion
   - `SelectProvider` - Provider selection logic

### Providers

- OpenAI (PROVIDER_OPENAI)
- Gemini (PROVIDER_GEMINI)
- Anthropic (PROVIDER_ANTHROPIC)

### Key Capabilities

- **Multitenancy** - `tenant_id` field for project isolation
- **Streaming** - Real-time token streaming via `GenerateReplyStream`
- **Failover** - Automatic provider failover with `enable_failover`
- **File Search** - RAG with `enable_file_search` and `file_store_id`
- **Web Search** - Grounding with `enable_web_search`
- **Conversation Continuity** - `previous_response_id` for OpenAI threads

### Request Fields (GenerateReplyRequest)

| Field | Type | Description |
|-------|------|-------------|
| tenant_id | string | Tenant identification (required for multitenant) |
| instructions | string | System prompt |
| user_input | string | User message |
| conversation_history | Message[] | Previous messages for context |
| preferred_provider | Provider | Which provider to use |
| model_override | string | Override default model |
| enable_file_search | bool | Enable RAG |
| enable_web_search | bool | Enable web grounding |
| file_store_id | string | Vector store ID |
| previous_response_id | string | For OpenAI conversation continuity |
| provider_configs | map | Per-provider settings override |
| enable_failover | bool | Enable automatic failover |
| client_id | string | Client identifier |
| request_id | string | Request ID for tracing |
| metadata | map | Additional key-value metadata |

### Response Fields (GenerateReplyResponse)

| Field | Type | Description |
|-------|------|-------------|
| text | string | Generated response |
| response_id | string | For conversation continuity |
| usage | Usage | Token metrics |
| citations | Citation[] | Source references |
| model | string | Actual model used |
| provider | Provider | Actual provider used |
| failed_over | bool | Whether failover occurred |
| original_provider | Provider | Provider that failed (if failover) |
| original_error | string | Error message (if failover) |

### Streaming Chunks (GenerateReplyChunk)

- `TextDelta` - Incremental text with index
- `UsageUpdate` - Intermediate token counts
- `CitationUpdate` - Citation during streaming
- `StreamComplete` - Final response metadata
- `StreamError` - Error with code and retryable flag

## Next Steps

1. Write the actual documentation files
2. Create Go SDK wrapper
3. Create Python SDK wrapper
4. Add integration examples for email4ai pattern
