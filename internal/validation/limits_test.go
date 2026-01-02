package validation

import (
	"strings"
	"testing"
)

func TestValidateGenerateRequest_RejectsOversizedInput(t *testing.T) {
	tests := []struct {
		name        string
		userInput   string
		instruction string
		historyLen  int
		wantErr     bool
	}{
		{
			name:      "normal input passes",
			userInput: "Hello, how are you?",
			wantErr:   false,
		},
		{
			name:      "oversized user input rejected",
			userInput: strings.Repeat("x", MaxUserInputBytes+1),
			wantErr:   true,
		},
		{
			name:        "oversized instructions rejected",
			userInput:   "test",
			instruction: strings.Repeat("x", MaxInstructionsBytes+1),
			wantErr:     true,
		},
		{
			name:       "too many history items rejected",
			userInput:  "test",
			historyLen: MaxHistoryCount + 1,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGenerateRequest(tt.userInput, tt.instruction, tt.historyLen)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGenerateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
