package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"scanr/pkg/reviewer"
	"strconv"
	"time"
)

// AIConfigFile is the name of the AI configuration file
const AIConfigFile = ".scanr-ai.yaml"

// LoadAIConfig loads AI configuration from file or environment
func LoadAIConfig() (reviewer.AIConfig, error) {
	config := reviewer.DefaultAIConfig()

	if cfg, err := loadAIConfigFromFile(); err == nil {
		config = cfg
	}

	LoadAIConfigFromEnv(&config)
	return config, nil
}

// loadAIConfigFromFile loads AI configuration from YAML file
func loadAIConfigFromFile() (reviewer.AIConfig, error) {
	var config reviewer.AIConfig

	// Try current directory first
	configPath := AIConfigFile
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Try home directory
		home, err := os.UserHomeDir()
		if err != nil {
			return config, fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(home, AIConfigFile)

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			return config, fmt.Errorf("AI config file not found")
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return config, fmt.Errorf("failed to read AI config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("failed to parse AI config file: %w", err)
	}

	return config, nil
}

func LoadAIConfigFromEnv(config *reviewer.AIConfig) {
	if provider := os.Getenv("SCANR_AI_PROVIDER"); provider != "" {
		config.Provider = provider
	}

	if model := os.Getenv("SCANR_AI_MODEL"); model != "" {
		config.Model = model
	}

	// API key
	if apiKey := os.Getenv("SCANR_AI_API_KEY"); apiKey != "" {
		config.APIKey = apiKey
	}

	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" && config.Provider == "gemini" {
		config.APIKey = apiKey
	}

	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" && config.Provider == "openai" {
		config.APIKey = apiKey
	}

	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" && config.Provider == "anthropic" {
		config.APIKey = apiKey
	}

	// Base URL for self-hosted or custom endpoints
	if baseURL := os.Getenv("SCANR_AI_BASE_URL"); baseURL != "" {
		config.BaseURL = baseURL
	}

	// Model parameters
	if maxTokens := os.Getenv("SCANR_AI_MAX_TOKENS"); maxTokens != "" {
		if val, err := strconv.Atoi(maxTokens); err == nil && val > 0 {
			config.MaxTokens = val
		}
	}

	if temp := os.Getenv("SCANR_AI_TEMPERATURE"); temp != "" {
		if val, err := strconv.ParseFloat(temp, 64); err == nil && val >= 0 && val <= 1 {
			config.Temperature = val
		}
	}

	// Timeout
	if timeout := os.Getenv("SCANR_AI_TIMEOUT"); timeout != "" {
		if val, err := time.ParseDuration(timeout); err == nil && val > 0 {
			config.Timeout = val
		}
	}

	// Retries
	if retries := os.Getenv("SCANR_AI_MAX_RETRIES"); retries != "" {
		if val, err := strconv.Atoi(retries); err == nil && val >= 0 {
			config.MaxRetries = val
		}
	}

	// Rate limiting
	if rpm := os.Getenv("SCANR_AI_RATE_LIMIT_RPM"); rpm != "" {
		if val, err := strconv.Atoi(rpm); err == nil && val > 0 {
			config.RateLimit.RequestsPerMinute = val
		}
	}

	if burst := os.Getenv("SCANR_AI_RATE_LIMIT_BURST"); burst != "" {
		if val, err := strconv.Atoi(burst); err == nil && val > 0 {
			config.RateLimit.Burst = val
		}
	}

	if waitTime := os.Getenv("SCANR_AI_RATE_LIMIT_WAIT"); waitTime != "" {
		if val, err := time.ParseDuration(waitTime); err == nil && val > 0 {
			config.RateLimit.WaitTime = val
		}
	}
}

// SaveAIConfig saves AI configuration to file
func SaveAIConfig(config reviewer.AIConfig, path string) error {
	// Don't save API key to file
	config.APIKey = ""

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal AI config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write AI config file: %w", err)
	}

	return nil
}

// CreateDefaultAIConfig creates a default AI config file
func CreateDefaultAIConfig() error {
	config := reviewer.DefaultAIConfig()
	config.APIKey = ""

	// Create template with instructions
	template := `# Scanr AI Configuration
		# Save this file as .scanr-ai.yaml in your home directory or project root

		provider: gemini
		model: gemini-2.5-pro
		# api_key: YOUR_API_KEY_HERE  # Set via SCANR_AI_API_KEY environment variable
		base_url: ""  # Optional custom endpoint

		# Model parameters
		max_tokens: 4096
		temperature: 0.1  # Low temperature for deterministic reviews

		# Request settings
		timeout: 60s
		max_retries: 3

		# Rate limiting (adjust based on your API tier)
		rate_limit:
		requests_per_minute: 10
		burst: 2
		wait_time: 5s

		# Environment variables override these settings
		# Required: SCANR_AI_API_KEY or provider-specific key (GEMINI_API_KEY, etc.)
		`

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(home, AIConfigFile)
	return os.WriteFile(configPath, []byte(template), 0644)
}
