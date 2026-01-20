package tests

import (
	"context"
	"os"
	"path/filepath"
	"scanr/internal/cli"
	"scanr/internal/config"
	"testing"

)



func TestCLIWtithFilesystemScanning(t *testing.T) {
	testDir := t.TempDir()

	testFiles := []struct {
		path string
		content	string
	}{
        {"main.go", "package main\n\nfunc main() {\n    println(\"Hello\")\n}"},
        {"utils.go", "package main\n\nfunc helper() {\n    // do something\n}"},
        {"test.py", "def hello():\n    print(\"Hello\")"},
        {"ignore.js", "console.log('ignore')"},
        {"node_modules/package/index.js", "module.exports = {}"},
        {".gitignore", "*.js\nnode_modules/\n"},
	}

	for _, tf := range testFiles {
		path := filepath.Join(testDir, tf.path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(tf.content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// change to test directory
	oldCwd, err := os.Getwd()
	if err != nil {
        t.Fatal(err)
    }
	defer os.Chdir(oldCwd)

	if err := os.Chdir(testDir); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name		string
		cfg			*config.Config
		wantFiles	int
		wantErr		bool
	}{
		{
			name: "scan Go files",
			 cfg: &config.Config{
                Languages:  "go",
                StagedOnly: false,
                MaxFiles:   10,
                Format:     "text",
            },
            wantFiles: 2,
            wantErr:   false,
		},
		{
            name: "scan Go and Python files",
            cfg: &config.Config{
                Languages:  "go,python",
                StagedOnly: false,
                MaxFiles:   10,
                Format:     "text",
            },
            wantFiles: 3,
            wantErr:   false,
        },
		{
            name: "respect max files limit",
            cfg: &config.Config{
                Languages:  "go",
                StagedOnly: false,
                MaxFiles:   1,
                Format:     "text",
            },
            wantFiles: 1,
            wantErr:   false,
        },
		{
            name: "ignore JavaScript files via .gitignore",
            cfg: &config.Config{
                Languages:  "javascript",
                StagedOnly: false,
                MaxFiles:   10,
                Format:     "text",
            },
            wantFiles: 0,
            wantErr:   false,
        },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T){
			ctx := context.Background()

			exitCode, err := cli.RunReview(ctx, tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, go nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if exitCode != 0 {
				t.Errorf("exected exit code 0, got %d", exitCode)
			}

		})
	}

}