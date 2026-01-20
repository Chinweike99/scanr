package fs

import (
	"os"
	"path/filepath"
	"testing"
)

// CreateTestDirStructure creates a test directory structure
func CreateTestDirStructure(t *testing.T, baseDir string) {
	t.Helper()

	dirs := []string{
		"src/main",
		"src/test",
		"node_modules/core",
		"vendor/pkg",
		".git/refs",
		"__pycache__",
	}

	files := map[string]string{
		"main.go":                    "package main\n\nfunc main() {\n    println(\"Hello\")\n}",
		"src/main/app.go":            "package main\n\nfunc App() string {\n    return \"app\"\n}",
		"src/main/app_test.go":       "package main\n\nimport \"testing\"\n\nfunc TestApp(t *testing.T) {}",
		"src/main/utils.py":          "def hello():\n    print(\"Hello\")\n",
		"src/main/large.go":          "package main\n\n" + repeatLines("// line\n", 2000),
		"src/main/binary.data":       "binary content",
		"node_modules/core/index.js": "module.exports = {}",
		"vendor/pkg/lib.go":          "package pkg\n\nfunc Lib() {}",
		".git/config":                "[core]",
		".gitignore":                 "*.log\nnode_modules/\nvendor/\n*.data\n",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(baseDir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}

	for path, content := range files {
		fullPath := filepath.Join(baseDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// repeatLines repeats a line n times
func repeatLines(line string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += line
	}
	return result
}

// CreateTempTestDir creates a temporary directory for testing
func CreateTempTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "preflight-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}
