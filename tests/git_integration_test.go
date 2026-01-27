package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"scanr/internal/cli"
	"scanr/internal/config"
	"testing"
)

func TestCLIWithGitIntegration(t *testing.T) {
    // Create a test directory and initialize git
    testDir := t.TempDir()
    
    // Initialize git repository
    cmd := exec.Command("git", "init")
    cmd.Dir = testDir
    if err := cmd.Run(); err != nil {
        t.Fatalf("failed to init git repository: %v", err)
    }
    
    // Configure git user
    cmds := []*exec.Cmd{
        exec.Command("git", "config", "user.email", "test@example.com"),
        exec.Command("git", "config", "user.name", "Test User"),
    }
    
    for _, cmd := range cmds {
        cmd.Dir = testDir
        if err := cmd.Run(); err != nil {
            t.Fatalf("failed to configure git: %v", err)
        }
    }
    
    // Create test files
    testFiles := []struct {
        path    string
        content string
        stage   bool
    }{
        {"staged.go", "package main\n\nfunc staged() {}", true},
        {"unstaged.go", "package main\n\nfunc unstaged() {}", false},
        {"staged.py", "def staged_func():\n    pass", true},
        {"unstaged.py", "def unstaged_func():\n    pass", false},
        {"ignored.js", "console.log('ignore')", false},
    }
    
    for _, tf := range testFiles {
        path := filepath.Join(testDir, tf.path)
        if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
            t.Fatal(err)
        }
        if err := os.WriteFile(path, []byte(tf.content), 0644); err != nil {
            t.Fatal(err)
        }
        
        if tf.stage {
            cmd := exec.Command("git", "add", tf.path)
            cmd.Dir = testDir
            if err := cmd.Run(); err != nil {
                t.Fatal(err)
            }
        }
    }
    
    // Create .gitignore
    gitignore := "*.js\n"
    if err := os.WriteFile(filepath.Join(testDir, ".gitignore"), 
        []byte(gitignore), 0644); err != nil {
        t.Fatal(err)
    }
    
    // Change to test directory
    oldCwd, err := os.Getwd()
    if err != nil {
        t.Fatal(err)
    }
    defer os.Chdir(oldCwd)
    
    if err := os.Chdir(testDir); err != nil {
        t.Fatal(err)
    }
    
    tests := []struct {
        name       string
        cfg        *config.Config
        wantErr    bool
    }{
        {
            name: "staged changes only",
            cfg: &config.Config{
                Languages:  "go,python",
                StagedOnly: true,
                MaxFiles:   10,
                Format:     "text",
            },
            wantErr: false,
        },
        {
            name: "all changes",
            cfg: &config.Config{
                Languages:  "go,python",
                StagedOnly: false,
                MaxFiles:   10,
                Format:     "text",
            },
            wantErr: false,
        },
        {
            name: "only Go files",
            cfg: &config.Config{
                Languages:  "go",
                StagedOnly: false,
                MaxFiles:   10,
                Format:     "text",
            },
            wantErr: false,
        },
        {
            name: "max files limit",
            cfg: &config.Config{
                Languages:  "go,python",
                StagedOnly: false,
                MaxFiles:   1,
                Format:     "text",
            },
            wantErr: false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx := context.Background()
            
            exitCode, err := cli.RunReview(ctx, tt.cfg)
            
            if tt.wantErr {
                if err == nil {
                    t.Error("expected error, got nil")
                }
                return
            }
            
            if err != nil {
                t.Errorf("unexpected error: %v", err)
                return
            }
            
            if exitCode != 0 {
                t.Errorf("expected exit code 0, got %d", exitCode)
            }
        })
    }
}

func TestCLIWithoutGitRepository(t *testing.T) {
    // Create a test directory without git
    testDir := t.TempDir()
    
    // Create some test files
    files := []struct {
        path    string
        content string
    }{
        {"file1.go", "package main\n\nfunc one() {}"},
        {"file2.py", "def two():\n    pass"},
        {"subdir/file3.go", "package main\n\nfunc three() {}"},
    }
    
    for _, f := range files {
        path := filepath.Join(testDir, f.path)
        if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
            t.Fatal(err)
        }
        if err := os.WriteFile(path, []byte(f.content), 0644); err != nil {
            t.Fatal(err)
        }
    }
    
    // Change to test directory
    oldCwd, err := os.Getwd()
    if err != nil {
        t.Fatal(err)
    }
    defer os.Chdir(oldCwd)
    
    if err := os.Chdir(testDir); err != nil {
        t.Fatal(err)
    }
    
    ctx := context.Background()
    cfg := &config.Config{
        Languages:  "go,python",
        StagedOnly: true,
        MaxFiles:   10,
        Format:     "text",
    }
    
    exitCode, err := cli.RunReview(ctx, cfg)
    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
    
    // Exit code 0 means no issues, 1 means warnings, 2 means critical
    // All are valid for this test as long as no error occurred
    if exitCode < 0 || exitCode > 2 {
        t.Errorf("unexpected exit code: %d", exitCode)
    }
}