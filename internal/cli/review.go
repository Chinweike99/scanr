package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"scanr/internal/config"
	"scanr/internal/fs"
)

/**
* RunReview is the main entry point for the review command
 */
func RunReview(ctx context.Context, cfg *config.Config) (int, error) {
	languages, err := ParseLanguages(cfg.Languages)
	if err != nil {
		return 2, fmt.Errorf("failed to parse language: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return 2, fmt.Errorf("failed to get current working directory: %v", err)
	}

	scanner, err := fs.NewScanner(fs.Config{
		RootDir:     cwd,
		Languages:   languages,
		MaxFileSize: 1024 * 1024,
		MaxLines:    1000,
		IgnoreDirs:  []string{},
	})

	if err != nil {
		return 2, fmt.Errorf("Failed to create scanner: %v", err)
	}

	// Scan for files
	files, err := scanner.Scan(ctx, cfg.MaxFiles)
	if err != nil {
		return 2, fmt.Errorf("failed to scan files: %v", err)
	}

	// Create review options
	opts := &config.ReviewOptions{
		Languages:   languages,
		StagedOnly:  cfg.StagedOnly,
		MaxFiles:    cfg.MaxFiles,
		Format:      cfg.Format,
		Interactive: cfg.Languages == "",
		Files:       files,
	}

	// This would be replaced with actual review pipeline in later phases
	fmt.Printf("Review configuration:\n")
	fmt.Printf("  Languages: %v\n", opts.Languages)
	fmt.Printf("  Staged only: %v\n", opts.StagedOnly)
	fmt.Printf("  Max files: %d\n", opts.MaxFiles)
	fmt.Printf("  Format: %s\n", opts.Format)
	fmt.Printf("  Interactive: %v\n", opts.Interactive)

	displayFoundFiles(files, cfg.Format)

	// This will be determined by actual review results in later phases
	return 0, nil
}

func displayFoundFiles(files []fs.FileInfo, format string) {
	if len(files) == 0 {
		log.Println("No files found to review")
		return
	}

	switch format {
	case "json":
		fmt.Println("[")
		for i, file := range files {
			fmt.Printf("  {\n")
			fmt.Printf("    \"path\": %q,\n", file.Path)
			fmt.Printf("    \"relative\": %q,\n", file.Relative)
			fmt.Printf("    \"size\": %d,\n", file.Size)
			fmt.Printf("    \"lines\": %d,\n", file.Lines)
			fmt.Printf("    \"language\": %q\n", file.Languages)
			fmt.Printf("  }")
			if i < len(files)-1 {
				fmt.Println(",")
			} else {
				fmt.Println()
			}
		}
		fmt.Println("]")
	default:
		fmt.Printf("Found %d file(s) to review:\n", len(files))
		for _, file := range files {
			size := formatFileSize(file.Size)
			fmt.Printf("  %s (%s, %d lines, %s)\n",
				file.Relative, file.Languages, file.Lines, size)
		}
	}
}

func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
