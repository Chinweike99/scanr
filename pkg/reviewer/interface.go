package reviewer

import (
	"fmt"
	"scanr/internal/review"
	"time"
)

// AIConfig - Holds configuration for AI reviewer
type AIConfig struct {
	Provider    string        `json:"provider"`
	Model       string        `json:"model"`
	APIKey      string        `json:"-"`
	BaseURL     string        `json:"base_url,omitempty"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
	Timeout     time.Duration `json:"timeout"`
	MaxRetries  int           `json:"max_retries"`
	RateLimit   RateLimit     `json:"rate_limit"`
}

type RateLimit struct {
	RequestsPerMinute int           `json:"requests_per_minute"`
	Burst             int           `json:"burst"`
	WaitTime          time.Duration `json:"wait_time"`
}

func DefaultAIConfig() AIConfig {
	return AIConfig{
		Provider:    "gemini",
		Model:       "gemini-2.5-pro",
		MaxTokens:   4096,
		Temperature: 0.1,
		Timeout:     60 * time.Second,
		MaxRetries:  3,
		RateLimit: RateLimit{
			RequestsPerMinute: 10,
			Burst:             2,
			WaitTime:          5 * time.Second,
		},
	}
}

type AIReviewer interface {
	review.Reviewer
	ValidateConfig() error
	GetUsage() UsageStats
}

type UsageStats struct {
	TotalRequests    int64         `json:"total_requests"`
	Successful       int64         `json:"successful"`
	Failed           int64         `json:"failed"`
	Retried          int64         `json:"retried"`
	TotalTokens      int64         `json:"total_tokens"`
	PromptTokens     int64         `json:"prompt_tokens"`
	CompletionTokens int64         `json:"completion_tokens"`
	TotalCost        float64       `json:"total_cost"`
	TotalDuration    time.Duration `json:"total_duration"`
}

// NewAIReviewer creates a new AI reviewer based on configuration
func NewAIReviewer(config AIConfig) (AIReviewer, error) {
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid AI configuration: %w", err)
	}

	switch config.Provider {
	case "gemini":
		return NewGeminiReviewer(config)
	case "openai":
		return NewOpenAIReviewer(config)
	case "anthropic":
		return NewAnthropicReviewer(config)
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", config.Provider)
	}
}

// validateConfig validates AI configuration
func validateConfig(config AIConfig) error {
	if config.APIKey == "" {
		return fmt.Errorf("API key is required")
	}

	if config.Model == "" {
		return fmt.Errorf("model name is required")
	}

	if config.MaxTokens <= 0 {
		return fmt.Errorf("max tokens must be positive")
	}

	if config.Temperature < 0 || config.Temperature > 1 {
		return fmt.Errorf("temperature must be between 0.0 and 1.0")
	}

	if config.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	if config.MaxRetries < 0 {
		return fmt.Errorf("max retries cannot be negative")
	}

	if config.RateLimit.RequestsPerMinute <= 0 {
		return fmt.Errorf("rate limit requests per minute must be positive")
	}

	return nil
}
