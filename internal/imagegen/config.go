// Package imagegen provides AI image generation capabilities.
package imagegen

// Config contains image generation settings from project specs.
type Config struct {
	// Enabled controls whether image generation is active
	Enabled bool `json:"enabled"`

	// Provider specifies which image generation service to use ("gemini", "openai")
	Provider string `json:"provider"`

	// Model specifies the model to use (e.g., "gemini-2.5-flash-image", "dall-e-3")
	Model string `json:"model"`

	// TriggerPhrases are phrases that trigger image generation (e.g., "@image")
	TriggerPhrases []string `json:"trigger_phrases"`

	// FallbackOnError if true, continues with text-only response on image gen failure
	FallbackOnError bool `json:"fallback_on_error"`

	// MaxImages limits the number of images per response
	MaxImages int `json:"max_images"`
}

// IsEnabled returns true if image generation is configured and enabled.
func (c *Config) IsEnabled() bool {
	return c != nil && c.Enabled
}

// GetProvider returns the configured provider, defaulting to "gemini".
func (c *Config) GetProvider() string {
	if c == nil || c.Provider == "" {
		return "gemini"
	}
	return c.Provider
}

// GetModel returns the configured model, or empty string for provider default.
func (c *Config) GetModel() string {
	if c == nil {
		return ""
	}
	return c.Model
}
