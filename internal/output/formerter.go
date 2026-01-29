package output

import (
	"io"
	"scanr/internal/review"
)

// Formatter is the interface for formatting review results
type Formatter interface {
    Format(result *review.ReviewResult, w io.Writer) error
    FormatStream(issues <-chan *review.FileReview, w io.Writer) error
}

// Config holds output configuration
type Config struct {
    Format       string 
    Color        bool  
    ShowSuccess  bool  
    GroupBy      string
    SortBy       string
    MaxIssues    int    
    SummaryOnly  bool  
}

// DefaultConfig returns the default output configuration
func DefaultConfig() Config {
    return Config{
        Format:      "text",
        Color:       true,
        ShowSuccess: false,
        GroupBy:     "file",
        SortBy:      "severity",
        MaxIssues:   0,
        SummaryOnly: false,
    }
}