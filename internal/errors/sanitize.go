package errors

import (
	"log/slog"
	"strings"
)

// clientSafePatterns maps error patterns to client-safe messages
var clientSafePatterns = map[string]string{
	"rate limit":   "rate limit exceeded",
	"quota":        "quota exceeded",
	"timeout":      "request timed out",
	"context dead": "request cancelled",
	"invalid api":  "authentication failed with provider",
	"unauthorized": "authentication failed with provider",
	"forbidden":    "access denied by provider",
	"not found":    "resource not found",
}

// SanitizeForClient converts internal errors to client-safe messages
// It logs the full error server-side and returns a sanitized version
func SanitizeForClient(err error) string {
	if err == nil {
		return ""
	}

	errLower := strings.ToLower(err.Error())

	// Check for known safe patterns
	for pattern, safeMsg := range clientSafePatterns {
		if strings.Contains(errLower, pattern) {
			slog.Debug("sanitizing error for client",
				"original", err.Error(),
				"sanitized", safeMsg,
			)
			return safeMsg
		}
	}

	// Log the full error, return generic message
	slog.Error("provider error (sanitized for client)", "error", err)
	return "provider temporarily unavailable"
}
