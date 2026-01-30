package output

import (
	"encoding/json"
	"fmt"
	"io"
	"scanr/internal/fs"
	"scanr/internal/review"
	"sort"
	"time"
)

// JSONFormatter formats review results as JSON
type JSONFormatter struct {
	config Config
}

// NewJSONFormatter creates a new JSON formatter
func NewJSONFormatter(config Config) *JSONFormatter {
	return &JSONFormatter{config: config}
}

// JSONOutput is the structured JSON output format
type JSONOutput struct {
	Meta    JSONMeta         `json:"meta"`
	Summary JSONSummary      `json:"summary"`
	Results []JSONFileResult `json:"results,omitempty"`
	Issues  []JSONIssue      `json:"issues,omitempty"`
}

// JSONMeta contains metadata about the review
type JSONMeta struct {
	Tool      string    `json:"tool"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Duration  float64   `json:"duration_ms"`
	Command   string    `json:"command,omitempty"`
}

// JSONSummary contains review summary statistics
type JSONSummary struct {
	TotalFiles    int `json:"total_files"`
	ReviewedFiles int `json:"reviewed_files"`
	FailedFiles   int `json:"failed_files"`
	TotalIssues   int `json:"total_issues"`
	CriticalCount int `json:"critical_count"`
	WarningCount  int `json:"warning_count"`
	InfoCount     int `json:"info_count"`
}

// JSONFileResult contains results for a single file
type JSONFileResult struct {
	File     JSONFileInfo `json:"file"`
	Issues   []JSONIssue  `json:"issues"`
	Duration float64      `json:"duration_ms"`
	Error    string       `json:"error,omitempty"`
}

// JSONFileInfo contains file information
type JSONFileInfo struct {
	Path     string `json:"path"`
	Relative string `json:"relative"`
	Language string `json:"language"`
	Size     int64  `json:"size"`
	Lines    int    `json:"lines"`
}

// JSONIssue contains a single issue
type JSONIssue struct {
	ID          string    `json:"id,omitempty"`
	FilePath    string    `json:"file_path"`
	Relative    string    `json:"relative_path,omitempty"`
	Line        int       `json:"line,omitempty"`
	Column      int       `json:"column,omitempty"`
	Code        string    `json:"code,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Severity    string    `json:"severity"`
	Category    string    `json:"category,omitempty"`
	Suggestions []string  `json:"suggestions,omitempty"`
	Confidence  float64   `json:"confidence,omitempty"`
	FoundAt     time.Time `json:"found_at"`
}

// Formats review results as JSON
func (f *JSONFormatter) Format(result *review.ReviewResult, w io.Writer) error {
	output := f.buildJSONOutput(result)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)

	return encoder.Encode(output)
}

// Formats streaming review results as JSON Lines (NDJSON)
func (f *JSONFormatter) FormatStream(issues <-chan *review.FileReview, w io.Writer) error {
	encoder := json.NewEncoder(w)

	for fileReview := range issues {
		jsonResult := f.convertFileReview(fileReview)
		if err := encoder.Encode(jsonResult); err != nil {
			return fmt.Errorf("failed to encode JSON line: %w", err)
		}
	}

	return nil
}

// buildJSONOutput builds the complete JSON output structure
func (f *JSONFormatter) buildJSONOutput(result *review.ReviewResult) JSONOutput {
	meta := JSONMeta{
		Tool:      "scanr",
		Version:   "1.0.0",
		Timestamp: result.StartTime,
		Duration:  result.Duration.Seconds() * 1000,
	}

	summary := JSONSummary{
		TotalFiles:    result.TotalFiles,
		ReviewedFiles: result.ReviewedFiles,
		FailedFiles:   result.TotalFiles - result.ReviewedFiles,
		TotalIssues:   result.TotalIssues,
		CriticalCount: result.CriticalCount,
		WarningCount:  result.WarningCount,
		InfoCount:     result.InfoCount,
	}

	output := JSONOutput{
		Meta:    meta,
		Summary: summary,
	}

	// Build results based on grouping preference
	if f.config.GroupBy == "file" || f.config.GroupBy == "" {
		output.Results = f.buildFileResults(result)
	} else {
		output.Issues = f.buildFlatIssues(result)
	}

	return output
}

// buildFileResults builds file-grouped results
func (f *JSONFormatter) buildFileResults(result *review.ReviewResult) []JSONFileResult {
	var results []JSONFileResult

	for _, fileReview := range result.FileReviews {
		if len(fileReview.Issues) == 0 && !f.config.ShowSuccess {
			continue
		}

		jsonResult := f.convertFileReview(&fileReview)
		results = append(results, jsonResult)
	}

	return results
}

// buildFlatIssues builds a flat list of issues
func (f *JSONFormatter) buildFlatIssues(result *review.ReviewResult) []JSONIssue {
	var issues []JSONIssue

	for _, fileReview := range result.FileReviews {
		for _, issue := range fileReview.Issues {
			jsonIssue := f.convertIssue(issue, *fileReview.File)
			issues = append(issues, jsonIssue)
		}
	}

	// Sort issues based on configuration
	issues = f.sortIssues(issues)

	// Apply max issues limit
	if f.config.MaxIssues > 0 && len(issues) > f.config.MaxIssues {
		issues = issues[:f.config.MaxIssues]
	}

	return issues
}

// convertFileReview converts a FileReview to JSONFileResult
func (f *JSONFormatter) convertFileReview(fileReview *review.FileReview) JSONFileResult {
	fileInfo := JSONFileInfo{
		Path:     fileReview.File.Path,
		Relative: fileReview.File.Relative,
		Language: fileReview.File.Languages,
		Size:     fileReview.File.Size,
		Lines:    fileReview.File.Lines,
	}

	var issues []JSONIssue
	for _, issue := range fileReview.Issues {
		jsonIssue := f.convertIssue(issue, *fileReview.File)
		issues = append(issues, jsonIssue)
	}

	// Sort issues within file
	issues = f.sortIssues(issues)

	if f.config.MaxIssues > 0 && len(issues) > f.config.MaxIssues {
		issues = issues[:f.config.MaxIssues]
	}

	return JSONFileResult{
		File:     fileInfo,
		Issues:   issues,
		Duration: fileReview.Duration.Seconds() * 1000,
		Error:    fileReview.Error,
	}
}

// convertIssue converts an Issue to JSONIssue
func (f *JSONFormatter) convertIssue(issue review.Issue, file fs.FileInfo) JSONIssue {
	return JSONIssue{
		FilePath:    issue.FilePath,
		Relative:    file.Relative,
		Line:        issue.Line,
		Column:      issue.Column,
		Code:        issue.Code,
		Title:       issue.Title,
		Description: issue.Description,
		Severity:    string(issue.Severity),
		Category:    issue.Category,
		Suggestions: issue.Suggestions,
		Confidence:  issue.Confidence,
		FoundAt:     issue.FoundAt,
	}
}

// sortIssues sorts JSON issues based on configuration
func (f *JSONFormatter) sortIssues(issues []JSONIssue) []JSONIssue {
	sorted := make([]JSONIssue, len(issues))
	copy(sorted, issues)

	switch f.config.SortBy {
	case "severity":
		sort.Slice(sorted, func(i, j int) bool {
			// Critical > Warning > Info
			severityOrder := map[string]int{
				"critical": 3,
				"warning":  2,
				"info":     1,
			}

			if severityOrder[sorted[i].Severity] == severityOrder[sorted[j].Severity] {
				// Same severity, sort by file then line
				if sorted[i].Relative == sorted[j].Relative {
					if sorted[i].Line == sorted[j].Line {
						return sorted[i].Title < sorted[j].Title
					}
					return sorted[i].Line < sorted[j].Line
				}
				return sorted[i].Relative < sorted[j].Relative
			}

			return severityOrder[sorted[i].Severity] > severityOrder[sorted[j].Severity]
		})

	case "file":
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].Relative == sorted[j].Relative {
				if sorted[i].Line == sorted[j].Line {
					return sorted[i].Title < sorted[j].Title
				}
				return sorted[i].Line < sorted[j].Line
			}
			return sorted[i].Relative < sorted[j].Relative
		})

	case "line":
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].Line == sorted[j].Line {
				if sorted[i].Column == sorted[j].Column {
					return sorted[i].Title < sorted[j].Title
				}
				return sorted[i].Column < sorted[j].Column
			}
			return sorted[i].Line < sorted[j].Line
		})

	default:
		// Default sort by severity then file then line
		sort.Slice(sorted, func(i, j int) bool {
			severityOrder := map[string]int{
				"critical": 3,
				"warning":  2,
				"info":     1,
			}

			if severityOrder[sorted[i].Severity] == severityOrder[sorted[j].Severity] {
				if sorted[i].Relative == sorted[j].Relative {
					if sorted[i].Line == sorted[j].Line {
						return sorted[i].Title < sorted[j].Title
					}
					return sorted[i].Line < sorted[j].Line
				}
				return sorted[i].Relative < sorted[j].Relative
			}

			return severityOrder[sorted[i].Severity] > severityOrder[sorted[j].Severity]
		})
	}

	return sorted
}
