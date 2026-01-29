package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"scanr/internal/review"
	"strings"
	"testing"
)

func TestJSONFormatter_Format(t *testing.T) {
    result := createTestReviewResult()
    
    tests := []struct {
        name   string
        config Config
        check  func(JSONOutput) bool
    }{
        {
            name: "default configuration",
            config: Config{
                Format:      "json",
                GroupBy:     "file",
                ShowSuccess: false,
            },
            check: func(output JSONOutput) bool {
                return output.Summary.TotalFiles == 10 &&
                    output.Summary.ReviewedFiles == 8 &&
                    output.Summary.TotalIssues == 5 &&
                    output.Summary.CriticalCount == 1 &&
                    output.Summary.WarningCount == 3 &&
                    output.Summary.InfoCount == 1 &&
                    len(output.Results) == 2 && // Two files with issues
                    len(output.Issues) == 0
            },
        },
        {
            name: "show success files",
            config: Config{
                Format:      "json",
                GroupBy:     "file",
                ShowSuccess: true,
            },
            check: func(output JSONOutput) bool {
                return len(output.Results) == 3 // All three files
            },
        },
        {
            name: "flat issues",
            config: Config{
                Format:  "json",
                GroupBy: "severity", // Anything except "file" gives flat issues
            },
            check: func(output JSONOutput) bool {
                return len(output.Issues) == 5 && // All issues flattened
                    len(output.Results) == 0
            },
        },
        {
            name: "max issues limit",
            config: Config{
                Format:   "json",
                GroupBy:  "file",
                MaxIssues: 1,
            },
            check: func(output JSONOutput) bool {
                // Should have at most 1 issue per file
                for _, result := range output.Results {
                    if len(result.Issues) > 1 {
                        return false
                    }
                }
                return true
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            formatter := NewJSONFormatter(tt.config)
            
            var buf bytes.Buffer
            err := formatter.Format(result, &buf)
            if err != nil {
                t.Fatalf("Format failed: %v", err)
            }
            
            var output JSONOutput
            if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
                t.Fatalf("Failed to unmarshal JSON: %v", err)
            }
            
            if !tt.check(output) {
                t.Errorf("Output validation failed for %s", tt.name)
                t.Logf("Output: %s", buf.String())
            }
        })
    }
}

func TestJSONFormatter_Sorting(t *testing.T) {
    result := createTestReviewResult()
    
    tests := []struct {
        name   string
        sortBy string
        check  func(JSONOutput) bool
    }{
        {
            name:   "sort by severity",
            sortBy: "severity",
            check: func(output JSONOutput) bool {
                if len(output.Issues) == 0 {
                    // Check file results
                    for _, fileResult := range output.Results {
                        if len(fileResult.Issues) < 2 {
                            continue
                        }
                        // Issues should be sorted by severity
                        severityOrder := map[string]int{
                            "critical": 3,
                            "warning":  2,
                            "info":     1,
                        }
                        for i := 1; i < len(fileResult.Issues); i++ {
                            prev := fileResult.Issues[i-1].Severity
                            curr := fileResult.Issues[i].Severity
                            if severityOrder[prev] < severityOrder[curr] {
                                return false
                            }
                        }
                    }
                }
                return true
            },
        },
        {
            name:   "sort by file",
            sortBy: "file",
            check: func(output JSONOutput) bool {
                if len(output.Results) > 0 {
                    // Files should be sorted alphabetically
                    for i := 1; i < len(output.Results); i++ {
                        prev := output.Results[i-1].File.Relative
                        curr := output.Results[i].File.Relative
                        if prev > curr {
                            return false
                        }
                    }
                }
                return true
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            config := Config{
                Format:  "json",
                GroupBy: "severity", // Use flat issues for easier testing
                SortBy:  tt.sortBy,
            }
            
            formatter := NewJSONFormatter(config)
            
            var buf bytes.Buffer
            err := formatter.Format(result, &buf)
            if err != nil {
                t.Fatalf("Format failed: %v", err)
            }
            
            var output JSONOutput
            if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
                t.Fatalf("Failed to unmarshal JSON: %v", err)
            }
            
            if !tt.check(output) {
                t.Errorf("Sorting validation failed for %s", tt.name)
                t.Logf("Output: %s", buf.String())
            }
        })
    }
}

func TestJSONFormatter_Stream(t *testing.T) {
    result := createTestReviewResult()
    
    config := Config{
        Format: "json",
    }
    
    formatter := NewJSONFormatter(config)
    
    // Create a channel with file reviews
    reviews := make(chan *review.FileReview, len(result.FileReviews))
    for i := range result.FileReviews {
        reviews <- &result.FileReviews[i]
    }
    close(reviews)
    
    var buf bytes.Buffer
    err := formatter.FormatStream(reviews, &buf)
    if err != nil {
        t.Fatalf("FormatStream failed: %v", err)
    }
    
    // Each line should be valid JSON
    lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
    if len(lines) != len(result.FileReviews) {
        t.Errorf("Expected %d JSON lines, got %d", len(result.FileReviews), len(lines))
    }
    
    for i, line := range lines {
        var jsonLine map[string]interface{}
        if err := json.Unmarshal([]byte(line), &jsonLine); err != nil {
            t.Errorf("Line %d is not valid JSON: %v", i, err)
        }
    }
}

func TestFormatterFactory(t *testing.T) {
    factory := NewFormatterFactory()
    
    tests := []struct {
		name   string
		format string
		want   string
		err    bool
    }{
        {
            name:   "text formatter",
            format: "text",
            want:   "*output.TextFormatter",
        },
        {
            name:   "json formatter",
            format: "json",
            want:   "*output.JSONFormatter",
        },
        {
            name:   "jsonl formatter",
            format: "jsonl",
            want:   "*output.JSONFormatter",
        },
        {
            name:   "invalid formatter",
            format: "xml",
            err:    true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            config := DefaultConfig()
            config.Format = tt.format
            
            formatter, err := factory.CreateFormatter(config)
            
            if tt.err {
                if err == nil {
                    t.Error("expected error, got nil")
                }
                return
            }
            
            if err != nil {
                t.Errorf("unexpected error: %v", err)
                return
            }
            
            got := fmt.Sprintf("%T", formatter)
            if got != tt.want {
                t.Errorf("CreateFormatter() = %s, want %s", got, tt.want)
            }
        })
    }
}