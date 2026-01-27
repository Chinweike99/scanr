package reviewer

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"scanr/internal/fs"
	"scanr/internal/review"
)

// MockReviewer is a mock implementation of the Reviewer interface for testing
type MockReviewer struct {
	name          string
	errorRate     float64 // 0.0 to 1.0
	avgLatency    time.Duration
	latencyJitter time.Duration
	issueRate     float64 // Average issues per file
	rng           *rand.Rand
}

// NewMockReviewer creates a new mock reviewer
func NewMockReviewer(name string, opts ...MockOption) *MockReviewer {
	mr := &MockReviewer{
		name:          name,
		errorRate:     0.05, // 5% error rate
		avgLatency:    100 * time.Millisecond,
		latencyJitter: 50 * time.Millisecond,
		issueRate:     0.3, // 30% chance of an issue per file
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	for _, opt := range opts {
		opt(mr)
	}

	return mr
}

// MockOption configures the mock reviewer
type MockOption func(*MockReviewer)

// WithErrorRate sets the error rate (0.0 to 1.0)
func WithErrorRate(rate float64) MockOption {
	return func(mr *MockReviewer) {
		mr.errorRate = rate
	}
}

// WithLatency sets the average latency and jitter
func WithLatency(avg, jitter time.Duration) MockOption {
	return func(mr *MockReviewer) {
		mr.avgLatency = avg
		mr.latencyJitter = jitter
	}
}

// WithIssueRate sets the issue rate (average issues per file)
func WithIssueRate(rate float64) MockOption {
	return func(mr *MockReviewer) {
		mr.issueRate = rate
	}
}

// ReviewFile implements the Reviewer interface
func (m *MockReviewer) ReviewFile(ctx context.Context, file *fs.FileInfo) ([]review.Issue, error) {
	// Simulate latency
	latency := m.avgLatency + time.Duration(m.rng.Int63n(int64(m.latencyJitter)))
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(latency):
		// Continue
	}

	// Simulate random errors
	if m.rng.Float64() < m.errorRate {
		return nil, fmt.Errorf("mock review error for %s", file.Path)
	}

	// Generate mock issues
	var issues []review.Issue

	// Determine number of issues for this file
	// Using Poisson distribution for more realistic distribution
	expectedIssues := m.issueRate
	numIssues := 0
	for i := 0; i < 10; i++ { // Cap at 10 issues per file
		if m.rng.Float64() < expectedIssues/float64(i+1) {
			numIssues++
		} else {
			break
		}
	}

	for i := 0; i < numIssues; i++ {
		issue := m.generateMockIssue(file)
		issues = append(issues, issue)
	}

	return issues, nil
}

// Name returns the reviewer name
func (m *MockReviewer) Name() string {
	return m.name
}

// generateMockIssue generates a mock issue
func (m *MockReviewer) generateMockIssue(file *fs.FileInfo) review.Issue {
	// Common issue patterns
	issueTypes := []struct {
		title       string
		description string
		severity    review.Severity
		category    string
	}{
		{
			title:       "Hardcoded secret",
			description: "Potential hardcoded API key or password found",
			severity:    review.SeverityCritical,
			category:    "security",
		},
		{
			title:       "Unhandled error",
			description: "Error returned from function is not handled",
			severity:    review.SeverityCritical,
			category:    "reliability",
		},
		{
			title:       "Resource leak",
			description: "File/connection may not be closed in all code paths",
			severity:    review.SeverityCritical,
			category:    "performance",
		},
		{
			title:       "Long function",
			description: "Function exceeds recommended length of 50 lines",
			severity:    review.SeverityHigh,
			category:    "maintainability",
		},
		{
			title:       "Complex conditional",
			description: "Conditional logic is too complex (high cyclomatic complexity)",
			severity:    review.SeverityHigh,
			category:    "readability",
		},
		{
			title:       "Naming inconsistency",
			description: "Variable/function naming doesn't follow project conventions",
			severity:    review.SeverityHigh,
			category:    "style",
		},
		{
			title:       "Missing documentation",
			description: "Public function/type lacks documentation",
			severity:    review.SeverityInfo,
			category:    "documentation",
		},
		{
			title:       "Magic number",
			description: "Consider using named constant instead of literal value",
			severity:    review.SeverityInfo,
			category:    "readability",
		},
	}

	issueType := issueTypes[m.rng.Intn(len(issueTypes))]

	// Generate random line number (1-100)
	line := m.rng.Intn(100) + 1

	// Generate random confidence (0.5-1.0)
	confidence := 0.5 + m.rng.Float64()*0.5

	return review.Issue{
		FilePath:    file.Path,
		Line:        line,
		Column:      m.rng.Intn(80) + 1,
		Code:        fmt.Sprintf("MOCK%03d", m.rng.Intn(1000)),
		Title:       issueType.title,
		Description: issueType.description,
		Severity:    issueType.severity,
		Category:    issueType.category,
		Suggestions: m.generateSuggestions(issueType.category),
		Confidence:  confidence,
		FoundAt:     time.Now(),
	}
}

// generateSuggestions generates mock suggestions based on category
func (m *MockReviewer) generateSuggestions(category string) []string {
	suggestions := map[string][]string{
		"security": {
			"Use environment variables or secret management system",
			"Rotate the credential immediately",
			"Add the pattern to .gitignore",
		},
		"reliability": {
			"Add error handling using try-catch or equivalent",
			"Log the error for debugging",
			"Return appropriate error to caller",
		},
		"performance": {
			"Use try-with-resources pattern",
			"Implement finally block for cleanup",
			"Consider using defer statement",
		},
		"maintainability": {
			"Break function into smaller helper functions",
			"Extract complex logic into separate method",
			"Consider using strategy pattern",
		},
		"readability": {
			"Extract condition to named boolean variable",
			"Use guard clauses to reduce nesting",
			"Consider using switch statement",
		},
		"style": {
			"Follow camelCase naming convention",
			"Use descriptive names that indicate purpose",
			"Avoid abbreviations unless widely understood",
		},
		"documentation": {
			"Add JSDoc/GoDoc style comment",
			"Document parameters and return value",
			"Add usage example if complex",
		},
	}

	if categorySuggestions, ok := suggestions[category]; ok {
		// Return 1-3 random suggestions
		count := m.rng.Intn(3) + 1
		if count > len(categorySuggestions) {
			count = len(categorySuggestions)
		}

		result := make([]string, count)
		for i := 0; i < count; i++ {
			result[i] = categorySuggestions[m.rng.Intn(len(categorySuggestions))]
		}
		return result
	}

	return []string{"Consider reviewing this code carefully"}
}
