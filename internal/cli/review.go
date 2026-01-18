package cli

import (
	"context"
	"fmt"
	"scanr/internal/config"
)

/**
* RunReview is the main entry point for the review command
*/

func RunReview(ctx context.Context, cfg *config.Config) (int, error){
	languages, err := ParseLanguages(cfg.Languages)
	if err != nil {
		return 2, fmt.Errorf("Failed to parse language: %v", err)
	}

	// Create review options
	opts := &config.ReviewOptions{
		Languages: languages,
		StagedOnly: cfg.StagedOnly,
		MaxFiles: cfg.MaxFiles,
		Format: cfg.Format,
		Interactive: cfg.Languages == "",
	}

	// This would be replaced with actual review pipeline in later phases
	fmt.Printf("Review configuration:\n")
    fmt.Printf("  Languages: %v\n", opts.Languages)
    fmt.Printf("  Staged only: %v\n", opts.StagedOnly)
    fmt.Printf("  Max files: %d\n", opts.MaxFiles)
    fmt.Printf("  Format: %s\n", opts.Format)
    fmt.Printf("  Interactive: %v\n", opts.Interactive)

	// This will be determined by actual review results in later phases
    return 0, nil
}