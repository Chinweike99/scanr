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

// OpenAIReviewer implements AI reviewer using OpenAI's API
type OpenAIReviewer struct {
	config      AIConfig
	client      *http.Client
	rateLimiter *RateLimiter
	usage       UsageStats
	mu          sync.RWMutex
}

// NewOpenAIReviewer creates a new OpenAI API reviewer
func NewOpenAIReviewer(config AIConfig) (*OpenAIReviewer, error) {
	if config.Provider != "openai" {
		return nil, fmt.Errorf("provider must be 'openai' for OpenAIReviewer")
	}

	// Set default model if not specified
	if config.Model == "" {
		config.Model = "gpt-4"
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

	return &OpenAIReviewer{
		config:      config,
		client:      client,
		rateLimiter: rateLimiter,
		usage:       UsageStats{},
	}, nil
}

// ReviewFile reviews a single file using OpenAI API
func (o *OpenAIReviewer) ReviewFile(ctx context.Context, file *internalfs.FileInfo) ([]review.Issue, error) {
	startTime := time.Now()

	// Read file content
	content, err := o.readFileContent(file.Path)
	if err != nil {
		o.recordFailure()
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
		o.recordFailure()
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}

	// Make API request with retries
	var issues []review.Issue
	var lastErr error

	for attempt := 0; attempt <= o.config.MaxRetries; attempt++ {
		// Wait for rate limiter
		if err := o.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}

		// Make API request
		issues, lastErr = o.makeAPIRequest(ctx, prompt, file)
		if lastErr == nil {
			break // Success
		}

		// Check if we should retry
		if attempt < o.config.MaxRetries && o.shouldRetry(lastErr) {
			o.recordRetry()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(o.config.RateLimit.WaitTime * time.Duration(attempt+1)):
				// Exponential backoff
				continue
			}
		}
	}

	if lastErr != nil {
		o.recordFailure()
		return nil, fmt.Errorf("failed to review file after %d attempts: %w",
			o.config.MaxRetries+1, lastErr)
	}

	// Record success
	duration := time.Since(startTime)
	o.recordSuccess(duration, len(prompt), 0)

	return issues, nil
}

// Name returns the reviewer name
func (o *OpenAIReviewer) Name() string {
	return fmt.Sprintf("openai-%s", o.config.Model)
}

// ValidateConfig validates the OpenAI configuration
func (o *OpenAIReviewer) ValidateConfig() error {
	if o.config.APIKey == "" {
		return fmt.Errorf("OpenAI API key is required")
	}

	// Test API connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testURL := "https://api.openai.com/v1/models"

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.config.APIKey))

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to OpenAI API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI API test failed with status %d: %s",
			resp.StatusCode, string(body))
	}

	return nil
}

// GetUsage returns usage statistics
func (o *OpenAIReviewer) GetUsage() UsageStats {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.usage
}

// makeAPIRequest makes the actual OpenAI API request
func (o *OpenAIReviewer) makeAPIRequest(ctx context.Context, prompt string, file *internalfs.FileInfo) ([]review.Issue, error) {
	// Prepare request payload
	payload := map[string]interface{}{
		"model": o.config.Model,
		"messages": []map[string]interface{}{
			{
				"role":    "system",
				"content": "You are an expert code reviewer. Respond with a JSON array of issues.",
			},
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature":       o.config.Temperature,
		"max_tokens":        o.config.MaxTokens,
		"top_p":             0.95,
		"frequency_penalty": 0,
		"presence_penalty":  0,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.config.APIKey))

	// Send request
	resp, err := o.client.Do(req)
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
					return nil, fmt.Errorf("OpenAI API error: %s", message)
				}
			}
		}
		return nil, fmt.Errorf("OpenAI API returned status %d: %s",
			resp.StatusCode, string(respBody))
	}

	// Parse response
	return o.parseAPIResponse(respBody, file)
}

// parseAPIResponse parses the OpenAI API response
func (o *OpenAIReviewer) parseAPIResponse(response []byte, file *internalfs.FileInfo) ([]review.Issue, error) {
	var apiResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(response, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal API response: %w", err)
	}

	if len(apiResponse.Choices) == 0 {
		return nil, fmt.Errorf("no choices in API response")
	}

	// Extract text from response
	text := apiResponse.Choices[0].Message.Content

	// Clean the response text
	text = o.cleanResponseText(text)

	// Parse JSON issues from response
	var issues []review.Issue
	if text != "" && text != "[]" {
		if err := json.Unmarshal([]byte(text), &issues); err != nil {
			// Try to extract JSON from malformed response
			issues = o.extractIssuesFromText(text, file)
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
	o.mu.Lock()
	o.usage.PromptTokens += int64(apiResponse.Usage.PromptTokens)
	o.usage.CompletionTokens += int64(apiResponse.Usage.CompletionTokens)
	o.usage.TotalTokens += int64(apiResponse.Usage.TotalTokens)
	o.mu.Unlock()

	return issues, nil
}

// cleanResponseText cleans the API response text
func (o *OpenAIReviewer) cleanResponseText(text string) string {
	// Remove markdown code blocks
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
	}
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
	}
	if strings.HasSuffix(text, "```") {
		text = strings.TrimSuffix(text, "```")
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
func (o *OpenAIReviewer) extractIssuesFromText(text string, file *internalfs.FileInfo) []review.Issue {
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
func (o *OpenAIReviewer) readFileContent(path string) (string, error) {
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
func (o *OpenAIReviewer) shouldRetry(err error) bool {
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
func (o *OpenAIReviewer) recordSuccess(duration time.Duration, promptTokens, completionTokens int) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.usage.TotalRequests++
	o.usage.Successful++
	o.usage.TotalDuration += duration
	o.usage.PromptTokens += int64(promptTokens)
	o.usage.CompletionTokens += int64(completionTokens)
	o.usage.TotalTokens += int64(promptTokens + completionTokens)

	// Calculate cost (approximate)
	// GPT-4 pricing example: $0.03 per 1K input, $0.06 per 1K output
	inputCost := float64(promptTokens) * 0.03 / 1000
	outputCost := float64(completionTokens) * 0.06 / 1000
	o.usage.TotalCost += inputCost + outputCost
}

// recordFailure records failed API call
func (o *OpenAIReviewer) recordFailure() {
	atomic.AddInt64(&o.usage.TotalRequests, 1)
	atomic.AddInt64(&o.usage.Failed, 1)
}

// recordRetry records retry attempt
func (o *OpenAIReviewer) recordRetry() {
	atomic.AddInt64(&o.usage.Retried, 1)
}
