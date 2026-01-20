package fs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNewScanner(t *testing.T) {
    tests := []struct {
        name      string
        config    Config
        wantErr   bool
        checkFunc func(*testing.T, *Scanner)
    }{
        {
            name: "valid config with root dir",
            config: Config{
                RootDir:     t.TempDir(),
                Languages:   []string{"go", "python"},
                MaxFileSize: 1024,
                MaxLines:    100,
            },
            wantErr: false,
            checkFunc: func(t *testing.T, s *Scanner) {
                if s.rootDir == "" {
                    t.Error("rootDir should be set")
                }
                if len(s.languages) != 2 {
                    t.Errorf("expected 2 languages, got %d", len(s.languages))
                }
                if s.maxFileSize != 1024 {
                    t.Errorf("expected maxFileSize 1024, got %d", s.maxFileSize)
                }
            },
        },
        {
            name: "empty root dir uses current dir",
            config: Config{
                Languages: []string{"go"},
            },
            wantErr: false,
            checkFunc: func(t *testing.T, s *Scanner) {
                if s.rootDir == "" {
                    t.Error("rootDir should be set")
                }
            },
        },
        {
            name: "invalid root dir",
            config: Config{
                RootDir:   "/nonexistent/path/123456",
                Languages: []string{"go"},
            },
            wantErr: true,
        },
        {
            name: "no languages",
            config: Config{
                RootDir: t.TempDir(),
            },
            wantErr: true,
        },
        {
            name: "unsupported language",
            config: Config{
                RootDir:   t.TempDir(),
                Languages: []string{"go", "invalid"},
            },
            wantErr: true,
        },
        {
            name: "default values",
            config: Config{
                RootDir:   t.TempDir(),
                Languages: []string{"go"},
            },
            wantErr: false,
            checkFunc: func(t *testing.T, s *Scanner) {
                if s.maxFileSize != DefaultMaxFileSize {
                    t.Errorf("expected default maxFileSize %d, got %d", DefaultMaxFileSize, s.maxFileSize)
                }
                if s.maxLines != DefaultMaxLines {
                    t.Errorf("expected default maxLines %d, got %d", DefaultMaxLines, s.maxLines)
                }
                for _, dir := range DefaultIgnoreDirs {
                    if !s.ignoreDirs[dir] {
                        t.Errorf("missing default ignore dir: %s", dir)
                    }
                }
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            scanner, err := NewScanner(tt.config)
            
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
            
            if tt.checkFunc != nil {
                tt.checkFunc(t, scanner)
            }
        })
    }
}

func TestScanner_Scan(t *testing.T) {
    ctx := context.Background()
    testDir := CreateTempTestDir(t)
    CreateTestDirStructure(t, testDir)
    
    scanner, err := NewScanner(Config{
        RootDir:     testDir,
        Languages:   []string{"go", "python"},
        MaxFileSize: 5000,
        MaxLines:    100,
    })
    if err != nil {
        t.Fatal(err)
    }
    
    files, err := scanner.Scan(ctx, 10)
    if err != nil {
        t.Fatalf("Scan failed: %v", err)
    }
    
    // Verify results
    if len(files) == 0 {
        t.Fatal("expected to find files, got none")
    }
    
    // Should find Go and Python files, but not:
    // - node_modules (ignored dir)
    // - vendor (ignored dir)
    // - binary.data (.gitignore pattern)
    // - large.go (too many lines)
    // - .git directory (ignored)
    
    foundGo := false
    foundPython := false
    foundIgnored := false
    
    for _, file := range files {
        switch filepath.Ext(file.Path) {
        case ".go":
            foundGo = true
            if file.Languages != "go" {
                t.Errorf("file %s should have language 'go', got %s", file.Path, file.Languages)
            }
        case ".py":
            foundPython = true
            if file.Languages != "python" {
                t.Errorf("file %s should have language 'python', got %s", file.Path, file.Languages)
            }
        default:
            foundIgnored = true
        }
        
        if file.Relative == "" {
            t.Errorf("file %s missing relative path", file.Path)
        }
    }
    
    if !foundGo {
        t.Error("did not find Go files")
    }
    if !foundPython {
        t.Error("did not find Python files")
    }
    if foundIgnored {
        t.Error("found files that should have been ignored")
    }
}

func TestScanner_ScanWithMaxFiles(t *testing.T) {
    ctx := context.Background()
    testDir := CreateTempTestDir(t)
    
    // Create many Go files
    for i := 0; i < 15; i++ {
        path := filepath.Join(testDir, fmt.Sprintf("file%d.go", i))
        content := fmt.Sprintf("package main\n\n// File %d\n", i)
        if err := os.WriteFile(path, []byte(content), 0644); err != nil {
            t.Fatal(err)
        }
    }
    
    scanner, err := NewScanner(Config{
        RootDir:   testDir,
        Languages: []string{"go"},
    })
    if err != nil {
        t.Fatal(err)
    }
    
    // Test with limit
    files, err := scanner.Scan(ctx, 5)
    if err != nil {
        t.Fatalf("Scan failed: %v", err)
    }
    
    if len(files) != 5 {
        t.Errorf("expected 5 files with limit, got %d", len(files))
    }
    
    // Test without limit
    files, err = scanner.Scan(ctx, 0)
    if err != nil {
        t.Fatalf("Scan failed: %v", err)
    }
    
    if len(files) != 15 {
        t.Errorf("expected 15 files without limit, got %d", len(files))
    }
}

func TestScanner_ContextCancellation(t *testing.T) {
    testDir := CreateTempTestDir(t)
    
    // Create many files to ensure scanning takes time
    for i := 0; i < 100; i++ {
        path := filepath.Join(testDir, fmt.Sprintf("file%d.go", i))
        content := fmt.Sprintf("package main\n\n// File %d\n", i)
        if err := os.WriteFile(path, []byte(content), 0644); err != nil {
            t.Fatal(err)
        }
    }
    
    scanner, err := NewScanner(Config{
        RootDir:   testDir,
        Languages: []string{"go"},
    })
    if err != nil {
        t.Fatal(err)
    }
    
    // Create a cancellable context
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    
    files, err := scanner.Scan(ctx, 100)
    if err == nil {
        t.Error("expected error due to context cancellation")
    }
    if len(files) > 0 {
        t.Errorf("expected no files due to cancellation, got %d", len(files))
    }
}

func TestGetLanguageForExtension(t *testing.T) {
    scanner := &Scanner{
        languages: map[string][]string{
            "go": {".go"},
            "python": {".py"},
        },
    }
    
    tests := []struct {
        ext      string
        expected string
    }{
        {".go", "go"},
        {".py", "python"},
        {".js", ""},
        {".", ""},
        {"", ""},
    }
    
    for _, tt := range tests {
        t.Run(tt.ext, func(t *testing.T) {
            result := scanner.getLanguageForExtension(tt.ext)
            if result != tt.expected {
                t.Errorf("getLanguageForExtension(%q) = %q, want %q", tt.ext, result, tt.expected)
            }
        })
    }
}

func TestCountLines(t *testing.T) {
    scanner := &Scanner{}
    
    tests := []struct {
        name     string
        content  string
        expected int
	}{
        {"empty", "", 0},
        {"single line", "Hello", 1},
        {"multiple lines", "Line1\nLine2\nLine3", 3},
        {"trailing newline", "Line1\nLine2\n", 2},
        {"empty lines", "\n\n\n", 3},
        {"mixed", "Line1\n\nLine3\n", 3},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            testDir := CreateTempTestDir(t)
            testFile := filepath.Join(testDir, "test.txt")
            
            if err := os.WriteFile(testFile, []byte(tt.content), 0644); err != nil {
                t.Fatal(err)
            }
            
            lines, err := scanner.countLines(testFile)
            if err != nil {
                t.Errorf("countLines failed: %v", err)
            }
            
            if lines != tt.expected {
                t.Errorf("countLines() = %d, want %d", lines, tt.expected)
            }
        })
    }
}

func TestCountLinesFromReader(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected int
        wantErr  bool
    }{
        {"empty", "", 0, false},
        {"single line", "Hello", 1, false},
        {"multiple lines", "Line1\nLine2\nLine3", 3, false},
        {"with carriage return", "Line1\r\nLine2\r\n", 2, false},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            r := bytes.NewReader([]byte(tt.input))
            lines, err := countLinesFromReader(r)
            
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
            
            if lines != tt.expected {
                t.Errorf("CountLinesFromReader() = %d, want %d", lines, tt.expected)
            }
        })
    }
}

func TestShouldIgnore(t *testing.T) {
    testDir := CreateTempTestDir(t)
    scanner := &Scanner{rootDir: testDir}
    
    patterns := []string{
        "*.log",
        "node_modules/*",
        "dist/",
        "**/temp/*",
    }
    
    tests := []struct {
        path     string
        expected bool
    }{
        {"file.log", true},
        {"node_modules/package/index.js", true},
        {"dist/app.js", true},
        {"src/temp/file.txt", true},
        {"src/main/app.go", false},
        {"test.log.txt", false},
        {"temp/main.go", false},
    }
    
    for _, tt := range tests {
        t.Run(tt.path, func(t *testing.T) {
            fullPath := filepath.Join(testDir, tt.path)
            result := scanner.shouldIgnore(fullPath, patterns)
            if result != tt.expected {
                t.Errorf("shouldIgnore(%q) = %v, want %v", tt.path, result, tt.expected)
            }
        })
    }
}