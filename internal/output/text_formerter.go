package output

import (
	"fmt"
	"io"
	"scanr/internal/review"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
)

// Formats review results as human-readable text
type TextFormatter struct {
	config Config
}

// NewTextFormatter creates a new text formatter
func NewTextFormatter(config Config) *TextFormatter {
	return &TextFormatter{config: config}
}

// Formats review results as text
func (f *TextFormatter) Format(result *review.ReviewResult, w io.Writer) error {
	f.writeHeader(result, w)
	f.writeSummary(result, w)

	if !f.config.SummaryOnly && result.TotalIssues > 0 {
		f.writeIssues(result, w)
	}
	f.writeFooter(result, w)
	return nil
}

// Formats streaming review results
func (f *TextFormatter) FormatStream(issues <-chan *review.FileReview, w io.Writer) error {
	return fmt.Errorf("stream formatting not supported for text output")
}

// writeHeader writes the report header
func (f *TextFormatter) writeHeader(result *review.ReviewResult, w io.Writer) {
	width := 70
	separator := strings.Repeat("=", width)

	fmt.Fprintf(w, "\n%s\n", separator)
	fmt.Fprintf(w, "scanr CODE REVIEW\n")
	fmt.Fprintf(w, "%s\n", separator)

	fmt.Fprintf(w, "Date:     %s\n", result.StartTime.Format(time.RFC1123))
	fmt.Fprintf(w, "Duration: %v\n", result.Duration.Round(time.Millisecond))
	fmt.Fprintf(w, "\n")
}

// writeSummary writes the summary section
func (f *TextFormatter) writeSummary(result *review.ReviewResult, w io.Writer) {
	fmt.Fprintf(w, "SUMMARY\n")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 40))

	fmt.Fprintf(w, "Files:\n")
	fmt.Fprintf(w, "  Total:     %d\n", result.TotalFiles)
	fmt.Fprintf(w, "  Reviewed:  %d\n", result.ReviewedFiles)
	if result.TotalFiles > 0 {
		successRate := float64(result.ReviewedFiles) / float64(result.TotalFiles) * 100
		fmt.Fprintf(w, "  Success:   %.1f%%\n", successRate)
	}

	fmt.Fprintf(w, "\nIssues:\n")

	criticalColor := color.New(color.FgRed, color.Bold)
	if f.config.Color && result.CriticalCount > 0 {
		criticalColor.Fprintf(w, "  Critical:  %d\n", result.CriticalCount)
	} else {
		fmt.Fprintf(w, "  Critical:  %d\n", result.CriticalCount)
	}

	// Warning issues
	warningColor := color.New(color.FgYellow, color.Bold)
	if f.config.Color && result.WarningCount > 0 {
		warningColor.Fprintf(w, "  Warnings:  %d\n", result.WarningCount)
	} else {
		fmt.Fprintf(w, "  Warnings:  %d\n", result.WarningCount)
	}

	// Info issues
	infoColor := color.New(color.FgCyan)
	if f.config.Color && result.InfoCount > 0 {
		infoColor.Fprintf(w, "  Info:      %d\n", result.InfoCount)
	} else {
		fmt.Fprintf(w, "  Info:      %d\n", result.InfoCount)
	}

	fmt.Fprintf(w, "  Total:     %d\n", result.TotalIssues)

	// Success message if no issues
	if result.TotalIssues == 0 {
		fmt.Fprintf(w, "\n")
		successColor := color.New(color.FgGreen, color.Bold)
		if f.config.Color {
			successColor.Fprintf(w, "✅ No issues found! Your code looks great!\n")
		} else {
			fmt.Fprintf(w, "✅ No issues found! Your code looks great!\n")
		}
	}

	fmt.Fprintf(w, "\n")
}

// writeIssues writes individual issues
func (f *TextFormatter) writeIssues(result *review.ReviewResult, w io.Writer) {
	// Group and sort issues based on config
	issuesByFile := f.groupIssuesByFile(result)
	files := f.getSortedFiles(issuesByFile)

	// Apply max issues limit
	issuesWritten := 0

	for _, file := range files {
		if f.config.MaxIssues > 0 && issuesWritten >= f.config.MaxIssues {
			fmt.Fprintf(w, "... and %d more issues\n", result.TotalIssues-issuesWritten)
			break
		}

		fileReview := issuesByFile[file]
		if len(fileReview.Issues) == 0 && !f.config.ShowSuccess {
			continue
		}

		f.writeFileHeader(fileReview, w)

		// Sort issues within file
		sortedIssues := f.sortIssues(fileReview.Issues)

		for _, issue := range sortedIssues {
			if f.config.MaxIssues > 0 && issuesWritten >= f.config.MaxIssues {
				break
			}

			f.writeIssue(issue, w)
			issuesWritten++
		}

		if len(fileReview.Issues) == 0 && f.config.ShowSuccess {
			successColor := color.New(color.FgGreen)
			if f.config.Color {
				successColor.Fprintf(w, "  ✅ No issues found\n")
			} else {
				fmt.Fprintf(w, "  ✅ No issues found\n")
			}
		}

		fmt.Fprintf(w, "\n")
	}
}

// writeFileHeader writes the header for a file section
func (f *TextFormatter) writeFileHeader(fileReview *review.FileReview, w io.Writer) {
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 60))

	// File path
	pathColor := color.New(color.FgBlue, color.Bold)
	if f.config.Color {
		pathColor.Fprintf(w, "%s", fileReview.File.Relative)
	} else {
		fmt.Fprintf(w, "%s", fileReview.File.Relative)
	}

	// File info
	fmt.Fprintf(w, " (%s, %d lines", fileReview.File.Languages, fileReview.File.Lines)
	if fileReview.Duration > 0 {
		fmt.Fprintf(w, ", reviewed in %v", fileReview.Duration.Round(time.Millisecond))
	}
	fmt.Fprintf(w, ")\n")

	// Issue count
	if len(fileReview.Issues) > 0 {
		issueText := "issue"
		if len(fileReview.Issues) != 1 {
			issueText = "issues"
		}

		severityCounts := make(map[review.Severity]int)
		for _, issue := range fileReview.Issues {
			severityCounts[issue.Severity]++
		}

		var counts []string
		for severity, count := range severityCounts {
			if count > 0 {
				counts = append(counts, fmt.Sprintf("%d %s", count, severity))
			}
		}

		fmt.Fprintf(w, "Found %d %s: %s\n",
			len(fileReview.Issues), issueText, strings.Join(counts, ", "))
	}

	fmt.Fprintf(w, "\n")
}

// writeIssue writes a single issue
func (f *TextFormatter) writeIssue(issue review.Issue, w io.Writer) {
	// Severity indicator
	severityStr := f.formatSeverity(issue.Severity, w)

	// Location
	location := ""
	if issue.Line > 0 {
		if issue.Column > 0 {
			location = fmt.Sprintf("(%d:%d)", issue.Line, issue.Column)
		} else {
			location = fmt.Sprintf("(line %d)", issue.Line)
		}
	}

	// Title and location
	fmt.Fprintf(w, "  %s %s\n", severityStr, location)

	// Title (indented)
	fmt.Fprintf(w, "    %s\n", issue.Title)

	// Description
	if issue.Description != "" {
		fmt.Fprintf(w, "    %s\n", issue.Description)
	}

	// Code reference
	if issue.Code != "" {
		codeColor := color.New(color.Faint)
		if f.config.Color {
			codeColor.Fprintf(w, "    [%s]\n", issue.Code)
		} else {
			fmt.Fprintf(w, "    [%s]\n", issue.Code)
		}
	}

	// Suggestions
	if len(issue.Suggestions) > 0 {
		fmt.Fprintf(w, "    Suggestions:\n")
		for _, suggestion := range issue.Suggestions {
			fmt.Fprintf(w, "    • %s\n", suggestion)
		}
	}

	// Confidence
	if issue.Confidence > 0 {
		confidence := fmt.Sprintf("%.0f%%", issue.Confidence*100)
		if issue.Confidence < 0.7 {
			confidenceColor := color.New(color.FgYellow)
			if f.config.Color {
				confidenceColor.Fprintf(w, "    Confidence: %s\n", confidence)
			} else {
				fmt.Fprintf(w, "    Confidence: %s\n", confidence)
			}
		} else {
			fmt.Fprintf(w, "    Confidence: %s\n", confidence)
		}
	}

	fmt.Fprintf(w, "\n")
}

// writeFooter writes the report footer
func (f *TextFormatter) writeFooter(result *review.ReviewResult, w io.Writer) {
	width := 70
	separator := strings.Repeat("=", width)

	fmt.Fprintf(w, "%s\n", separator)

	// Exit code guidance
	if result.CriticalCount > 0 {
		fmt.Fprintf(w, "❌ Critical issues found. Exit code: 2\n")
	} else if result.WarningCount > 0 {
		fmt.Fprintf(w, "⚠️  Warnings found. Exit code: 1\n")
	} else {
		fmt.Fprintf(w, "✅ Review passed. Exit code: 0\n")
	}

	fmt.Fprintf(w, "%s\n", separator)
}

// formatSeverity formats a severity with appropriate color
func (f *TextFormatter) formatSeverity(severity review.Severity, w io.Writer) string {
	if !f.config.Color {
		return fmt.Sprintf("[%s]", strings.ToUpper(string(severity)))
	}

	var severityColor *color.Color
	switch severity {
	case review.SeverityCritical:
		severityColor = color.New(color.BgRed, color.FgWhite, color.Bold)
		return severityColor.Sprint(" CRITICAL ")
	case review.SeverityHigh:
		severityColor = color.New(color.BgYellow, color.FgBlack, color.Bold)
		return severityColor.Sprint(" WARNING  ")
	case review.SeverityInfo:
		severityColor = color.New(color.BgCyan, color.FgWhite)
		return severityColor.Sprint("   INFO   ")
	default:
		return fmt.Sprintf("[%s]", strings.ToUpper(string(severity)))
	}
}

// groupIssuesByFile groups issues by file
func (f *TextFormatter) groupIssuesByFile(result *review.ReviewResult) map[string]*review.FileReview {
	issuesByFile := make(map[string]*review.FileReview)

	for _, fileReview := range result.FileReviews {
		if len(fileReview.Issues) > 0 || f.config.ShowSuccess {
			issuesByFile[fileReview.File.Relative] = &fileReview
		}
	}

	return issuesByFile
}

// getSortedFiles returns files sorted based on configuration
func (f *TextFormatter) getSortedFiles(issuesByFile map[string]*review.FileReview) []string {
	files := make([]string, 0, len(issuesByFile))
	for file := range issuesByFile {
		files = append(files, file)
	}

	switch f.config.SortBy {
	case "severity":
		// Sort by highest severity issue in file
		sort.Slice(files, func(i, j int) bool {
			iIssues := issuesByFile[files[i]].Issues
			jIssues := issuesByFile[files[j]].Issues

			if len(iIssues) == 0 && len(jIssues) == 0 {
				return files[i] < files[j]
			}
			if len(iIssues) == 0 {
				return false
			}
			if len(jIssues) == 0 {
				return true
			}

			iMaxSeverity := f.getMaxSeverity(iIssues)
			jMaxSeverity := f.getMaxSeverity(jIssues)

			if iMaxSeverity == jMaxSeverity {
				return files[i] < files[j]
			}

			// Critical > Warning > Info
			severityOrder := map[review.Severity]int{
				review.SeverityCritical: 3,
				review.SeverityHigh:     2,
				review.SeverityInfo:     1,
			}

			return severityOrder[iMaxSeverity] > severityOrder[jMaxSeverity]
		})

	case "line":
		// Already sorted by file, line sorting happens within file
		sort.Strings(files)

	default: // "file" or any other value
		sort.Strings(files)
	}

	return files
}

// getMaxSeverity returns the highest severity in a list of issues
func (f *TextFormatter) getMaxSeverity(issues []review.Issue) review.Severity {
	maxSeverity := review.SeverityInfo
	for _, issue := range issues {
		switch issue.Severity {
		case review.SeverityCritical:
			return review.SeverityCritical
		case review.SeverityHigh:
			if maxSeverity == review.SeverityInfo {
				maxSeverity = review.SeverityHigh
			}
		}
	}
	return maxSeverity
}

// sortIssues sorts issues within a file
func (f *TextFormatter) sortIssues(issues []review.Issue) []review.Issue {
	sorted := make([]review.Issue, len(issues))
	copy(sorted, issues)

	switch f.config.SortBy {
	case "severity":
		sort.Slice(sorted, func(i, j int) bool {
			// Critical > Warning > Info
			severityOrder := map[review.Severity]int{
				review.SeverityCritical: 3,
				review.SeverityHigh:     2,
				review.SeverityInfo:     1,
			}

			if severityOrder[sorted[i].Severity] == severityOrder[sorted[j].Severity] {
				// Same severity, sort by line
				if sorted[i].Line == sorted[j].Line {
					return sorted[i].Title < sorted[j].Title
				}
				return sorted[i].Line < sorted[j].Line
			}

			return severityOrder[sorted[i].Severity] > severityOrder[sorted[j].Severity]
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
		// Default sort by severity then line
		sort.Slice(sorted, func(i, j int) bool {
			severityOrder := map[review.Severity]int{
				review.SeverityCritical: 3,
				review.SeverityHigh:     2,
				review.SeverityInfo:     1,
			}

			if severityOrder[sorted[i].Severity] == severityOrder[sorted[j].Severity] {
				if sorted[i].Line == sorted[j].Line {
					return sorted[i].Title < sorted[j].Title
				}
				return sorted[i].Line < sorted[j].Line
			}

			return severityOrder[sorted[i].Severity] > severityOrder[sorted[j].Severity]
		})
	}

	return sorted
}
