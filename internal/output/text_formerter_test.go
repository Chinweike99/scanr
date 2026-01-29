package output

import (
	"bytes"
	"scanr/internal/fs"
	"scanr/internal/review"
	"strings"
	"testing"
	"time"
)

func createTestReviewResult() *review.ReviewResult {
	now := time.Now()

	return &review.ReviewResult{
		TotalFiles:    10,
		ReviewedFiles: 8,
		TotalIssues:   5,
		CriticalCount: 1,
		WarningCount:  3,
		InfoCount:     1,
		FileReviews: []review.FileReview{
			{
				File: &fs.FileInfo{
					Path:      "/project/src/main.go",
					Relative:  "src/main.go",
					Languages: "go",
					Size:      1024,
					Lines:     50,
				},
				Issues: []review.Issue{
					{
						FilePath:    "/project/src/main.go",
						Line:        25,
						Column:      10,
						Code:        "SEC001",
						Title:       "Hardcoded API key",
						Description: "Potential hardcoded API key found in source code",
						Severity:    review.SeverityCritical,
						Category:    "security",
						Suggestions: []string{
							"Use environment variable",
							"Store in secure vault",
						},
						Confidence: 0.9,
						FoundAt:    now,
					},
					{
						FilePath:    "/project/src/main.go",
						Line:        42,
						Title:       "Long function",
						Description: "Function exceeds 40 lines, consider refactoring",
						Severity:    review.SeverityHigh,
						Category:    "maintainability",
						Confidence:  0.7,
						FoundAt:     now,
					},
				},
				Duration: 150 * time.Millisecond,
			},
			{
				File: &fs.FileInfo{
					Path:      "/project/src/utils.py",
					Relative:  "src/utils.py",
					Languages: "python",
					Size:      2048,
					Lines:     30,
				},
				Issues: []review.Issue{
					{
						FilePath:    "/project/src/utils.py",
						Line:        15,
						Title:       "Missing docstring",
						Description: "Function is missing docstring",
						Severity:    review.SeverityInfo,
						Category:    "documentation",
						Confidence:  0.8,
						FoundAt:     now,
					},
				},
				Duration: 100 * time.Millisecond,
			},
			{
				File: &fs.FileInfo{
					Path:      "/project/src/clean.go",
					Relative:  "src/clean.go",
					Languages: "go",
					Size:      512,
					Lines:     20,
				},
				Issues:   []review.Issue{},
				Duration: 50 * time.Millisecond,
			},
		},
		Duration:  2 * time.Second,
		StartTime: now.Add(-2 * time.Second),
		EndTime:   now,
	}
}

func TestTextFormatter_Format(t *testing.T) {
	result := createTestReviewResult()

	tests := []struct {
		name   string
		config Config
		check  func(string) bool
	}{
		{
			name: "default configuration",
			config: Config{
				Format:      "text",
				Color:       false,
				ShowSuccess: false,
				GroupBy:     "file",
				SortBy:      "severity",
			},
			check: func(output string) bool {
				return strings.Contains(output, "scanr CODE REVIEW") &&
					strings.Contains(output, "SUMMARY") &&
					strings.Contains(output, "src/main.go") &&
					strings.Contains(output, "Critical:  1") &&
					strings.Contains(output, "Warnings:  3") &&
					strings.Contains(output, "Info:      1")
			},
		},
		{
			name: "show success files",
			config: Config{
				Format:      "text",
				Color:       false,
				ShowSuccess: true,
				GroupBy:     "file",
			},
			check: func(output string) bool {
				return strings.Contains(output, "src/clean.go") &&
					strings.Contains(output, "No issues found")
			},
		},
		{
			name: "summary only",
			config: Config{
				Format:      "text",
				Color:       false,
				SummaryOnly: true,
			},
			check: func(output string) bool {
				return strings.Contains(output, "SUMMARY") &&
					!strings.Contains(output, "Hardcoded API key") // Should not show individual issues
			},
		},
		{
			name: "max issues limit",
			config: Config{
				Format:    "text",
				Color:     false,
				MaxIssues: 1,
			},
			check: func(output string) bool {
				// Should only show one issue or indicate more
				criticalCount := strings.Count(output, "CRITICAL")
				return criticalCount <= 2
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewTextFormatter(tt.config)

			var buf bytes.Buffer
			err := formatter.Format(result, &buf)
			if err != nil {
				t.Fatalf("Format failed: %v", err)
			}

			output := buf.String()
			if !tt.check(output) {
				t.Errorf("Output validation failed for %s", tt.name)
				t.Logf("Output:\n%s", output)
			}
		})
	}
}

func TestTextFormatter_Sorting(t *testing.T) {
	result := createTestReviewResult()

	tests := []struct {
		name   string
		sortBy string
		check  func(string) bool
	}{
		{
			name:   "sort by severity",
			sortBy: "severity",
			check: func(output string) bool {
				// Critical should come before warning, warning before info
				critIndex := strings.Index(output, "CRITICAL")
				warnIndex := strings.Index(output, "WARNING")
				infoIndex := strings.Index(output, "INFO")

				return critIndex < warnIndex && warnIndex < infoIndex
			},
		},
		{
			name:   "sort by file",
			sortBy: "file",
			check: func(output string) bool {
				// Files should be in alphabetical order
				mainIndex := strings.Index(output, "src/main.go")
				utilsIndex := strings.Index(output, "src/utils.py")
				return mainIndex < utilsIndex
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Format: "text",
				Color:  false,
				SortBy: tt.sortBy,
			}

			formatter := NewTextFormatter(config)

			var buf bytes.Buffer
			err := formatter.Format(result, &buf)
			if err != nil {
				t.Fatalf("Format failed: %v", err)
			}

			output := buf.String()
			if !tt.check(output) {
				t.Errorf("Sorting validation failed for %s", tt.name)
				t.Logf("Output:\n%s", output)
			}
		})
	}
}

func TestTextFormatter_Color(t *testing.T) {
	result := createTestReviewResult()

	config := Config{
		Format: "text",
		Color:  true,
	}

	formatter := NewTextFormatter(config)

	var buf bytes.Buffer
	err := formatter.Format(result, &buf)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	output := buf.String()
	// Ensure colored output still includes expected content
	if !strings.Contains(output, "scanr CODE REVIEW") {
		t.Error("Expected header in colored output")
	}
}

func TestDetermineExitCode(t *testing.T) {
	tests := []struct {
		name     string
		result   *review.ReviewResult
		expected int
	}{
		{
			name: "critical issues",
			result: &review.ReviewResult{
				CriticalCount: 1,
				WarningCount:  5,
				InfoCount:     10,
			},
			expected: 2,
		},
		{
			name: "warning issues only",
			result: &review.ReviewResult{
				CriticalCount: 0,
				WarningCount:  3,
				InfoCount:     5,
			},
			expected: 1,
		},
		{
			name: "info issues only",
			result: &review.ReviewResult{
				CriticalCount: 0,
				WarningCount:  0,
				InfoCount:     2,
			},
			expected: 0,
		},
		{
			name: "no issues",
			result: &review.ReviewResult{
				CriticalCount: 0,
				WarningCount:  0,
				InfoCount:     0,
			},
			expected: 0,
		},
		{
			name:     "nil result",
			result:   nil,
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineExitCode(tt.result)
			if got != tt.expected {
				t.Errorf("DetermineExitCode() = %d, want %d", got, tt.expected)
			}
		})
	}
}
