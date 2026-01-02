package auth

import (
	"testing"
)

func TestRateLimiter_AtomicIncrement(t *testing.T) {
	t.Run("increment and expire are atomic", func(t *testing.T) {
		// The checkLimit function uses a Lua script that:
		// 1. Increments counter
		// 2. Sets TTL if new key
		// 3. Returns current count
		// All in a single atomic operation
		t.Skip("requires Redis test container - see integration tests")
	})
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	t.Run("keys expire after window", func(t *testing.T) {
		t.Skip("requires Redis test container - see integration tests")
	})
}
