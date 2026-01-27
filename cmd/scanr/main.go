package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"scanr/internal/cli"
	"scanr/internal/config"
	"strings"
)

func main() {
	ctx := context.Background()

	// Define CLI flag
	langFlag := flag.String("lang", "", "Comma-separated language names to review (go,java,typescript,etc)")
	stagedFlag := flag.Bool("staged", true, "Review only staged changes")
	maxFilesFlag := flag.Int("max-files", 100, "Maximum number of files to review")
	formatFlag := flag.String("format", "text", "Output format: text or json")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExit codes:\n")
		fmt.Fprintf(os.Stderr, "	0 - No issues found\n")
		fmt.Fprintf(os.Stderr, "	1 - Warnings found\n")
		fmt.Fprintf(os.Stderr, "	2 - Critical issues found\n")
	}

	flag.Parse()

	// Create config
	cfg := &config.Config{
		Languages:  *langFlag,
		StagedOnly: *stagedFlag,
		MaxFiles:   *maxFilesFlag,
		Format:     strings.ToLower(*formatFlag),
	}

	// Validate config
	if err := config.ValidateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	// Run the code review command
	exitCode, err := cli.RunReview(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if exitCode == 0 {
			exitCode = 2
		}
	}
	os.Exit(exitCode)
}
