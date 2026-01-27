package cli

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"scanr/internal/config"
	"scanr/internal/fs"
	"scanr/internal/git"
	"scanr/internal/review"
	"scanr/pkg/reviewer"
)

// RunReview is the main entry point for the review command
func RunReview(ctx context.Context, cfg *config.Config) (int, error) {
	// Parse or prompt for languages
	languages, err := ParseLanguages(cfg.Languages)
	if err != nil {
		return 2, fmt.Errorf("failed to parse languages: %v", err)
	}

	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return 2, fmt.Errorf("failed to get current directory: %v", err)
	}

	// Get files to review
	files, err := getFilesToReview(ctx, cwd, languages, cfg)
	if err != nil {
		return 2, fmt.Errorf("failed to get files: %v", err)
	}

	if len(files) == 0 {
		log.Println("No files found to review")
		return 0, nil
	}

	log.Printf("Found %d file(s) to review", len(files))

	// Convert to pointer slice
	filePointers := make([]*fs.FileInfo, len(files))
	for i := range files {
		filePointers[i] = &files[i]
	}

	// Create mock reviewer for now
	mockReviewer := reviewer.NewMockReviewer("scanr-mock")

	// Create review pipeline
	pipeline, err := review.NewPipeline(review.DefaultConfig(), mockReviewer)
	if err != nil {
		return 2, fmt.Errorf("failed to create review pipeline: %v", err)
	}
	defer pipeline.Stop()

	// Run review
	result, err := pipeline.Run(ctx, filePointers)
	if err != nil {
		return 2, fmt.Errorf("review failed: %v", err)
	}

	// Display results
	displayReviewResults(result, cfg.Format)

	// Determine exit code
	exitCode := determineExitCode(result)

	return exitCode, nil
}

// getFilesToReview gets files to review based on git status or full scan
func getFilesToReview(ctx context.Context, cwd string, languages []string, cfg *config.Config) ([]fs.FileInfo, error) {
	// Detect git repository
	repo, err := git.DetectRepository(cwd)
	if err != nil {
		log.Printf("Warning: Not a git repository (%v), scanning all files", err)
		files, err := scanAllFiles(ctx, cwd, languages, cfg.MaxFiles)
		return files, err
	}

	log.Printf("Found git repository at: %s", repo.Path)

	// Get git changes based on staged flag
	var changes []git.FileChange
	if cfg.StagedOnly {
		changes, err = repo.GetStagedChanges(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get staged changes: %v", err)
		}
		log.Printf("Found %d staged file(s)", len(changes))
	} else {
		changes, err = repo.GetAllChanges(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get changes: %v", err)
		}
		log.Printf("Found %d changed file(s)", len(changes))
	}

	// Filter changes by language
	files, err := filterAndConvertChanges(repo, changes, languages, cfg.MaxFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to process changes: %v", err)
	}

	return files, nil
}

// scanAllFiles handles non-git repository scanning
func scanAllFiles(ctx context.Context, cwd string, languages []string, maxFiles int) ([]fs.FileInfo, error) {
	log.Println("Scanning all files (not a git repository)")

	// Create filesystem scanner
	scanner, err := fs.NewScanner(fs.Config{
		RootDir:     cwd,
		Languages:   languages,
		MaxFileSize: 1024 * 1024, // 1MB
		MaxLines:    1000,
		IgnoreDirs:  []string{},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create scanner: %v", err)
	}

	// Scan for files
	return scanner.Scan(ctx, maxFiles)
}

// filterAndConvertChanges filters git changes by language and converts to FileInfo
func filterAndConvertChanges(repo *git.Repository, changes []git.FileChange, languages []string, maxFiles int) ([]fs.FileInfo, error) {
	// Build language extensions map for filtering
	langExts := make(map[string]bool)
	for _, lang := range languages {
		exts, ok := fs.SupportedExtensions[lang]
		if !ok {
			continue
		}
		for _, ext := range exts {
			langExts[ext] = true
		}
	}

	var files []fs.FileInfo
	fileCount := 0

	for _, change := range changes {
		// Skip deleted files
		if change.ChangeType == git.ChangeDeleted {
			continue
		}

		// Check file extension
		ext := strings.ToLower(filepath.Ext(change.Path))
		if !langExts[ext] {
			continue
		}

		// Get file info
		fullPath := filepath.Join(repo.Path, change.Path)
		info, err := os.Stat(fullPath)
		if err != nil {
			// File might not exist (e.g., for staged deletions)
			continue
		}

		// Check size limit
		if info.Size() > 1024*1024 { // 1MB
			continue
		}

		// Count lines
		lines, err := countFileLines(fullPath)
		if err != nil {
			continue
		}

		// Check line limit
		if lines > 1000 {
			continue
		}

		// Determine language from extension
		language := ""
		for lang, exts := range fs.SupportedExtensions {
			for _, e := range exts {
				if ext == e {
					language = lang
					break
				}
			}
			if language != "" {
				break
			}
		}

		if language == "" {
			continue
		}

		files = append(files, fs.FileInfo{
			Path:     fullPath,
			Size:     info.Size(),
			Lines:    lines,
			Relative: change.Path,
		})

		fileCount++
		if maxFiles > 0 && fileCount >= maxFiles {
			break
		}
	}

	return files, nil
}

// countFileLines counts lines in a file
func countFileLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
		if count > 1000 {
			break
		}
	}

	return count, scanner.Err()
}

// displayReviewResults displays the review results
func displayReviewResults(result *review.ReviewResult, format string) {
	if format == "json" {
		displayJSONResults(result)
	} else {
		displayTextResults(result)
	}
}

// displayJSONResults displays results in JSON format
func displayJSONResults(result *review.ReviewResult) {
	// Simple JSON output for now
	// In Phase 5, we'll implement proper JSON marshaling
	fmt.Printf("{\n")
	fmt.Printf("  \"total_files\": %d,\n", result.TotalFiles)
	fmt.Printf("  \"reviewed_files\": %d,\n", result.ReviewedFiles)
	fmt.Printf("  \"total_issues\": %d,\n", result.TotalIssues)
	fmt.Printf("  \"critical_count\": %d,\n", result.CriticalCount)
	fmt.Printf("  \"warning_count\": %d,\n", result.WarningCount)
	fmt.Printf("  \"info_count\": %d,\n", result.InfoCount)
	fmt.Printf("  \"duration_ms\": %.0f\n", result.Duration.Seconds()*1000)
	fmt.Printf("}\n")
}

// displayTextResults displays results in text format
func displayTextResults(result *review.ReviewResult) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("scanr REVIEW RESULTS")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Files reviewed: %d/%d\n", result.ReviewedFiles, result.TotalFiles)
	fmt.Printf("  Total issues: %d\n", result.TotalIssues)
	fmt.Printf("  Critical: %d\n", result.CriticalCount)
	fmt.Printf("  Warnings: %d\n", result.WarningCount)
	fmt.Printf("  Info: %d\n", result.InfoCount)
	fmt.Printf("  Duration: %v\n", result.Duration.Round(time.Millisecond))

	if result.TotalIssues > 0 {
		fmt.Println("\nIssues by file:")
		for _, fileReview := range result.FileReviews {
			if len(fileReview.Issues) > 0 {
				fmt.Printf("\n%s:\n", fileReview.File.Relative)
				for _, issue := range fileReview.Issues {
					severityColor := getSeverityColor(issue.Severity)
					fmt.Printf("  [%s] %s", severityColor, issue.Title)
					if issue.Line > 0 {
						fmt.Printf(" (line %d)", issue.Line)
					}
					fmt.Println()
					fmt.Printf("      %s\n", issue.Description)
					if len(issue.Suggestions) > 0 {
						fmt.Printf("      Suggestions:\n")
						for _, suggestion := range issue.Suggestions {
							fmt.Printf("      - %s\n", suggestion)
						}
					}
				}
			}
		}
	} else {
		fmt.Println("\n✅ No issues found!")
	}

	fmt.Println(strings.Repeat("=", 60))
}

// getSeverityColor returns a colored string for severity
func getSeverityColor(severity review.Severity) string {
	switch severity {
	case review.SeverityCritical:
		return "🔴 CRITICAL"
	case review.SeverityHigh:
		return "🟡 WARNING"
	case review.SeverityInfo:
		return "🔵 INFO"
	default:
		return string(severity)
	}
}

// determineExitCode determines the exit code based on review results
func determineExitCode(result *review.ReviewResult) int {
	if result.CriticalCount > 0 {
		return 2
	}
	if result.WarningCount > 0 {
		return 1
	}
	return 0
}
