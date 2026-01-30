package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	internalfs "scanr/internal/fs"
	"scanr/internal/review"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// AnthropicReviewer implements AI reviewer using Anthropic's Claude API
type AnthropicReviewer struct {
	config      AIConfig
	client      *http.Client
	rateLimiter *RateLimiter
	usage       UsageStats
	mu          sync.RWMutex
}

// NewAnthropicReviewer creates a new Anthropic API reviewer
func NewAnthropicReviewer(config AIConfig) (*AnthropicReviewer, error) {
	if config.Provider != "anthropic" {
		return nil, fmt.Errorf("provider must be 'anthropic' for AnthropicReviewer")
	}

	// Set default model if not specified
	if config.Model == "" {
		config.Model = "claude-3-sonnet-20240229"
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: config.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  false,
			MaxIdleConnsPerHost: 10,
		},
	}

	// Create rate limiter
	rateLimiter := NewRateLimiter(
		config.RateLimit.RequestsPerMinute,
		config.RateLimit.Burst,
		config.RateLimit.WaitTime,
	)

	return &AnthropicReviewer{
		config:      config,
		client:      client,
		rateLimiter: rateLimiter,
		usage:       UsageStats{},
	}, nil
}

// ReviewFile reviews a single file using Anthropic API
func (a *AnthropicReviewer) ReviewFile(ctx context.Context, file *internalfs.FileInfo) ([]review.Issue, error) {
	startTime := time.Now()

	// Read file content
	content, err := a.readFileContent(file.Path)
	if err != nil {
		a.recordFailure()
		return nil, fmt.Errorf("failed to read file %s: %w", file.Path, err)
	}

	// Build review request
	req := ReviewRequest{
		Relative:   file.Relative,
		Content:    content,
		Language:   file.Languages,
		Guidelines: GetLanguageGuidelines(file.Languages),
		MaxIssues:  10,
	}

	// Build prompt
	prompt, err := BuildPrompt(req)
	if err != nil {
		a.recordFailure()
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}

	// Make API request with retries
	var issues []review.Issue
	var lastErr error

	for attempt := 0; attempt <= a.config.MaxRetries; attempt++ {
		// Wait for rate limiter
		if err := a.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}

		// Make API request
		issues, lastErr = a.makeAPIRequest(ctx, prompt, file)
		if lastErr == nil {
			break // Success
		}

		// Check if we should retry
		if attempt < a.config.MaxRetries && a.shouldRetry(lastErr) {
			a.recordRetry()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(a.config.RateLimit.WaitTime * time.Duration(attempt+1)):
				// Exponential backoff
				continue
			}
		}
	}

	if lastErr != nil {
		a.recordFailure()
		return nil, fmt.Errorf("failed to review file after %d attempts: %w",
			a.config.MaxRetries+1, lastErr)
	}

	// Record success
	duration := time.Since(startTime)
	a.recordSuccess(duration, len(prompt), 0)

	return issues, nil
}

// Name returns the reviewer name
func (a *AnthropicReviewer) Name() string {
	return fmt.Sprintf("anthropic-%s", a.config.Model)
}

// ValidateConfig validates the Anthropic configuration
func (a *AnthropicReviewer) ValidateConfig() error {
	if a.config.APIKey == "" {
		return fmt.Errorf("Anthropic API key is required")
	}

	// Test API connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a minimal test request
	payload := map[string]interface{}{
		"model":      a.config.Model,
		"max_tokens": 10,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "test",
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal test request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Anthropic API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Anthropic API test failed with status %d: %s",
			resp.StatusCode, string(body))
	}

	return nil
}

// GetUsage returns usage statistics
func (a *AnthropicReviewer) GetUsage() UsageStats {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.usage
}

// makeAPIRequest makes the actual Anthropic API request
func (a *AnthropicReviewer) makeAPIRequest(ctx context.Context, prompt string, file *internalfs.FileInfo) ([]review.Issue, error) {
	// Prepare request payload
	payload := map[string]interface{}{
		"model":       a.config.Model,
		"max_tokens":  a.config.MaxTokens,
		"temperature": a.config.Temperature,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"system": "You are an expert code reviewer. Respond with a JSON array of issues. Return only valid JSON without any markdown formatting.",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Send request
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for API errors
	if resp.StatusCode != http.StatusOK {
		var apiErr map[string]interface{}
		if err := json.Unmarshal(respBody, &apiErr); err == nil {
			if errorMsg, ok := apiErr["error"].(map[string]interface{}); ok {
				if message, ok := errorMsg["message"].(string); ok {
					return nil, fmt.Errorf("Anthropic API error: %s", message)
				}
			}
		}
		return nil, fmt.Errorf("Anthropic API returned status %d: %s",
			resp.StatusCode, string(respBody))
	}

	// Parse response
	return a.parseAPIResponse(respBody, file)
}

// parseAPIResponse parses the Anthropic API response
func (a *AnthropicReviewer) parseAPIResponse(response []byte, file *internalfs.FileInfo) ([]review.Issue, error) {
	var apiResponse struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(response, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal API response: %w", err)
	}

	if len(apiResponse.Content) == 0 {
		return nil, fmt.Errorf("no content in API response")
	}

	// Extract text from response
	text := apiResponse.Content[0].Text

	// Clean the response text
	text = a.cleanResponseText(text)

	// Parse JSON issues from response
	var issues []review.Issue
	if text != "" && text != "[]" {
		if err := json.Unmarshal([]byte(text), &issues); err != nil {
			// Try to extract JSON from malformed response
			issues = a.extractIssuesFromText(text, file)
		}
	}

	// Convert to review.Issue format and add file path
	for i := range issues {
		issues[i].FilePath = file.Path
		if issues[i].FoundAt.IsZero() {
			issues[i].FoundAt = time.Now()
		}
	}

	// Record token usage
	a.mu.Lock()
	a.usage.PromptTokens += int64(apiResponse.Usage.InputTokens)
	a.usage.CompletionTokens += int64(apiResponse.Usage.OutputTokens)
	a.usage.TotalTokens += int64(apiResponse.Usage.InputTokens + apiResponse.Usage.OutputTokens)
	a.mu.Unlock()

	return issues, nil
}

// cleanResponseText cleans the API response text
func (a *AnthropicReviewer) cleanResponseText(text string) string {
	// Remove markdown code blocks
	text = strings.TrimSpace(text)
	if after, ok := strings.CutPrefix(text, "```json"); ok {
		text = after
	}
	if after, ok := strings.CutPrefix(text, "```"); ok {
		text = after
	}
	if before, ok := strings.CutSuffix(text, "```"); ok {
		text = before
	}

	// Remove any explanatory text before or after JSON
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")

	if start != -1 && end != -1 && end > start {
		text = text[start : end+1]
	}

	return strings.TrimSpace(text)
}

// extractIssuesFromText attempts to extract issues from malformed JSON response
func (a *AnthropicReviewer) extractIssuesFromText(text string, file *internalfs.FileInfo) []review.Issue {
	var issues []review.Issue

	// Simple heuristic: look for issue-like structures
	lines := strings.Split(text, "\n")
	var currentIssue *review.Issue

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for JSON-like structures
		if strings.Contains(line, "\"title\":") {
			if currentIssue != nil {
				issues = append(issues, *currentIssue)
			}
			currentIssue = &review.Issue{
				FilePath: file.Path,
				FoundAt:  time.Now(),
			}

			// Extract title
			if start := strings.Index(line, "\"title\":\""); start != -1 {
				start += 9
				if end := strings.Index(line[start:], "\""); end != -1 {
					currentIssue.Title = line[start : start+end]
				}
			}
		} else if currentIssue != nil {
			// Extract other fields similarly
			if strings.Contains(line, "\"severity\":") {
				if start := strings.Index(line, "\"severity\":\""); start != -1 {
					start += 12
					if end := strings.Index(line[start:], "\""); end != -1 {
						currentIssue.Severity = review.Severity(line[start : start+end])
					}
				}
			} else if strings.Contains(line, "\"description\":") {
				if start := strings.Index(line, "\"description\":\""); start != -1 {
					start += 15
					if end := strings.Index(line[start:], "\""); end != -1 {
						currentIssue.Description = line[start : start+end]
					}
				}
			} else if strings.Contains(line, "\"line\":") {
				if start := strings.Index(line, "\"line\":"); start != -1 {
					start += 7
					var lineNum int
					fmt.Sscanf(line[start:], "%d", &lineNum)
					currentIssue.Line = lineNum
				}
			}
		}
	}

	if currentIssue != nil {
		issues = append(issues, *currentIssue)
	}

	return issues
}

// readFileContent reads file content with size limits
func (a *AnthropicReviewer) readFileContent(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// Limit file size for AI processing
	maxSize := 64 * 1024 // 64KB max for AI review
	if len(content) > maxSize {
		content = content[:maxSize]
	}

	return string(content), nil
}

// shouldRetry determines if an error is retryable
func (a *AnthropicReviewer) shouldRetry(err error) bool {
	errStr := err.Error()

	// Retry on network errors, rate limits, and temporary failures
	retryableErrors := []string{
		"timeout",
		"deadline exceeded",
		"network",
		"rate limit",
		"too many requests",
		"429",
		"503",
		"504",
		"temporary",
		"unavailable",
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(strings.ToLower(errStr), retryable) {
			return true
		}
	}

	return false
}

// recordSuccess records successful API call
func (a *AnthropicReviewer) recordSuccess(duration time.Duration, promptTokens, completionTokens int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.usage.TotalRequests++
	a.usage.Successful++
	a.usage.TotalDuration += duration
	a.usage.PromptTokens += int64(promptTokens)
	a.usage.CompletionTokens += int64(completionTokens)
	a.usage.TotalTokens += int64(promptTokens + completionTokens)

	// Calculate cost (approximate)
	// Claude 3 Sonnet pricing: $0.003 per 1K input, $0.015 per 1K output
	inputCost := float64(promptTokens) * 0.003 / 1000
	outputCost := float64(completionTokens) * 0.015 / 1000
	a.usage.TotalCost += inputCost + outputCost
}

// recordFailure records failed API call
func (a *AnthropicReviewer) recordFailure() {
	atomic.AddInt64(&a.usage.TotalRequests, 1)
	atomic.AddInt64(&a.usage.Failed, 1)
}

// recordRetry records retry attempt
func (a *AnthropicReviewer) recordRetry() {
	atomic.AddInt64(&a.usage.Retried, 1)
}
