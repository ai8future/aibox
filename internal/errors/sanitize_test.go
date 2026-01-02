package errors

import (
	"errors"
	"testing"
)

func TestSanitizeForClient(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "generic error returns generic message",
			err:      errors.New("connection refused to api.openai.com:443"),
			expected: "provider temporarily unavailable",
		},
		{
			name:     "nil error returns empty",
			err:      nil,
			expected: "",
		},
		{
			name:     "api key error sanitized",
			err:      errors.New("invalid API key: sk-proj-xxxxx"),
			expected: "authentication failed with provider",
		},
		{
			name:     "rate limit preserved",
			err:      errors.New("rate limit exceeded"),
			expected: "rate limit exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeForClient(tt.err)
			if result != tt.expected {
				t.Errorf("SanitizeForClient() = %q, want %q", result, tt.expected)
			}
		})
	}
}
