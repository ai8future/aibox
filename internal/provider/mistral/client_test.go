package mistral

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")

	client := NewClient()
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
}
