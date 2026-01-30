package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	internalfs "scanr/internal/fs"
	"scanr/internal/review"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// GeminiReviewer implements AI reviewer using Google's Gemini API
type GeminiReviewer struct {
	config      AIConfig
	client      *http.Client
	rateLimiter *RateLimiter
	usage       UsageStats
	mu          sync.RWMutex
}

// NewGeminiReviewer creates a new Gemini API reviewer
func NewGeminiReviewer(config AIConfig) (*GeminiReviewer, error) {
	if config.Provider != "gemini" {
		return nil, fmt.Errorf("provider must be 'gemini' for GeminiReviewer")
	}

	// Set default model if not specified
	if config.Model == "" {
		config.Model = "gemini-2.5-pro"
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

	return &GeminiReviewer{
		config:      config,
		client:      client,
		rateLimiter: rateLimiter,
		usage:       UsageStats{},
	}, nil
}

// ReviewFile reviews a single file using Gemini API
func (g *GeminiReviewer) ReviewFile(ctx context.Context, file *internalfs.FileInfo) ([]review.Issue, error) {
	startTime := time.Now()

	// Read file content
	content, err := g.readFileContent(file.Path)
	if err != nil {
		g.recordFailure()
		return nil, fmt.Errorf("failed to read file %s: %w", file.Path, err)
	}

	// Build review request
	req := ReviewRequest{
		Relative:   file.Relative,
		Content:    content,
		Language:   file.Languages,
		Guidelines: GetLanguageGuidelines(file.Languages),
	}

	// Build prompt
	prompt, err := BuildPrompt(req)
	if err != nil {
		g.recordFailure()
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}

	// Make API request with retries
	var issues []review.Issue
	var lastErr error

	for attempt := 0; attempt <= g.config.MaxRetries; attempt++ {
		// Wait for rate limiter
		if err := g.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}

		// Make API request
		issues, lastErr = g.makeAPIRequest(ctx, prompt, file)
		if lastErr == nil {
			break // Success
		}

		// Check if we should retry
		if attempt < g.config.MaxRetries && g.shouldRetry(lastErr) {
			g.recordRetry()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(g.config.RateLimit.WaitTime * time.Duration(attempt+1)):
				// Exponential backoff
				continue
			}
		}
	}

	if lastErr != nil {
		g.recordFailure()
		return nil, fmt.Errorf("failed to review file after %d attempts: %w",
			g.config.MaxRetries+1, lastErr)
	}

	// Record success
	duration := time.Since(startTime)
	g.recordSuccess(duration, len(prompt), 0) // Token counting would need actual API response

	return issues, nil
}

// Name returns the reviewer name
func (g *GeminiReviewer) Name() string {
	return fmt.Sprintf("gemini-%s", g.config.Model)
}

// ValidateConfig validates the Gemini configuration
func (g *GeminiReviewer) ValidateConfig() error {
	if g.config.APIKey == "" {
		return fmt.Errorf("Gemini API key is required")
	}

	// Test API connectivity with a simple request
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s?key=%s",
		g.config.Model, g.config.APIKey)

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Gemini API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gemini API test failed with status %d: %s",
			resp.StatusCode, string(body))
	}

	return nil
}

// GetUsage returns usage statistics
func (g *GeminiReviewer) GetUsage() UsageStats {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.usage
}

// makeAPIRequest makes the actual Gemini API request
func (g *GeminiReviewer) makeAPIRequest(ctx context.Context, prompt string, file *internalfs.FileInfo) ([]review.Issue, error) {
	// Prepare request payload
	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{
						"text": prompt,
					},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     g.config.Temperature,
			"topP":            0.95,
			"topK":            40,
			"maxOutputTokens": g.config.MaxTokens,
			"stopSequences":   []string{},
		},
		"safetySettings": []map[string]interface{}{
			{
				"category":  "HARM_CATEGORY_HARASSMENT",
				"threshold": "BLOCK_NONE",
			},
			{
				"category":  "HARM_CATEGORY_HATE_SPEECH",
				"threshold": "BLOCK_NONE",
			},
			{
				"category":  "HARM_CATEGORY_SEXUALLY_EXPLICIT",
				"threshold": "BLOCK_NONE",
			},
			{
				"category":  "HARM_CATEGORY_DANGEROUS_CONTENT",
				"threshold": "BLOCK_NONE",
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	// Create request
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		g.config.Model, g.config.APIKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := g.client.Do(req)
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
					return nil, fmt.Errorf("Gemini API error: %s", message)
				}
			}
		}
		return nil, fmt.Errorf("Gemini API returned status %d: %s",
			resp.StatusCode, string(respBody))
	}

	// Debug: log response body if empty or suspicious
	if len(respBody) == 0 {
		log.Printf("Warning: Empty response body from Gemini API for file %s", file.Path)
		return nil, fmt.Errorf("empty response from Gemini API")
	}
	if len(respBody) < 50 {
		log.Printf("Warning: Small response body from Gemini API (%d bytes): %s", len(respBody), string(respBody))
	}

	// Parse response
	return g.parseAPIResponse(respBody, file)
}

// parseAPIResponse parses the Gemini API response
func (g *GeminiReviewer) parseAPIResponse(response []byte, file *internalfs.FileInfo) ([]review.Issue, error) {
	var apiResponse struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason  string `json:"finishReason"`
			SafetyRatings []struct {
				Category    string `json:"category"`
				Probability string `json:"probability"`
			} `json:"safetyRatings"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}

	if err := json.Unmarshal(response, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal API response: %w", err)
	}

	if len(apiResponse.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in API response")
	}

	// Extract text from response
	var textBuilder strings.Builder
	for _, part := range apiResponse.Candidates[0].Content.Parts {
		textBuilder.WriteString(part.Text)
	}
	text := textBuilder.String()

	// Clean the response text (remove markdown code blocks, etc.)
	text = g.cleanResponseText(text)

	// Parse JSON issues from response
	var issues []review.Issue
	if text != "" && text != "[]" {
		if err := json.Unmarshal([]byte(text), &issues); err != nil {
			// Try to extract JSON from malformed response
			issues = g.extractIssuesFromText(text, file)
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
	g.mu.Lock()
	g.usage.PromptTokens += int64(apiResponse.UsageMetadata.PromptTokenCount)
	g.usage.CompletionTokens += int64(apiResponse.UsageMetadata.CandidatesTokenCount)
	g.usage.TotalTokens += int64(apiResponse.UsageMetadata.TotalTokenCount)
	g.mu.Unlock()

	return issues, nil
}

// cleanResponseText cleans the API response text
func (g *GeminiReviewer) cleanResponseText(text string) string {
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
func (g *GeminiReviewer) extractIssuesFromText(text string, file *internalfs.FileInfo) []review.Issue {
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
				start += 9 // len("\"title\":\"")
				if end := strings.Index(line[start:], "\""); end != -1 {
					currentIssue.Title = line[start : start+end]
				}
			}
		} else if currentIssue != nil {
			// Extract other fields similarly
			if strings.Contains(line, "\"severity\":") {
				if start := strings.Index(line, "\"severity\":\""); start != -1 {
					start += 12 // len("\"severity\":\"")
					if end := strings.Index(line[start:], "\""); end != -1 {
						currentIssue.Severity = review.Severity(line[start : start+end])
					}
				}
			} else if strings.Contains(line, "\"description\":") {
				if start := strings.Index(line, "\"description\":\""); start != -1 {
					start += 15 // len("\"description\":\"")
					if end := strings.Index(line[start:], "\""); end != -1 {
						currentIssue.Description = line[start : start+end]
					}
				}
			} else if strings.Contains(line, "\"line\":") {
				if start := strings.Index(line, "\"line\":"); start != -1 {
					start += 7 // len("\"line\":")
					// Parse integer
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
func (g *GeminiReviewer) readFileContent(path string) (string, error) {
	// Use standard library to read file
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
func (g *GeminiReviewer) shouldRetry(err error) bool {
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
func (g *GeminiReviewer) recordSuccess(duration time.Duration, promptTokens, completionTokens int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.usage.TotalRequests++
	g.usage.Successful++
	g.usage.TotalDuration += duration
	g.usage.PromptTokens += int64(promptTokens)
	g.usage.CompletionTokens += int64(completionTokens)
	g.usage.TotalTokens += int64(promptTokens + completionTokens)

	// Calculate cost (approximate, would need actual pricing)
	// Gemini 2.5 Pro pricing example: $0.000125 per 1K tokens
	costPerToken := 0.000125 / 1000
	g.usage.TotalCost += float64(promptTokens+completionTokens) * costPerToken
}

// recordFailure records failed API call
func (g *GeminiReviewer) recordFailure() {
	atomic.AddInt64(&g.usage.TotalRequests, 1)
	atomic.AddInt64(&g.usage.Failed, 1)
}

// recordRetry records retry attempt
func (g *GeminiReviewer) recordRetry() {
	atomic.AddInt64(&g.usage.Retried, 1)
}



// BuggyFunction demonstrates critical security and resource management issues
func (g *GeminiReviewer) BuggyFunction(path string) (string, error) {
    file, err := os.Open(path)
    if err != nil {
        return "", err
    }
    // MISSING: defer file.Close() - This is a resource leak!
    
    // Hardcoded API key - SECURITY VULNERABILITY
    apiKey := "sk-1234567890abcdefghijklmnop"
    
    content, _ := io.ReadAll(file)
    // Ignoring error - bad practice
    
    return string(content) + apiKey, nil
}
// VeryObviousBug has multiple critical issues
