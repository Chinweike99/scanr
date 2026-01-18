package tests

import (
	"context"
	"flag"
	"scanr/internal/cli"
	"scanr/internal/config"
	"strings"
	"testing"
)

func TestCLIFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantLang   string
		wantStaged bool
		wantMax    int
		wantFormat string
		wantErr    bool
	}{
		{
			name:       "all flags provided",
			args:       []string{"--lang=go,python", "--staged=false", "--max-files=50", "--format=json"},
			wantLang:   "go,python",
			wantStaged: false,
			wantMax:    50,
			wantFormat: "json",
			wantErr:    false,
		},
		{
			name:       "default values",
			args:       []string{"--lang=go"},
			wantLang:   "go",
			wantStaged: true,
			wantMax:    100,
			wantFormat: "text",
			wantErr:    false,
		},
		{
			name:    "invalid format",
			args:    []string{"--lang=go", "--format=xml"},
			wantErr: true,
		},
		{
			name:    "invalid max files",
			args:    []string{"--lang=go", "--max-files=0"},
			wantErr: true,
		},
		{
			name:    "negative max files",
			args:    []string{"--lang=go", "--max-files=-1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flag.CommandLine to avoid test pollution
			flag.CommandLine = flag.NewFlagSet(t.Name(), flag.ContinueOnError)

			// Test flag parsing by creating a minimal main
			langFlag := flag.String("lang", "", "")
			stagedFlag := flag.Bool("staged", true, "")
			maxFilesFlag := flag.Int("max-files", 100, "")
			formatFlag := flag.String("format", "text", "")

			err := flag.CommandLine.Parse(tt.args)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("unexpected parse error for args %v: %v", tt.args, err)
				}
				return
			}

			// Create config and validate
			cfg := &config.Config{
				Languages:  *langFlag,
				StagedOnly: *stagedFlag,
				MaxFiles:   *maxFilesFlag,
				Format:     strings.ToLower(*formatFlag),
			}

			validateErr := config.ValidateConfig(cfg)

			if tt.wantErr {
				if validateErr == nil {
					t.Errorf("expected error for args %v, got none", tt.args)
				}
				return
			}

			if validateErr != nil {
				t.Errorf("unexpected validation error for args %v: %v", tt.args, validateErr)
				return
			}

			if *langFlag != tt.wantLang {
				t.Errorf("lang flag = %q, want %q", *langFlag, tt.wantLang)
			}
			if *stagedFlag != tt.wantStaged {
				t.Errorf("staged flag = %v, want %v", *stagedFlag, tt.wantStaged)
			}
			if *maxFilesFlag != tt.wantMax {
				t.Errorf("max-files flag = %d, want %d", *maxFilesFlag, tt.wantMax)
			}
			if strings.ToLower(*formatFlag) != tt.wantFormat {
				t.Errorf("format flag = %q, want %q", *formatFlag, tt.wantFormat)
			}
		})
	}
}

func TestRunReview(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		wantCode int
		wantErr  bool
	}{
		{
			name: "valid config with languages",
			cfg: &config.Config{
				Languages:  "go,python",
				StagedOnly: true,
				MaxFiles:   100,
				Format:     "text",
			},
			wantCode: 0,
			wantErr:  false,
		},
		{
			name: "invalid language",
			cfg: &config.Config{
				Languages:  "invalid",
				StagedOnly: true,
				MaxFiles:   100,
				Format:     "text",
			},
			wantCode: 2,
			wantErr:  true,
		},
		{
			name: "empty language triggers interactive prompt",
			cfg: &config.Config{
				Languages:  "",
				StagedOnly: true,
				MaxFiles:   100,
				Format:     "text",
			},
			// Should attempt to prompt (will fail in test without stdin)
			wantCode: 2,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// For the interactive prompt case, we can't test easily without mocking stdin
			// This test will verify the non-interactive cases work
			if tt.cfg.Languages != "" {
				exitCode, err := cli.RunReview(ctx, tt.cfg)

				if tt.wantErr {
					if err == nil {
						t.Errorf("RunReview() expected error, got nil")
					}
				} else {
					if err != nil {
						t.Errorf("RunReview() unexpected error: %v", err)
					}
				}

				if exitCode != tt.wantCode {
					t.Errorf("RunReview() exit code = %d, want %d", exitCode, tt.wantCode)
				}
			}
		})
	}
}
