# Code Test Report 2026-01-07-16

## Scope & Untested Areas
- `cmd/aibox` (main entrypoint)
- `internal/provider/openai`
- `internal/provider/anthropic`
- `internal/provider/gemini`
- `internal/redis`
- `internal/tenant`
- `internal/rag/testutil`

## Proposed Unit Test Coverage

### cmd/aibox
- Validate `main` exits non-zero when configuration validation fails (ex: `AIBOX_GRPC_PORT=0`).
- Note: successful startup, gRPC server creation, and Redis-dependent paths are better handled via integration tests with controlled Redis and port binding.

### internal/provider/openai
- `buildUserPrompt`: trims input, formats history with role labeling, includes separators.
- `mapReasoningEffort` and `mapServiceTier`: case-insensitive mappings with defaults.
- `isRetryableError`: OpenAI API status code mapping + string-based fallbacks.
- `waitForCompletion`: error on nil response, short-circuit on completed status or empty ID.
- `extractCitations`: URL and file citations from response output JSON and filename override mapping.

### internal/provider/anthropic
- `buildMessages`: conversation history mapping, prepended placeholder when history starts with assistant, input trimming.
- `extractText`: pulls only text blocks and trims output; ignores non-text blocks.
- `isRetryableError`: string-based retry detection (rate limits, overloads).

### internal/provider/gemini
- `buildContents`: role mapping (assistant -> model), input trimming, history ordering.
- `extractText`: concatenates candidate content parts; skips nil candidates.
- `extractUsage`: nil-safe conversion from usage metadata.
- `extractCitations`: grounding metadata -> URL citations.
- `buildSafetySettings`: threshold parsing and default behavior.
- `isRetryableError`: string-based retry detection.

### internal/redis
- Exercise wrapper methods against a local in-memory Redis (`miniredis`):
  - `NewClient` success + failure, `Ping`, `Set/Get/Del/Exists/TTL`, `Incr/IncrBy`, hash ops, `Eval`.
- Validate `IsNil` behavior with `redis.Nil`.
- Adds dev-only dependency on `github.com/alicebob/miniredis/v2` (diff below).

### internal/tenant
- `TenantConfig.GetProvider` and `DefaultProvider` (failover ordering vs fallback).
- `loadEnv`: defaults, overrides, invalid port, TLS validation.
- `loadSecret` and `resolveSecrets`: ENV/FILE/${VAR}/inline handling.
- `validateTenantConfig`: required fields, bounds checking, failover validation.
- `loadTenants`: JSON/YAML parsing, skipping non-configs, secret resolution, duplicates.
- `Manager` behaviors: sorted tenant codes, default tenant, reload diff results.

### internal/rag/testutil
- `MockEmbedder` default/custom behavior and call tracking.
- `MockStore` create/upsert/search/delete lifecycle + call tracking.
- `MockExtractor` default response, call tracking, supported formats.
- `RandomEmbedding` and `SampleText` helper sanity checks.

## Patch-Ready Diffs

### `cmd/aibox/main_test.go`
```diff
diff --git a/cmd/aibox/main_test.go b/cmd/aibox/main_test.go
new file mode 100644
--- /dev/null
+++ b/cmd/aibox/main_test.go
@@ -0,0 +1,44 @@
+package main
+
+import (
+\t"os"
+\t"os/exec"
+\t"testing"
+)
+
+func TestMain_InvalidConfigExits(t *testing.T) {
+\tcmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "main")
+\tcmd.Env = append(os.Environ(),
+\t\t"GO_WANT_HELPER_PROCESS=1",
+\t\t"AIBOX_GRPC_PORT=0",
+\t)
+
+\terr := cmd.Run()
+\tif err == nil {
+\t\tt.Fatal("expected non-zero exit status")
+\t}
+
+\tif exitErr, ok := err.(*exec.ExitError); ok {
+\t\tif exitErr.ExitCode() == 0 {
+\t\t\tt.Fatalf("expected non-zero exit status, got %d", exitErr.ExitCode())
+\t\t}
+\t\treturn
+\t}
+
+\tt.Fatalf("expected ExitError, got %T", err)
+}
+
+func TestHelperProcess(t *testing.T) {
+\tif os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
+\t\treturn
+\t}
+
+\tif len(os.Args) > 3 && os.Args[3] == "main" {
+\t\tmain()
+\t}
+\tos.Exit(0)
+}
```

### `internal/provider/openai/client_test.go`
```diff
diff --git a/internal/provider/openai/client_test.go b/internal/provider/openai/client_test.go
new file mode 100644
--- /dev/null
+++ b/internal/provider/openai/client_test.go
@@ -0,0 +1,178 @@
+package openai
+
+import (
+\t"context"
+\t"encoding/json"
+\t"errors"
+\t"testing"
+
+\topenai "github.com/openai/openai-go"
+\t"github.com/openai/openai-go/responses"
+\t"github.com/openai/openai-go/shared"
+
+\t"github.com/cliffpyles/aibox/internal/provider"
+)
+
+func TestBuildUserPrompt_NoHistory(t *testing.T) {
+\tgot := buildUserPrompt("  hello  ", nil)
+\tif got != "hello" {
+\t\tt.Fatalf("buildUserPrompt() = %q, want %q", got, "hello")
+\t}
+}
+
+func TestBuildUserPrompt_WithHistory(t *testing.T) {
+\thistory := []provider.Message{
+\t\t{Role: "user", Content: "Hello"},
+\t\t{Role: "assistant", Content: "Hi there"},
+\t}
+
+\tgot := buildUserPrompt("  How are you?  ", history)
+\twant := "Previous conversation:\n\nUser: Hello\n\nAssistant: Hi there\n\n---\n\nNew message:\n\nHow are you?"
+\tif got != want {
+\t\tt.Fatalf("buildUserPrompt() = %q, want %q", got, want)
+\t}
+}
+
+func TestMapReasoningEffort(t *testing.T) {
+\ttests := []struct {
+\t\tname  string
+\t\tinput string
+\t\twant  shared.ReasoningEffort
+\t}{
+\t\t{"none", "none", shared.ReasoningEffort("none")},
+\t\t{"low", "LOW", shared.ReasoningEffortLow},
+\t\t{"medium", "Medium", shared.ReasoningEffortMedium},
+\t\t{"high", "high", shared.ReasoningEffortHigh},
+\t\t{"default", "unknown", shared.ReasoningEffortHigh},
+\t}
+
+\tfor _, tt := range tests {
+\t\tt.Run(tt.name, func(t *testing.T) {
+\t\t\tgot := mapReasoningEffort(tt.input)
+\t\t\tif got != tt.want {
+\t\t\t\tt.Fatalf("mapReasoningEffort(%q) = %q, want %q", tt.input, got, tt.want)
+\t\t\t}
+\t\t})
+\t}
+}
+
+func TestMapServiceTier(t *testing.T) {
+\ttests := []struct {
+\t\tinput string
+\t\twant  responses.ResponseNewParamsServiceTier
+\t}{
+\t\t{"default", responses.ResponseNewParamsServiceTierDefault},
+\t\t{"flex", responses.ResponseNewParamsServiceTierFlex},
+\t\t{"priority", responses.ResponseNewParamsServiceTierPriority},
+\t\t{"unknown", responses.ResponseNewParamsServiceTierAuto},
+\t}
+
+\tfor _, tt := range tests {
+\t\tgot := mapServiceTier(tt.input)
+\t\tif got != tt.want {
+\t\t\tt.Fatalf("mapServiceTier(%q) = %q, want %q", tt.input, got, tt.want)
+\t\t}
+\t}
+}
+
+func TestIsRetryableError(t *testing.T) {
+\tif isRetryableError(nil) {
+\t\tt.Fatalf("isRetryableError(nil) = true, want false")
+\t}
+
+\tif !isRetryableError(&openai.Error{StatusCode: 429}) {
+\t\tt.Fatalf("expected retryable for 429")
+\t}
+\tif isRetryableError(&openai.Error{StatusCode: 400}) {
+\t\tt.Fatalf("expected non-retryable for 400")
+\t}
+\tif !isRetryableError(errors.New("temporary connection failure")) {
+\t\tt.Fatalf("expected retryable for connection failure")
+\t}
+\tif isRetryableError(errors.New("bad request")) {
+\t\tt.Fatalf("expected non-retryable for generic error")
+\t}
+}
+
+func TestWaitForCompletion_NilResponse(t *testing.T) {
+\t_, err := waitForCompletion(context.Background(), openai.Client{}, nil)
+\tif err == nil {
+\t\tt.Fatal("expected error for nil response")
+\t}
+}
+
+func TestWaitForCompletion_CompletedOrNoID(t *testing.T) {
+\tresp := &responses.Response{ID: "resp_1", Status: responses.ResponseStatusCompleted}
+\tgot, err := waitForCompletion(context.Background(), openai.Client{}, resp)
+\tif err != nil {
+\t\tt.Fatalf("unexpected error: %v", err)
+\t}
+\tif got != resp {
+\t\tt.Fatalf("expected same response pointer")
+\t}
+
+\trespNoID := &responses.Response{}
+\tgot, err = waitForCompletion(context.Background(), openai.Client{}, respNoID)
+\tif err != nil {
+\t\tt.Fatalf("unexpected error: %v", err)
+\t}
+\tif got != respNoID {
+\t\tt.Fatalf("expected same response pointer for empty ID")
+\t}
+}
+
+func TestExtractCitations(t *testing.T) {
+\traw := `{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"See sources","annotations":[{"type":"url_citation","url":"https://example.com","title":"Example","start_index":0,"end_index":3},{"type":"file_citation","file_id":"file_123","filename":"orig.pdf","index":7}]}]}`
+\tvar item responses.ResponseOutputItemUnion
+\tif err := json.Unmarshal([]byte(raw), &item); err != nil {
+\t\tt.Fatalf("unmarshal output item: %v", err)
+\t}
+
+\tresp := &responses.Response{Output: []responses.ResponseOutputItemUnion{item}}
+\tcitations := extractCitations(resp, map[string]string{"file_123": "mapped.pdf"})
+
+\tif len(citations) != 2 {
+\t\tt.Fatalf("expected 2 citations, got %d", len(citations))
+\t}
+
+\tif citations[0].Type != provider.CitationTypeURL || citations[0].URL != "https://example.com" || citations[0].Title != "Example" {
+\t\tt.Fatalf("unexpected url citation: %#v", citations[0])
+\t}
+\tif citations[0].StartIndex != 0 || citations[0].EndIndex != 3 {
+\t\tt.Fatalf("unexpected url citation indexes: %#v", citations[0])
+\t}
+
+\tif citations[1].Type != provider.CitationTypeFile || citations[1].FileID != "file_123" || citations[1].Filename != "mapped.pdf" {
+\t\tt.Fatalf("unexpected file citation: %#v", citations[1])
+\t}
+\tif citations[1].StartIndex != 7 {
+\t\tt.Fatalf("unexpected file citation index: %#v", citations[1])
+\t}
+}
```

### `internal/provider/anthropic/client_test.go`
```diff
diff --git a/internal/provider/anthropic/client_test.go b/internal/provider/anthropic/client_test.go
new file mode 100644
--- /dev/null
+++ b/internal/provider/anthropic/client_test.go
@@ -0,0 +1,120 @@
+package anthropic
+
+import (
+\t"encoding/json"
+\t"errors"
+\t"testing"
+
+\tanthropic "github.com/anthropics/anthropic-sdk-go"
+
+\t"github.com/cliffpyles/aibox/internal/provider"
+)
+
+func TestBuildMessages_PrependsUserWhenAssistantFirst(t *testing.T) {
+\thistory := []provider.Message{
+\t\t{Role: "assistant", Content: "Hi"},
+\t}
+
+\tmessages := buildMessages("  How are you?  ", history)
+\tif len(messages) != 3 {
+\t\tt.Fatalf("expected 3 messages, got %d", len(messages))
+\t}
+
+\tif messages[0].Role != anthropic.MessageParamRoleUser {
+\t\tt.Fatalf("expected first message role user, got %s", messages[0].Role)
+\t}
+\tif messages[0].Content[0].OfText == nil || messages[0].Content[0].OfText.Text != "[continuing conversation]" {
+\t\tt.Fatalf("unexpected placeholder message: %#v", messages[0].Content[0])
+\t}
+
+\tif messages[1].Role != anthropic.MessageParamRoleAssistant {
+\t\tt.Fatalf("expected second message role assistant, got %s", messages[1].Role)
+\t}
+\tif messages[1].Content[0].OfText == nil || messages[1].Content[0].OfText.Text != "Hi" {
+\t\tt.Fatalf("unexpected assistant message content: %#v", messages[1].Content[0])
+\t}
+
+\tif messages[2].Role != anthropic.MessageParamRoleUser {
+\t\tt.Fatalf("expected final message role user, got %s", messages[2].Role)
+\t}
+\tif messages[2].Content[0].OfText == nil || messages[2].Content[0].OfText.Text != "How are you?" {
+\t\tt.Fatalf("unexpected final message content: %#v", messages[2].Content[0])
+\t}
+}
+
+func TestBuildMessages_NormalHistory(t *testing.T) {
+\thistory := []provider.Message{
+\t\t{Role: "user", Content: "Hello"},
+\t\t{Role: "assistant", Content: "Hi"},
+\t}
+
+\tmessages := buildMessages("  Next  ", history)
+\tif len(messages) != 3 {
+\t\tt.Fatalf("expected 3 messages, got %d", len(messages))
+\t}
+
+\tif messages[0].Content[0].OfText == nil || messages[0].Content[0].OfText.Text != "Hello" {
+\t\tt.Fatalf("unexpected first message content: %#v", messages[0].Content[0])
+\t}
+\tif messages[1].Content[0].OfText == nil || messages[1].Content[0].OfText.Text != "Hi" {
+\t\tt.Fatalf("unexpected second message content: %#v", messages[1].Content[0])
+\t}
+\tif messages[2].Content[0].OfText == nil || messages[2].Content[0].OfText.Text != "Next" {
+\t\tt.Fatalf("unexpected final message content: %#v", messages[2].Content[0])
+\t}
+}
+
+func TestExtractText(t *testing.T) {
+\tvar textBlock anthropic.ContentBlockUnion
+\tif err := json.Unmarshal([]byte(`{"type":"text","text":"Hello "}`), &textBlock); err != nil {
+\t\tt.Fatalf("unmarshal text block: %v", err)
+\t}
+\tvar thinkingBlock anthropic.ContentBlockUnion
+\tif err := json.Unmarshal([]byte(`{"type":"thinking","thinking":"secret"}`), &thinkingBlock); err != nil {
+\t\tt.Fatalf("unmarshal thinking block: %v", err)
+\t}
+
+\tmsg := &anthropic.Message{Content: []anthropic.ContentBlockUnion{textBlock, thinkingBlock}}
+\tgot := extractText(msg)
+\tif got != "Hello" {
+\t\tt.Fatalf("extractText() = %q, want %q", got, "Hello")
+\t}
+}
+
+func TestExtractText_Nil(t *testing.T) {
+\tif got := extractText(nil); got != "" {
+\t\tt.Fatalf("extractText(nil) = %q, want empty", got)
+\t}
+}
+
+func TestIsRetryableError(t *testing.T) {
+\tif isRetryableError(nil) {
+\t\tt.Fatalf("isRetryableError(nil) = true, want false")
+\t}
+\tif !isRetryableError(errors.New("429 too many requests")) {
+\t\tt.Fatalf("expected retryable for rate limit")
+\t}
+\tif !isRetryableError(errors.New("service overloaded")) {
+\t\tt.Fatalf("expected retryable for overload")
+\t}
+\tif isRetryableError(errors.New("bad request")) {
+\t\tt.Fatalf("expected non-retryable for generic error")
+\t}
+}
```

### `internal/provider/gemini/client_test.go`
```diff
diff --git a/internal/provider/gemini/client_test.go b/internal/provider/gemini/client_test.go
new file mode 100644
--- /dev/null
+++ b/internal/provider/gemini/client_test.go
@@ -0,0 +1,130 @@
+package gemini
+
+import (
+\t"errors"
+\t"testing"
+
+\t"google.golang.org/genai"
+
+\t"github.com/cliffpyles/aibox/internal/provider"
+)
+
+func TestBuildContents(t *testing.T) {
+\thistory := []provider.Message{
+\t\t{Role: "user", Content: "Hello"},
+\t\t{Role: "assistant", Content: "Hi"},
+\t}
+
+\tcontents := buildContents("  Next  ", history)
+\tif len(contents) != 3 {
+\t\tt.Fatalf("expected 3 contents, got %d", len(contents))
+\t}
+
+\tif contents[0].Role != "user" {
+\t\tt.Fatalf("expected role user, got %q", contents[0].Role)
+\t}
+\tif contents[1].Role != "model" {
+\t\tt.Fatalf("expected role model, got %q", contents[1].Role)
+\t}
+\tif contents[2].Parts[0].Text != "Next" {
+\t\tt.Fatalf("expected trimmed input, got %q", contents[2].Parts[0].Text)
+\t}
+}
+
+func TestExtractText(t *testing.T) {
+\tresp := &genai.GenerateContentResponse{
+\t\tCandidates: []*genai.Candidate{
+\t\t\t{Content: &genai.Content{Parts: []*genai.Part{{Text: "Hello "}, {Text: "world"}}}},
+\t\t\t{Content: nil},
+\t\t},
+\t}
+
+\tgot := extractText(resp)
+\tif got != "Hello world" {
+\t\tt.Fatalf("extractText() = %q, want %q", got, "Hello world")
+\t}
+
+\tif extractText(nil) != "" {
+\t\tt.Fatalf("extractText(nil) should be empty")
+\t}
+}
+
+func TestExtractUsage(t *testing.T) {
+\tresp := &genai.GenerateContentResponse{
+\t\tUsageMetadata: &genai.GenerateContentResponseUsageMetadata{
+\t\t\tPromptTokenCount:     10,
+\t\t\tCandidatesTokenCount: 20,
+\t\t\tTotalTokenCount:      30,
+\t\t},
+\t}
+
+\tusage := extractUsage(resp)
+\tif usage == nil || usage.InputTokens != 10 || usage.OutputTokens != 20 || usage.TotalTokens != 30 {
+\t\tt.Fatalf("unexpected usage: %#v", usage)
+\t}
+
+\tif extractUsage(&genai.GenerateContentResponse{}) != nil {
+\t\tt.Fatalf("expected nil usage when metadata missing")
+\t}
+}
+
+func TestExtractCitations(t *testing.T) {
+\tresp := &genai.GenerateContentResponse{
+\t\tCandidates: []*genai.Candidate{
+\t\t\t{
+\t\t\t\tGroundingMetadata: &genai.GroundingMetadata{
+\t\t\t\t\tGroundingChunks: []*genai.GroundingChunk{
+\t\t\t\t\t\t{Web: &genai.GroundingChunkWeb{URI: "https://example.com", Title: "Example"}},
+\t\t\t\t\t\t{Web: nil},
+\t\t\t\t\t},
+\t\t\t\t},
+\t\t\t},
+\t\t},
+\t}
+
+\tcitations := extractCitations(resp, nil)
+\tif len(citations) != 1 {
+\t\tt.Fatalf("expected 1 citation, got %d", len(citations))
+\t}
+\tif citations[0].Type != provider.CitationTypeURL || citations[0].URL != "https://example.com" || citations[0].Title != "Example" {
+\t\tt.Fatalf("unexpected citation: %#v", citations[0])
+\t}
+}
+
+func TestBuildSafetySettings(t *testing.T) {
+\tsettings := buildSafetySettings("LOW_AND_ABOVE")
+\tif len(settings) != 4 {
+\t\tt.Fatalf("expected 4 settings, got %d", len(settings))
+\t}
+\tfor _, setting := range settings {
+\t\tif setting.Threshold != genai.HarmBlockThresholdBlockLowAndAbove {
+\t\t\tt.Fatalf("unexpected threshold: %v", setting.Threshold)
+\t\t}
+\t}
+
+\tsettings = buildSafetySettings("unknown")
+\tif settings[0].Threshold != genai.HarmBlockThresholdBlockMediumAndAbove {
+\t\tt.Fatalf("unexpected default threshold: %v", settings[0].Threshold)
+\t}
+}
+
+func TestIsRetryableError(t *testing.T) {
+\tif isRetryableError(nil) {
+\t\tt.Fatalf("isRetryableError(nil) = true, want false")
+\t}
+\tif !isRetryableError(errors.New("resource exhausted")) {
+\t\tt.Fatalf("expected retryable for resource exhaustion")
+\t}
+\tif isRetryableError(errors.New("bad request")) {
+\t\tt.Fatalf("expected non-retryable for generic error")
+\t}
+}
```

### `internal/redis/client_test.go`
```diff
diff --git a/internal/redis/client_test.go b/internal/redis/client_test.go
new file mode 100644
--- /dev/null
+++ b/internal/redis/client_test.go
@@ -0,0 +1,164 @@
+package redis
+
+import (
+\t"context"
+\t"errors"
+\t"testing"
+\t"time"
+
+\t"github.com/alicebob/miniredis/v2"
+\t"github.com/redis/go-redis/v9"
+)
+
+func newTestClient(t *testing.T) *Client {
+\tserver := miniredis.RunT(t)
+\tclient, err := NewClient(Config{Addr: server.Addr()})
+\tif err != nil {
+\t\tt.Fatalf("NewClient failed: %v", err)
+\t}
+\tt.Cleanup(func() {
+\t\t_ = client.Close()
+\t})
+\treturn client
+}
+
+func TestNewClient_PingFailure(t *testing.T) {
+\t_, err := NewClient(Config{Addr: "127.0.0.1:0"})
+\tif err == nil {
+\t\tt.Fatalf("expected error for invalid redis address")
+\t}
+}
+
+func TestClient_BasicKV(t *testing.T) {
+\tclient := newTestClient(t)
+\tctx := context.Background()
+
+\tif err := client.Ping(ctx); err != nil {
+\t\tt.Fatalf("Ping failed: %v", err)
+\t}
+
+\tif err := client.Set(ctx, "key", "value", time.Second); err != nil {
+\t\tt.Fatalf("Set failed: %v", err)
+\t}
+\tvalue, err := client.Get(ctx, "key")
+\tif err != nil {
+\t\tt.Fatalf("Get failed: %v", err)
+\t}
+\tif value != "value" {
+\t\tt.Fatalf("Get returned %q, want %q", value, "value")
+\t}
+
+\texists, err := client.Exists(ctx, "key")
+\tif err != nil {
+\t\tt.Fatalf("Exists failed: %v", err)
+\t}
+\tif exists != 1 {
+\t\tt.Fatalf("Exists returned %d, want 1", exists)
+\t}
+
+\tttl, err := client.TTL(ctx, "key")
+\tif err != nil {
+\t\tt.Fatalf("TTL failed: %v", err)
+\t}
+\tif ttl <= 0 {
+\t\tt.Fatalf("expected positive TTL, got %v", ttl)
+\t}
+
+\tif err := client.Del(ctx, "key"); err != nil {
+\t\tt.Fatalf("Del failed: %v", err)
+\t}
+\t_, err = client.Get(ctx, "key")
+\tif !IsNil(err) {
+\t\tt.Fatalf("expected redis.Nil after delete, got %v", err)
+\t}
+}
+
+func TestClient_Counters(t *testing.T) {
+\tclient := newTestClient(t)
+\tctx := context.Background()
+
+\tvalue, err := client.Incr(ctx, "counter")
+\tif err != nil {
+\t\tt.Fatalf("Incr failed: %v", err)
+\t}
+\tif value != 1 {
+\t\tt.Fatalf("Incr returned %d, want 1", value)
+\t}
+
+\tvalue, err = client.IncrBy(ctx, "counter", 4)
+\tif err != nil {
+\t\tt.Fatalf("IncrBy failed: %v", err)
+\t}
+\tif value != 5 {
+\t\tt.Fatalf("IncrBy returned %d, want 5", value)
+\t}
+}
+
+func TestClient_HashOps(t *testing.T) {
+\tclient := newTestClient(t)
+\tctx := context.Background()
+
+\tif err := client.HSet(ctx, "hash", "a", "1", "b", "2"); err != nil {
+\t\tt.Fatalf("HSet failed: %v", err)
+\t}
+
+\tvalue, err := client.HGet(ctx, "hash", "a")
+\tif err != nil {
+\t\tt.Fatalf("HGet failed: %v", err)
+\t}
+\tif value != "1" {
+\t\tt.Fatalf("HGet returned %q, want %q", value, "1")
+\t}
+
+\tall, err := client.HGetAll(ctx, "hash")
+\tif err != nil {
+\t\tt.Fatalf("HGetAll failed: %v", err)
+\t}
+\tif all["b"] != "2" {
+\t\tt.Fatalf("HGetAll returned %#v", all)
+\t}
+
+\tif err := client.HDel(ctx, "hash", "a"); err != nil {
+\t\tt.Fatalf("HDel failed: %v", err)
+\t}
+\t_, err = client.HGet(ctx, "hash", "a")
+\tif !IsNil(err) {
+\t\tt.Fatalf("expected redis.Nil after HDel, got %v", err)
+\t}
+}
+
+func TestClient_Eval(t *testing.T) {
+\tclient := newTestClient(t)
+\tctx := context.Background()
+
+\tresult, err := client.Eval(ctx, "return 42", nil)
+\tif err != nil {
+\t\tt.Fatalf("Eval failed: %v", err)
+\t}
+
+\tvalue, ok := result.(int64)
+\tif !ok || value != 42 {
+\t\tt.Fatalf("unexpected eval result: %#v", result)
+\t}
+}
+
+func TestIsNil(t *testing.T) {
+\tif !IsNil(redis.Nil) {
+\t\tt.Fatalf("expected redis.Nil to be recognized")
+\t}
+\tif IsNil(errors.New("not nil")) {
+\t\tt.Fatalf("expected non-nil error to return false")
+\t}
+}
```

### `internal/tenant/config_test.go`
```diff
diff --git a/internal/tenant/config_test.go b/internal/tenant/config_test.go
new file mode 100644
--- /dev/null
+++ b/internal/tenant/config_test.go
@@ -0,0 +1,60 @@
+package tenant
+
+import "testing"
+
+func TestTenantConfigGetProvider(t *testing.T) {
+\tcfg := TenantConfig{
+\t\tProviders: map[string]ProviderConfig{
+\t\t\t"openai": {Enabled: true, APIKey: "key", Model: "model"},
+\t\t\t"gemini": {Enabled: false},
+\t\t},
+\t}
+
+\tproviderCfg, ok := cfg.GetProvider("openai")
+\tif !ok || providerCfg.APIKey != "key" {
+\t\tt.Fatalf("expected enabled provider config")
+\t}
+
+\tif _, ok := cfg.GetProvider("gemini"); ok {
+\t\tt.Fatalf("expected disabled provider to return false")
+\t}
+}
+
+func TestTenantConfigDefaultProvider(t *testing.T) {
+\tcfg := TenantConfig{
+\t\tFailover: FailoverConfig{Enabled: true, Order: []string{"gemini", "openai"}},
+\t\tProviders: map[string]ProviderConfig{
+\t\t\t"openai": {Enabled: true, APIKey: "key", Model: "model"},
+\t\t\t"gemini": {Enabled: true, APIKey: "key2", Model: "model2"},
+\t\t},
+\t}
+
+\tname, _, ok := cfg.DefaultProvider()
+\tif !ok || name != "gemini" {
+\t\tt.Fatalf("expected failover order provider, got %q", name)
+\t}
+
+\tcfg = TenantConfig{
+\t\tProviders: map[string]ProviderConfig{
+\t\t\t"openai": {Enabled: true, APIKey: "key", Model: "model"},
+\t\t\t"gemini": {Enabled: false},
+\t\t},
+\t}
+
+\tname, _, ok = cfg.DefaultProvider()
+\tif !ok || name != "openai" {
+\t\tt.Fatalf("expected fallback provider, got %q", name)
+\t}
+}
```

### `internal/tenant/env_test.go`
```diff
diff --git a/internal/tenant/env_test.go b/internal/tenant/env_test.go
new file mode 100644
--- /dev/null
+++ b/internal/tenant/env_test.go
@@ -0,0 +1,63 @@
+package tenant
+
+import "testing"
+
+func TestLoadEnv_OverridesAndDefaults(t *testing.T) {
+\tt.Setenv("AIBOX_CONFIGS_DIR", "/tmp/configs")
+\tt.Setenv("AIBOX_GRPC_PORT", "6000")
+\tt.Setenv("AIBOX_HOST", "127.0.0.1")
+\tt.Setenv("AIBOX_TLS_ENABLED", "true")
+\tt.Setenv("AIBOX_TLS_CERT_FILE", "/tmp/cert.pem")
+\tt.Setenv("AIBOX_TLS_KEY_FILE", "/tmp/key.pem")
+\tt.Setenv("REDIS_ADDR", "redis:6379")
+\tt.Setenv("REDIS_PASSWORD", "pass")
+\tt.Setenv("REDIS_DB", "2")
+\tt.Setenv("AIBOX_LOG_LEVEL", "debug")
+\tt.Setenv("AIBOX_LOG_FORMAT", "text")
+\tt.Setenv("AIBOX_ADMIN_TOKEN", "token")
+
+\tcfg, err := loadEnv()
+\tif err != nil {
+\t\tt.Fatalf("loadEnv failed: %v", err)
+\t}
+
+\tif cfg.ConfigsDir != "/tmp/configs" || cfg.GRPCPort != 6000 || cfg.Host != "127.0.0.1" {
+\t\tt.Fatalf("unexpected env overrides: %#v", cfg)
+\t}
+\tif !cfg.TLSEnabled || cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
+\t\tt.Fatalf("unexpected TLS config: %#v", cfg)
+\t}
+\tif cfg.RedisAddr != "redis:6379" || cfg.RedisPassword != "pass" || cfg.RedisDB != 2 {
+\t\tt.Fatalf("unexpected redis config: %#v", cfg)
+\t}
+\tif cfg.LogLevel != "debug" || cfg.LogFormat != "text" || cfg.AdminToken != "token" {
+\t\tt.Fatalf("unexpected logging/admin config: %#v", cfg)
+\t}
+}
+
+func TestLoadEnv_InvalidPort(t *testing.T) {
+\tt.Setenv("AIBOX_GRPC_PORT", "not-a-number")
+\tif _, err := loadEnv(); err == nil {
+\t\tt.Fatalf("expected error for invalid port")
+\t}
+}
+
+func TestLoadEnv_TLSValidation(t *testing.T) {
+\tt.Setenv("AIBOX_TLS_ENABLED", "true")
+\tif _, err := loadEnv(); err == nil {
+\t\tt.Fatalf("expected TLS validation error")
+\t}
+}
```

### `internal/tenant/secrets_test.go`
```diff
diff --git a/internal/tenant/secrets_test.go b/internal/tenant/secrets_test.go
new file mode 100644
--- /dev/null
+++ b/internal/tenant/secrets_test.go
@@ -0,0 +1,78 @@
+package tenant
+
+import (
+\t"os"
+\t"path/filepath"
+\t"testing"
+)
+
+func TestLoadSecret(t *testing.T) {
+\tt.Setenv("TEST_SECRET", "value")
+
+\tgot, err := loadSecret("ENV=TEST_SECRET")
+\tif err != nil || got != "value" {
+\t\tt.Fatalf("ENV= loadSecret failed: %v, %q", err, got)
+\t}
+
+\tpath := filepath.Join(t.TempDir(), "secret.txt")
+\tif err := os.WriteFile(path, []byte(" filevalue \n"), 0o600); err != nil {
+\t\tt.Fatalf("write file: %v", err)
+\t}
+\tgot, err = loadSecret("FILE=" + path)
+\tif err != nil || got != "filevalue" {
+\t\tt.Fatalf("FILE= loadSecret failed: %v, %q", err, got)
+\t}
+
+\tgot, err = loadSecret("${TEST_SECRET}")
+\tif err != nil || got != "value" {
+\t\tt.Fatalf("${} loadSecret failed: %v, %q", err, got)
+\t}
+
+\tgot, err = loadSecret("inline")
+\tif err != nil || got != "inline" {
+\t\tt.Fatalf("inline loadSecret failed: %v, %q", err, got)
+\t}
+
+\tif _, err := loadSecret("ENV=MISSING"); err == nil {
+\t\tt.Fatalf("expected missing env error")
+\t}
+}
+
+func TestResolveSecrets(t *testing.T) {
+\tt.Setenv("API_KEY", "resolved")
+
+\tcfg := TenantConfig{
+\t\tProviders: map[string]ProviderConfig{
+\t\t\t"openai": {Enabled: true, APIKey: "ENV=API_KEY", Model: "model"},
+\t\t},
+\t}
+
+\tif err := resolveSecrets(&cfg); err != nil {
+\t\tt.Fatalf("resolveSecrets failed: %v", err)
+\t}
+\tif cfg.Providers["openai"].APIKey != "resolved" {
+\t\tt.Fatalf("expected resolved api key, got %q", cfg.Providers["openai"].APIKey)
+\t}
+}
```

### `internal/tenant/loader_test.go`
```diff
diff --git a/internal/tenant/loader_test.go b/internal/tenant/loader_test.go
new file mode 100644
--- /dev/null
+++ b/internal/tenant/loader_test.go
@@ -0,0 +1,176 @@
+package tenant
+
+import (
+\t"os"
+\t"path/filepath"
+\t"strings"
+\t"testing"
+)
+
+func floatPtr(v float64) *float64 {
+\treturn &v
+}
+
+func intPtr(v int) *int {
+\treturn &v
+}
+
+func TestValidateTenantConfig(t *testing.T) {
+\tbase := TenantConfig{
+\t\tTenantID: "tenant",
+\t\tProviders: map[string]ProviderConfig{
+\t\t\t"openai": {Enabled: true, APIKey: "key", Model: "model"},
+\t\t},
+\t}
+
+\ttests := []struct {
+\t\tname    string
+\t\tmutate  func(*TenantConfig)
+\t\twantErr bool
+\t}{
+\t\t{"missing tenant id", func(c *TenantConfig) { c.TenantID = "" }, true},
+\t\t{"long tenant id", func(c *TenantConfig) { c.TenantID = strings.Repeat("x", 65) }, true},
+\t\t{"no enabled provider", func(c *TenantConfig) { c.Providers["openai"] = ProviderConfig{Enabled: false} }, true},
+\t\t{"missing api key", func(c *TenantConfig) { c.Providers["openai"] = ProviderConfig{Enabled: true, Model: "model"} }, true},
+\t\t{"missing model", func(c *TenantConfig) { c.Providers["openai"] = ProviderConfig{Enabled: true, APIKey: "key"} }, true},
+\t\t{"temperature out of range", func(c *TenantConfig) { p := c.Providers["openai"]; p.Temperature = floatPtr(3); c.Providers["openai"] = p }, true},
+\t\t{"top_p out of range", func(c *TenantConfig) { p := c.Providers["openai"]; p.TopP = floatPtr(2); c.Providers["openai"] = p }, true},
+\t\t{"max_output_tokens out of range", func(c *TenantConfig) { p := c.Providers["openai"]; p.MaxOutputTokens = intPtr(0); c.Providers["openai"] = p }, true},
+\t\t{"invalid failover", func(c *TenantConfig) { c.Failover = FailoverConfig{Enabled: true, Order: []string{"missing"}} }, true},
+\t\t{"valid config", func(c *TenantConfig) {}, false},
+\t}
+
+\tfor _, tt := range tests {
+\t\tt.Run(tt.name, func(t *testing.T) {
+\t\t\tcfg := base
+\t\t\t// Copy providers map to avoid mutation across tests
+\t\t\tcfg.Providers = map[string]ProviderConfig{"openai": base.Providers["openai"]}
+\t\t\ttt.mutate(&cfg)
+\t\t\terr := validateTenantConfig(&cfg)
+\t\t\tif tt.wantErr && err == nil {
+\t\t\t\tt.Fatalf("expected error, got nil")
+\t\t\t}
+\t\t\tif !tt.wantErr && err != nil {
+\t\t\t\tt.Fatalf("unexpected error: %v", err)
+\t\t\t}
+\t\t})
+\t}
+}
+
+func TestLoadTenants(t *testing.T) {
+\tdir := t.TempDir()
+\tt.Setenv("TEST_API_KEY", "env-value")
+
+\tjsonCfg := `{"tenant_id":"t1","providers":{"openai":{"enabled":true,"api_key":"ENV=TEST_API_KEY","model":"gpt-4o"}}}`
+\tif err := os.WriteFile(filepath.Join(dir, "tenant1.json"), []byte(jsonCfg), 0o600); err != nil {
+\t\tt.Fatalf("write json config: %v", err)
+\t}
+
+\tyamlCfg := "tenant_id: t2\nproviders:\n  openai:\n    enabled: true\n    api_key: inline\n    model: gpt-4o\n"
+\tif err := os.WriteFile(filepath.Join(dir, "tenant2.yaml"), []byte(yamlCfg), 0o600); err != nil {
+\t\tt.Fatalf("write yaml config: %v", err)
+\t}
+
+\tif err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("skip"), 0o600); err != nil {
+\t\tt.Fatalf("write notes: %v", err)
+\t}
+
+\tmissingID := "providers:\n  openai:\n    enabled: true\n    api_key: inline\n    model: gpt-4o\n"
+\tif err := os.WriteFile(filepath.Join(dir, "missing.yaml"), []byte(missingID), 0o600); err != nil {
+\t\tt.Fatalf("write missing id: %v", err)
+\t}
+
+\tconfigs, err := loadTenants(dir)
+\tif err != nil {
+\t\tt.Fatalf("loadTenants failed: %v", err)
+\t}
+\tif len(configs) != 2 {
+\t\tt.Fatalf("expected 2 tenants, got %d", len(configs))
+\t}
+\tif configs["t1"].Providers["openai"].APIKey != "env-value" {
+\t\tt.Fatalf("expected env-resolved API key")
+\t}
+}
+
+func TestLoadTenants_DuplicateTenantID(t *testing.T) {
+\t\tdir := t.TempDir()
+
+\tjsonCfg := `{"tenant_id":"dup","providers":{"openai":{"enabled":true,"api_key":"key","model":"gpt-4o"}}}`
+\tif err := os.WriteFile(filepath.Join(dir, "tenant1.json"), []byte(jsonCfg), 0o600); err != nil {
+\t\tt.Fatalf("write json config: %v", err)
+\t}
+\tif err := os.WriteFile(filepath.Join(dir, "tenant2.json"), []byte(jsonCfg), 0o600); err != nil {
+\t\tt.Fatalf("write json config: %v", err)
+\t}
+
+\tif _, err := loadTenants(dir); err == nil {
+\t\tt.Fatalf("expected duplicate tenant_id error")
+\t}
+}
```

### `internal/tenant/manager_test.go`
```diff
diff --git a/internal/tenant/manager_test.go b/internal/tenant/manager_test.go
new file mode 100644
--- /dev/null
+++ b/internal/tenant/manager_test.go
@@ -0,0 +1,115 @@
+package tenant
+
+import (
+\t"encoding/json"
+\t"os"
+\t"path/filepath"
+\t"reflect"
+\t"testing"
+)
+
+func writeTenantJSON(t *testing.T, dir, filename, tenantID string) {
+\tt.Helper()
+\tcfg := TenantConfig{
+\t\tTenantID: tenantID,
+\t\tProviders: map[string]ProviderConfig{
+\t\t\t"openai": {Enabled: true, APIKey: "key", Model: "model"},
+\t\t},
+\t}
+\tdata, err := json.Marshal(cfg)
+\tif err != nil {
+\t\tt.Fatalf("marshal config: %v", err)
+\t}
+\tif err := os.WriteFile(filepath.Join(dir, filename), data, 0o600); err != nil {
+\t\tt.Fatalf("write config: %v", err)
+\t}
+}
+
+func TestManagerTenantCodesAndDefault(t *testing.T) {
+\tmgr := &Manager{
+\t\tTenants: map[string]TenantConfig{
+\t\t\t"b": {TenantID: "b"},
+\t\t\t"a": {TenantID: "a"},
+\t\t},
+\t}
+
+\tcodes := mgr.TenantCodes()
+\tif !reflect.DeepEqual(codes, []string{"a", "b"}) {
+\t\tt.Fatalf("unexpected tenant codes: %#v", codes)
+\t}
+
+\tdef, ok := mgr.DefaultTenant()
+\tif !ok || def.TenantID != "a" {
+\t\tt.Fatalf("unexpected default tenant: %#v", def)
+\t}
+}
+
+func TestManagerReloadDiff(t *testing.T) {
+\tdir := t.TempDir()
+\twriteTenantJSON(t, dir, "t1.json", "t1")
+\twriteTenantJSON(t, dir, "t2.json", "t2")
+
+\tinitial, err := loadTenants(dir)
+\tif err != nil {
+\t\tt.Fatalf("loadTenants failed: %v", err)
+\t}
+
+\tmgr := &Manager{Env: EnvConfig{ConfigsDir: dir}, Tenants: initial}
+
+\tif err := os.Remove(filepath.Join(dir, "t1.json")); err != nil {
+\t\tt.Fatalf("remove t1: %v", err)
+\t}
+\twriteTenantJSON(t, dir, "t3.json", "t3")
+
+\tdiff, err := mgr.Reload()
+\tif err != nil {
+\t\tt.Fatalf("reload failed: %v", err)
+\t}
+
+\tif !reflect.DeepEqual(diff.Added, []string{"t3"}) {
+\t\tt.Fatalf("unexpected added: %#v", diff.Added)
+\t}
+\tif !reflect.DeepEqual(diff.Removed, []string{"t1"}) {
+\t\tt.Fatalf("unexpected removed: %#v", diff.Removed)
+\t}
+\tif !reflect.DeepEqual(diff.Unchanged, []string{"t2"}) {
+\t\tt.Fatalf("unexpected unchanged: %#v", diff.Unchanged)
+\t}
+}
```

### `internal/rag/testutil/mocks_test.go`
```diff
diff --git a/internal/rag/testutil/mocks_test.go b/internal/rag/testutil/mocks_test.go
new file mode 100644
--- /dev/null
+++ b/internal/rag/testutil/mocks_test.go
@@ -0,0 +1,190 @@
+package testutil
+
+import (
+\t"bytes"
+\t"context"
+\t"testing"
+
+\t"github.com/cliffpyles/aibox/internal/rag/vectorstore"
+)
+
+func TestMockEmbedder_Default(t *testing.T) {
+\tctx := context.Background()
+\temb := NewMockEmbedder(3)
+
+\tvec, err := emb.Embed(ctx, "hello")
+\tif err != nil {
+\t\tt.Fatalf("Embed failed: %v", err)
+\t}
+\tif len(vec) != 3 {
+\t\tt.Fatalf("expected 3-dim embedding, got %d", len(vec))
+\t}
+\tif len(emb.EmbedCalls) != 1 || emb.EmbedCalls[0] != "hello" {
+\t\tt.Fatalf("unexpected embed calls: %#v", emb.EmbedCalls)
+\t}
+
+\tbatch, err := emb.EmbedBatch(ctx, []string{"a", "b"})
+\tif err != nil {
+\t\tt.Fatalf("EmbedBatch failed: %v", err)
+\t}
+\tif len(batch) != 2 {
+\t\tt.Fatalf("expected 2 embeddings, got %d", len(batch))
+\t}
+\tif len(emb.EmbedBatchCalls) != 1 {
+\t\tt.Fatalf("unexpected batch calls: %#v", emb.EmbedBatchCalls)
+\t}
+
+\temb.Reset()
+\tif len(emb.EmbedCalls) != 0 || len(emb.EmbedBatchCalls) != 0 {
+\t\tt.Fatalf("expected calls to reset")
+\t}
+}
+
+func TestMockStore_BasicFlow(t *testing.T) {
+\tctx := context.Background()
+\tstore := NewMockStore()
+
+\tif err := store.CreateCollection(ctx, "c1", 2); err != nil {
+\t\tt.Fatalf("CreateCollection failed: %v", err)
+\t}
+
+\tpoints := []vectorstore.Point{
+\t\t{ID: "p1", Vector: []float32{0.1, 0.2}, Payload: map[string]any{"k": "v"}},
+\t}
+\tif err := store.Upsert(ctx, "c1", points); err != nil {
+\t\tt.Fatalf("Upsert failed: %v", err)
+\t}
+
+\tinfo, err := store.CollectionInfo(ctx, "c1")
+\tif err != nil {
+\t\tt.Fatalf("CollectionInfo failed: %v", err)
+\t}
+\tif info.PointCount != 1 || info.Dimensions != 2 {
+\t\tt.Fatalf("unexpected collection info: %#v", info)
+\t}
+
+\tresults, err := store.Search(ctx, vectorstore.SearchParams{Collection: "c1", Limit: 1})
+\tif err != nil {
+\t\tt.Fatalf("Search failed: %v", err)
+\t}
+\tif len(results) != 1 || results[0].ID != "p1" {
+\t\tt.Fatalf("unexpected search results: %#v", results)
+\t}
+
+\tif err := store.Delete(ctx, "c1", []string{"p1"}); err != nil {
+\t\tt.Fatalf("Delete failed: %v", err)
+\t}
+\tif len(store.GetPoints("c1")) != 0 {
+\t\tt.Fatalf("expected points to be deleted")
+\t}
+}
+
+func TestMockExtractor_Default(t *testing.T) {
+\tctx := context.Background()
+\textractor := NewMockExtractor()
+
+\tres, err := extractor.Extract(ctx, bytes.NewBufferString("content"), "file.txt", "text/plain")
+\tif err != nil {
+\t\tt.Fatalf("Extract failed: %v", err)
+\t}
+\tif res.Text != extractor.DefaultText || res.PageCount != 1 {
+\t\tt.Fatalf("unexpected extraction result: %#v", res)
+\t}
+
+\tif len(extractor.ExtractCalls) != 1 {
+\t\tt.Fatalf("expected 1 extract call, got %d", len(extractor.ExtractCalls))
+\t}
+\tif extractor.ExtractCalls[0].Filename != "file.txt" {
+\t\tt.Fatalf("unexpected extract call: %#v", extractor.ExtractCalls[0])
+\t}
+
+\tformats := extractor.SupportedFormats()
+\tif len(formats) == 0 {
+\t\tt.Fatalf("expected supported formats")
+\t}
+
+\textractor.Reset()
+\tif len(extractor.ExtractCalls) != 0 {
+\t\tt.Fatalf("expected extract calls to reset")
+\t}
+}
+
+func TestHelpers(t *testing.T) {
+\tvec := RandomEmbedding(4)
+\tif len(vec) != 4 {
+\t\tt.Fatalf("expected embedding length 4, got %d", len(vec))
+\t}
+
+\ttext := SampleText(25)
+\tif len(text) != 25 {
+\t\tt.Fatalf("expected text length 25, got %d", len(text))
+\t}
+}
```

### `go.mod`
```diff
diff --git a/go.mod b/go.mod
--- a/go.mod
+++ b/go.mod
@@ -3,6 +3,7 @@ module github.com/cliffpyles/aibox
 go 1.25.5
 
 require (
+\tgithub.com/alicebob/miniredis/v2 v2.35.0
 \tgithub.com/anthropics/anthropic-sdk-go v1.19.0
 \tgithub.com/openai/openai-go v1.12.0
 \tgithub.com/redis/go-redis/v9 v9.17.2
```

### `go.sum`
```diff
diff --git a/go.sum b/go.sum
--- a/go.sum
+++ b/go.sum
@@ -1,3 +1,7 @@
+github.com/alicebob/miniredis/v2 v2.35.0 h1:QwLphYqCEAo1eu1TqPRN2jgVMPBweeQcR21jeqDCONI=
+github.com/alicebob/miniredis/v2 v2.35.0/go.mod h1:TcL7YfarKPGDAthEtl5NBeHZfeUQj6OXMm/+iu5cLMM=
+github.com/yuin/gopher-lua v1.1.1 h1:kYKnWBjvbNP4XLT3+bPEwAXJx262OhaHDWDVOPjL46M=
+github.com/yuin/gopher-lua v1.1.1/go.mod h1:GBR0iDaNXjAgGg9zfCvksxSRnQx76gclCIb7kdAd1Pw=
```
