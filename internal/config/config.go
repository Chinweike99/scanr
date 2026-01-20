package config

import (
	"fmt"
	"scanr/internal/fs"
	"strings"
)

type Config struct {
	Languages  string
	StagedOnly bool
	MaxFiles   int
	Format     string
}

type ReviewOptions struct {
	Languages   []string
	StagedOnly  bool
	MaxFiles    int
	Format      string
	Interactive bool
	Files       []fs.FileInfo
}

// ValidateConfig validates the configuration values
func ValidateConfig(cfg *Config) error {
	// Validate format
	format := strings.ToLower(cfg.Format)
	if format != "text" && format != "json" {
		return fmt.Errorf("format must be 'text' or 'json', got %q", cfg.Format)
	}

	// Validate max files
	if cfg.MaxFiles <= 0 {
		return fmt.Errorf("max-files must be positive, got %d", cfg.MaxFiles)
	}

	return nil
}
