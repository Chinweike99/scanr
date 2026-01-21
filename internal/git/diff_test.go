package git

import (
    "context"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"
)

func TestRepository_GetDiff(t *testing.T) {
    testDir := setupTestRepository(t)
    ctx := context.Background()
    
    repo, err := DetectRepository(testDir)
    if err != nil {
        t.Fatal(err)
    }
    
    // Create and stage a file
    originalContent := "package main\n\nfunc original() {\n    // original code\n}"
    filePath := filepath.Join(testDir, "test.go")
    if err := os.WriteFile(filePath, []byte(originalContent), 0644); err != nil {
        t.Fatal(err)
    }
    
    stageCmd := exec.Command("git", "add", "test.go")
    stageCmd.Dir = testDir
    if err := stageCmd.Run(); err != nil {
        t.Fatal(err)
    }
    
    // Commit the file
    commitCmd := exec.Command("git", "commit", "-m", "Initial commit")
    commitCmd.Dir = testDir
    if err := commitCmd.Run(); err != nil {
        t.Fatal(err)
    }
    
    // Modify the file
    modifiedContent := "package main\n\nfunc modified() {\n    // modified code\n    // new line\n}"
    if err := os.WriteFile(filePath, []byte(modifiedContent), 0644); err != nil {
        t.Fatal(err)
    }
    
    // Get diff
    diff, err := repo.GetDiff(ctx, "test.go", DiffOptions{})
    if err != nil {
        t.Fatalf("GetDiff failed: %v", err)
    }
    
    // Verify diff contains expected content
    if !strings.Contains(diff, "func original") {
        t.Error("diff should contain original function")
    }
    if !strings.Contains(diff, "func modified") {
        t.Error("diff should contain modified function")
    }
    if !strings.Contains(diff, "new line") {
        t.Error("diff should contain new line")
    }
    
    // Test staged diff
    stageCmd = exec.Command("git", "add", "test.go")
    stageCmd.Dir = testDir
    if err := stageCmd.Run(); err != nil {
        t.Fatal(err)
    }
    
    stagedDiff, err := repo.GetDiff(ctx, "test.go", DiffOptions{Cached: true})
    if err != nil {
        t.Fatalf("GetDiff with cached failed: %v", err)
    }
    
    if !strings.Contains(stagedDiff, "func original") {
        t.Error("staged diff should contain original function")
    }
    if !strings.Contains(stagedDiff, "func modified") {
        t.Error("staged diff should contain modified function")
    }
}

func TestRepository_GetFileContent(t *testing.T) {
    testDir := setupTestRepository(t)
    ctx := context.Background()
    
    repo, err := DetectRepository(testDir)
    if err != nil {
        t.Fatal(err)
    }
    
    // Create and commit a file
    content := "package main\n\nfunc test() {\n    return 42\n}"
    filePath := filepath.Join(testDir, "content.go")
    if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
        t.Fatal(err)
    }
    
    stageCmd := exec.Command("git", "add", "content.go")
    stageCmd.Dir = testDir
    if err := stageCmd.Run(); err != nil {
        t.Fatal(err)
    }
    
    commitCmd := exec.Command("git", "commit", "-m", "Add content")
    commitCmd.Dir = testDir
    if err := commitCmd.Run(); err != nil {
        t.Fatal(err)
    }
    
    // Get content from HEAD
    headContent, err := repo.GetFileContent(ctx, "HEAD", "content.go")
    if err != nil {
        t.Fatalf("GetFileContent from HEAD failed: %v", err)
    }
    
    if string(headContent) != content {
        t.Errorf("HEAD content doesn't match:\ngot:\n%s\nwant:\n%s", 
            string(headContent), content)
    }
    
    // Get content from working tree (should be the same)
    workingContent, err := repo.GetWorkingTreeContent(ctx, "content.go")
    if err != nil {
        t.Fatalf("GetWorkingTreeContent failed: %v", err)
    }
    
    if string(workingContent) != content {
        t.Errorf("working tree content doesn't match")
    }
}

func TestRepository_GetChangedFiles(t *testing.T) {
    testDir := setupTestRepository(t)
    ctx := context.Background()
    
    repo, err := DetectRepository(testDir)
    if err != nil {
        t.Fatal(err)
    }
    
    // Create multiple files
    files := map[string]string{
        "file1.go": "package main\n\nfunc one() {}",
        "file2.go": "package main\n\nfunc two() {}",
        "file3.go": "package main\n\nfunc three() {}",
    }
    
    for name, content := range files {
        path := filepath.Join(testDir, name)
        if err := os.WriteFile(path, []byte(content), 0644); err != nil {
            t.Fatal(err)
        }
    }
    
    // Stage some files
    stageCmd := exec.Command("git", "add", "file1.go", "file2.go")
    stageCmd.Dir = testDir
    if err := stageCmd.Run(); err != nil {
        t.Fatal(err)
    }
    
    // Get staged changes
    stagedFiles, err := repo.GetChangedFiles(ctx, true)
    if err != nil {
        t.Fatalf("GetChangedFiles (staged) failed: %v", err)
    }
    
    if len(stagedFiles) != 2 {
        t.Errorf("expected 2 staged files, got %d", len(stagedFiles))
    }
    
    if _, ok := stagedFiles["file1.go"]; !ok {
        t.Error("file1.go should be in staged files")
    }
    if _, ok := stagedFiles["file2.go"]; !ok {
        t.Error("file2.go should be in staged files")
    }
    if _, ok := stagedFiles["file3.go"]; ok {
        t.Error("file3.go should not be in staged files")
    }
    
    // Get all changes
    allFiles, err := repo.GetChangedFiles(ctx, false)
    if err != nil {
        t.Fatalf("GetChangedFiles (all) failed: %v", err)
    }
    
    if len(allFiles) != 3 {
        t.Errorf("expected 3 files total, got %d", len(allFiles))
    }
}