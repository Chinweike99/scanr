package review

import (
	"context"
	"io/fs"
	"time"
)


type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "warning"
	SeverityInfo     Severity = "info"
)

type Issue struct {
    FilePath    string    `json:"file_path"`
    Line        int       `json:"line,omitempty"`
    Column      int       `json:"column,omitempty"`
    Code        string    `json:"code,omitempty"`
    Title       string    `json:"title"`
    Description string    `json:"description"`
    Severity    Severity  `json:"severity"`
    Category    string    `json:"category,omitempty"`
    Suggestions []string  `json:"suggestions,omitempty"`
    Confidence  float64   `json:"confidence,omitempty"`
    FoundAt     time.Time `json:"found_at"`
}

type FileReview struct {
    File     fs.FileInfo `json:"file"`
    Issues   []Issue     `json:"issues"`
    Duration time.Duration `json:"duration_ms"`
    Error    string      `json:"error,omitempty"`
}

type ReviewResult struct {
    TotalFiles    int           `json:"total_files"`
    ReviewedFiles int           `json:"reviewed_files"`
    TotalIssues   int           `json:"total_issues"`
    CriticalCount int           `json:"critical_count"`
    WarningCount  int           `json:"warning_count"`
    InfoCount     int           `json:"info_count"`
    FileReviews   []FileReview  `json:"file_reviews"`
    Duration      time.Duration `json:"total_duration_ms"`
    StartTime     time.Time     `json:"start_time"`
    EndTime       time.Time     `json:"end_time"`
}

// interface for reviewing files
type Reviewer interface {
    ReviewFile(ctx context.Context, file fs.FileInfo) ([]Issue, error)
    Name() string
}

type Pipeline interface {
    Run(ctx context.Context, files []fs.FileInfo) (*ReviewResult, error)
    Stop() error
}


