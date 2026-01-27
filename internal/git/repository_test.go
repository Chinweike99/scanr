package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupTestRepository(t *testing.T) string {
	t.Helper()

	// Create a test directory
	testDir := t.TempDir()

	// Initialize git repository
	cmd := exec.Command("git", "init")
	cmd.Dir = testDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repository: %v", err)
	}

	// Configure git user for commits
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

	return testDir
}

func TestDetectRepository(t *testing.T) {
	// Create a test repository
	testDir := setupTestRepository(t)

	// Test detection from repository root
	repo, err := DetectRepository(testDir)
	if err != nil {
		t.Fatalf("failed to detect repository: %v", err)
	}

	if repo.Path != testDir {
		t.Errorf("expected path %s, got %s", testDir, repo.Path)
	}

	if repo.GitDir != filepath.Join(testDir, ".git") {
		t.Errorf("expected git dir %s, got %s",
			filepath.Join(testDir, ".git"), repo.GitDir)
	}

	if repo.IsBare {
		t.Error("expected non-bare repository")
	}

	// Test detection from subdirectory
	subDir := filepath.Join(testDir, "src", "main")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	repo2, err := DetectRepository(subDir)
	if err != nil {
		t.Fatalf("failed to detect repository from subdirectory: %v", err)
	}

	if repo2.Path != testDir {
		t.Errorf("expected repository root %s from subdirectory, got %s",
			testDir, repo2.Path)
	}

	// Test detection outside repository
	tmpDir := t.TempDir()
	repo3, err := DetectRepository(tmpDir)
	if err != ErrorNotARepository {
		t.Errorf("expected ErrorNotARepository, got %v (repo: %v)", err, repo3)
	}
}

func TestIsRepository(t *testing.T) {
	testDir := setupTestRepository(t)

	if !IsRepository(testDir) {
		t.Error("expected IsRepository to return true")
	}

	tmpDir := t.TempDir()
	if IsRepository(tmpDir) {
		t.Error("expected IsRepository to return false for non-repo")
	}
}

func TestGetRepositoryRoot(t *testing.T) {
	testDir := setupTestRepository(t)

	// Create a subdirectory
	subDir := filepath.Join(testDir, "deeply", "nested", "directory")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	root, err := GetRepositoryRoot(subDir)
	if err != nil {
		t.Fatalf("failed to get repository root: %v", err)
	}

	if root != testDir {
		t.Errorf("expected root %s, got %s", testDir, root)
	}
}

func TestRepository_GetStatus(t *testing.T) {
	testDir := setupTestRepository(t)
	ctx := context.Background()

	repo, err := DetectRepository(testDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create some test files
	files := map[string]string{
		"staged.go":   "package main\n\nfunc staged() {}",
		"unstaged.go": "package main\n\nfunc unstaged() {}",
		"modified.go": "package main\n\nfunc original() {}",
		"deleted.go":  "package main\n\nfunc deleted() {}",
	}

	for path, content := range files {
		fullPath := filepath.Join(testDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Stage some files
	stageCmd := exec.Command("git", "add", "staged.go", "modified.go", "deleted.go")
	stageCmd.Dir = testDir
	if err := stageCmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Modify a staged file
	modifiedContent := "package main\n\nfunc modified() {}"
	if err := os.WriteFile(filepath.Join(testDir, "modified.go"),
		[]byte(modifiedContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Delete a staged file
	if err := os.Remove(filepath.Join(testDir, "deleted.go")); err != nil {
		t.Fatal(err)
	}

	// Test getting all changes
	changes, err := repo.GetStatus(ctx, StatusOptions{IncludeRenames: true})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	// Verify we got expected changes
	foundStaged := false
	foundUnstaged := false
	foundModified := false
	foundDeleted := false

	for _, change := range changes {
		switch change.Path {
		case "staged.go":
			if change.ChangeType == ChangeAdded {
				foundStaged = true
			}
		case "unstaged.go":
			if change.ChangeType == ChangeUnknown {
				foundUnstaged = true
			}
		case "modified.go":
			foundModified = true
		case "deleted.go":
			if change.ChangeType == ChangeDeleted {
				foundDeleted = true
			}
		}
	}

	if !foundStaged {
		t.Error("did not find staged file")
	}
	if !foundUnstaged {
		t.Error("did not find unstaged file")
	}
	if !foundModified {
		t.Error("did not find modified file")
	}
	// deleted.go should be found but filtered out in parseStatusOutput
	if foundDeleted {
		t.Error("deleted file should be filtered out")
	}
}

func TestRepository_GetStagedChanges(t *testing.T) {
	testDir := setupTestRepository(t)
	ctx := context.Background()

	repo, err := DetectRepository(testDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create and stage a file
	content := "package main\n\nfunc test() {}"
	if err := os.WriteFile(filepath.Join(testDir, "staged.go"),
		[]byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	stageCmd := exec.Command("git", "add", "staged.go")
	stageCmd.Dir = testDir
	if err := stageCmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Create an unstaged file
	if err := os.WriteFile(filepath.Join(testDir, "unstaged.go"),
		[]byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	changes, err := repo.GetStagedChanges(ctx)
	if err != nil {
		t.Fatalf("GetStagedChanges failed: %v", err)
	}

	if len(changes) != 1 {
		t.Errorf("expected 1 staged change, got %d", len(changes))
	}

	if changes[0].Path != "staged.go" {
		t.Errorf("expected staged.go, got %s", changes[0].Path)
	}
}

func TestRepository_GetAllChanges(t *testing.T) {
	testDir := setupTestRepository(t)
	ctx := context.Background()

	repo, err := DetectRepository(testDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create multiple files
	files := []string{"file1.go", "file2.go", "file3.go"}
	for _, file := range files {
		content := fmt.Sprintf("package main\n\n// %s\n", file)
		if err := os.WriteFile(filepath.Join(testDir, file),
			[]byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Stage some files
	stageCmd := exec.Command("git", "add", "file1.go", "file2.go")
	stageCmd.Dir = testDir
	if err := stageCmd.Run(); err != nil {
		t.Fatal(err)
	}

	changes, err := repo.GetAllChanges(ctx)
	if err != nil {
		t.Fatalf("GetAllChanges failed: %v", err)
	}

	// Should have 3 changes: 2 staged, 1 unstaged
	if len(changes) != 3 {
		t.Errorf("expected 3 changes, got %d", len(changes))
	}
}
