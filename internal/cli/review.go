package cli

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"scanr/internal/config"
	"scanr/internal/fs"
	"scanr/internal/git"
	"scanr/internal/output"
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
	files, _, err := getFilesToReview(ctx, cwd, languages, cfg)
	if err != nil {
		return 2, fmt.Errorf("failed to get files: %v", err)
	}

	if len(files) == 0 {
		log.Println("No files found to review")
		return 0, nil
	}

	log.Printf("Found %d file(s) to review", len(files))

	// Create mock reviewer for now
	// mockReviewer := reviewer.NewMockReviewer("scanr-mock")
	rev, err := createReviewer()
	if err != nil {
		return 2, fmt.Errorf("failed to create reviewer: %v", err)
	}

	log.Printf("Using reviewer: %s", rev.Name())

	// Create review pipeline
	pipeline, err := review.NewPipeline(review.DefaultConfig(), rev)
	if err != nil {
		return 2, fmt.Errorf("failed to create review pipeline: %v", err)
	}
	defer pipeline.Stop()

	// Run review
	filePointers := make([]*fs.FileInfo, len(files))
	for i := range files {
		filePointers[i] = &files[i]
	}

	result, err := pipeline.Run(ctx, filePointers)
	if err != nil {
		return 2, fmt.Errorf("review failed: %v", err)
	}

	// Create output formatter
	factory := output.NewFormatterFactory()
	formatter, err := factory.CreateFormatterFromFlags(cfg.Format, true)
	if err != nil {
		return 2, fmt.Errorf("failed to create formatter: %w", err)
	}

	// Format and display results
	if err := formatter.Format(result, os.Stdout); err != nil {
		return 2, fmt.Errorf("failed to format output: %w", err)
	}

	// Log AI usage stats if available
	if aiRev, ok := rev.(reviewer.AIReviewer); ok {
		usage := aiRev.GetUsage()
		log.Printf("AI Usage: %d requests, %d tokens, %v duration",
			usage.TotalRequests, usage.TotalTokens, usage.TotalDuration)
	}

	// Determine exit code
	exitCode := output.DetermineExitCode(result)

	return exitCode, nil
}

func createReviewer() (review.Reviewer, error) {
	aiConfig, err := config.LoadAIConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load AI config: %v", err)
	}

	// Check if API key is available
	if aiConfig.APIKey == "" {
		return nil, fmt.Errorf("AI API key is required. Set SCANR_AI_API_KEY environment variable or create ~/.scanr-ai.yaml configuration file")
	}

	// Create AI reviewer
	aiRev, err := reviewer.NewAIReviewer(aiConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create AI reviewer: %v", err)
	}

	// Validate configuration
	if err := aiRev.(reviewer.AIReviewer).ValidateConfig(); err != nil {
		return nil, fmt.Errorf("AI configuration invalid: %v", err)
	}

	log.Println("Using AI-powered code review")
	return aiRev, nil
}

// getFilesToReview gets files to review based on git status or full scan
func getFilesToReview(ctx context.Context, cwd string, languages []string, cfg *config.Config) ([]fs.FileInfo, *git.Repository, error) {
	// Detect git repository
	repo, err := git.DetectRepository(cwd)
	if err != nil {
		log.Printf("Warning: Not a git repository (%v), scanning all files", err)
		files, err := scanAllFiles(ctx, cwd, languages, cfg.MaxFiles)
		return files, nil, err
	}

	log.Printf("Found git repository at: %s", repo.Path)

	// Get git changes based on staged flag
	var changes []git.FileChange
	if cfg.StagedOnly {
		changes, err = repo.GetStagedChanges(ctx)
		if err != nil {
			return nil, repo, fmt.Errorf("failed to get staged changes: %v", err)
		}
		log.Printf("Found %d staged file(s)", len(changes))
	} else {
		changes, err = repo.GetAllChanges(ctx)
		if err != nil {
			return nil, repo, fmt.Errorf("failed to get changes: %v", err)
		}
		log.Printf("Found %d changed file(s)", len(changes))
	}

	// Filter changes by language
	files, err := filterAndConvertChanges(repo, changes, languages, cfg.MaxFiles)
	if err != nil {
		return nil, repo, fmt.Errorf("failed to process changes: %v", err)
	}

	return files, repo, nil
}

// scanAllFiles handles non-git repository scanning
func scanAllFiles(ctx context.Context, cwd string, languages []string, maxFiles int) ([]fs.FileInfo, error) {
	log.Println("Scanning all files (not a git repository)")

	scanner, err := fs.NewScanner(fs.Config{
		RootDir:     cwd,
		Languages:   languages,
		MaxFileSize: 1024 * 1024,
		MaxLines:    1000,
		IgnoreDirs:  []string{},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create scanner: %v", err)
	}

	return scanner.Scan(ctx, maxFiles)
}

// filterAndConvertChanges filters git changes by language and converts to FileInfo
func filterAndConvertChanges(repo *git.Repository, changes []git.FileChange, languages []string, maxFiles int) ([]fs.FileInfo, error) {
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
			continue
		}

		// Check size limit
		if info.Size() > 1024*1024 {
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
			Path:      fullPath,
			Size:      info.Size(),
			Lines:     lines,
			Languages: language,
			Relative:  change.Path,
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
