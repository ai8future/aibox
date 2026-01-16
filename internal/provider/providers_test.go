package provider_test

import (
	"testing"

	"github.com/ai8future/airborne/internal/provider"
	"github.com/ai8future/airborne/internal/provider/cerebras"
	"github.com/ai8future/airborne/internal/provider/cohere"
	"github.com/ai8future/airborne/internal/provider/deepinfra"
	"github.com/ai8future/airborne/internal/provider/deepseek"
	"github.com/ai8future/airborne/internal/provider/fireworks"
	"github.com/ai8future/airborne/internal/provider/grok"
	"github.com/ai8future/airborne/internal/provider/hyperbolic"
	"github.com/ai8future/airborne/internal/provider/mistral"
	"github.com/ai8future/airborne/internal/provider/nebius"
	"github.com/ai8future/airborne/internal/provider/openrouter"
	"github.com/ai8future/airborne/internal/provider/perplexity"
	"github.com/ai8future/airborne/internal/provider/together"
	"github.com/ai8future/airborne/internal/provider/upstage"
)

// providerCase defines expected capabilities for each provider.
type providerCase struct {
	name               string
	constructor        func() provider.Provider
	supportsFileSearch bool
	supportsWebSearch  bool
	supportsStreaming  bool
	supportsContinuity bool
}

// TestCompatProviderCapabilities verifies that all OpenAI-compatible providers
// are correctly configured with expected capabilities.
func TestCompatProviderCapabilities(t *testing.T) {
	tests := []providerCase{
		{"cerebras", func() provider.Provider { return cerebras.NewClient() }, false, false, true, false},
		{"cohere", func() provider.Provider { return cohere.NewClient() }, false, true, true, false},
		{"deepinfra", func() provider.Provider { return deepinfra.NewClient() }, false, false, true, false},
		{"deepseek", func() provider.Provider { return deepseek.NewClient() }, false, false, true, false},
		{"fireworks", func() provider.Provider { return fireworks.NewClient() }, false, false, true, false},
		{"grok", func() provider.Provider { return grok.NewClient() }, false, false, true, false},
		{"hyperbolic", func() provider.Provider { return hyperbolic.NewClient() }, false, false, true, false},
		{"mistral", func() provider.Provider { return mistral.NewClient() }, false, false, true, false},
		{"nebius", func() provider.Provider { return nebius.NewClient() }, false, false, true, false},
		{"openrouter", func() provider.Provider { return openrouter.NewClient() }, false, false, true, false},
		{"perplexity", func() provider.Provider { return perplexity.NewClient() }, false, true, true, false},
		{"together", func() provider.Provider { return together.NewClient() }, false, false, true, false},
		{"upstage", func() provider.Provider { return upstage.NewClient() }, false, false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.constructor()

			if client == nil {
				t.Fatal("constructor returned nil")
			}

			if got := client.Name(); got != tt.name {
				t.Errorf("Name() = %q, want %q", got, tt.name)
			}

			if got := client.SupportsFileSearch(); got != tt.supportsFileSearch {
				t.Errorf("SupportsFileSearch() = %v, want %v", got, tt.supportsFileSearch)
			}

			if got := client.SupportsWebSearch(); got != tt.supportsWebSearch {
				t.Errorf("SupportsWebSearch() = %v, want %v", got, tt.supportsWebSearch)
			}

			if got := client.SupportsStreaming(); got != tt.supportsStreaming {
				t.Errorf("SupportsStreaming() = %v, want %v", got, tt.supportsStreaming)
			}

			if got := client.SupportsNativeContinuity(); got != tt.supportsContinuity {
				t.Errorf("SupportsNativeContinuity() = %v, want %v", got, tt.supportsContinuity)
			}
		})
	}
}

// TestCompatProviderImplementsInterface ensures all providers implement provider.Provider.
func TestCompatProviderImplementsInterface(t *testing.T) {
	// Compile-time interface compliance checks
	var _ provider.Provider = cerebras.NewClient()
	var _ provider.Provider = cohere.NewClient()
	var _ provider.Provider = deepinfra.NewClient()
	var _ provider.Provider = deepseek.NewClient()
	var _ provider.Provider = fireworks.NewClient()
	var _ provider.Provider = grok.NewClient()
	var _ provider.Provider = hyperbolic.NewClient()
	var _ provider.Provider = mistral.NewClient()
	var _ provider.Provider = nebius.NewClient()
	var _ provider.Provider = openrouter.NewClient()
	var _ provider.Provider = perplexity.NewClient()
	var _ provider.Provider = together.NewClient()
	var _ provider.Provider = upstage.NewClient()
}
