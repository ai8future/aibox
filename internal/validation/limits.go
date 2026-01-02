package validation

import (
	"errors"
	"fmt"
)

const (
	// MaxUserInputBytes is the maximum size of user input (100KB)
	MaxUserInputBytes = 100 * 1024

	// MaxInstructionsBytes is the maximum size of system instructions (50KB)
	MaxInstructionsBytes = 50 * 1024

	// MaxHistoryCount is the maximum number of conversation history messages
	MaxHistoryCount = 100

	// MaxMetadataEntries is the maximum number of metadata key-value pairs
	MaxMetadataEntries = 50
)

var (
	ErrUserInputTooLarge    = errors.New("user_input exceeds maximum size")
	ErrInstructionsTooLarge = errors.New("instructions exceed maximum size")
	ErrHistoryTooLong       = errors.New("conversation_history exceeds maximum length")
	ErrMetadataTooLarge     = errors.New("metadata exceeds maximum entries")
)

// ValidateGenerateRequest validates size limits for a generate request
func ValidateGenerateRequest(userInput, instructions string, historyCount int) error {
	if len(userInput) > MaxUserInputBytes {
		return fmt.Errorf("%w: %d bytes (max %d)", ErrUserInputTooLarge, len(userInput), MaxUserInputBytes)
	}

	if len(instructions) > MaxInstructionsBytes {
		return fmt.Errorf("%w: %d bytes (max %d)", ErrInstructionsTooLarge, len(instructions), MaxInstructionsBytes)
	}

	if historyCount > MaxHistoryCount {
		return fmt.Errorf("%w: %d messages (max %d)", ErrHistoryTooLong, historyCount, MaxHistoryCount)
	}

	return nil
}

// ValidateMetadata validates metadata size limits
func ValidateMetadata(metadata map[string]string) error {
	if len(metadata) > MaxMetadataEntries {
		return fmt.Errorf("%w: %d entries (max %d)", ErrMetadataTooLarge, len(metadata), MaxMetadataEntries)
	}
	return nil
}
