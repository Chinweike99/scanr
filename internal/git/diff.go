package git

import (
    "bufio"
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

// DiffOptions holds options for getting git diff
/**
* Cached     bool Show staged changes (--cached)
* NameOnly   bool Show only names of changed files
* NoRenames  bool Disable rename detection
* Unified    int  Number of context lines (default: 3)
*/
type DiffOptions struct {
    Cached     bool
    NameOnly   bool
    NoRenames  bool
    Unified    int
}

// GetDiff returns the diff for a specific file or all changes
func (r *Repository) GetDiff(ctx context.Context, path string, opts DiffOptions) (string, error) {
    args := []string{"diff", "--no-color", "--no-ext-diff"}
    
    if opts.Cached {
        args = append(args, "--cached")
    }
    
    if opts.NameOnly {
        args = append(args, "--name-only")
    }
    
    if opts.NoRenames {
        args = append(args, "--no-renames")
    }
    
    if opts.Unified > 0 {
        args = append(args, fmt.Sprintf("--unified=%d", opts.Unified))
    } else {
        args = append(args, "--unified=3")
    }
    
    // Add path if specified
    if path != "" {
        args = append(args, "--", path)
    }
    
    cmd := exec.CommandContext(ctx, "git", args...)
    cmd.Dir = r.Path
    
    output, err := cmd.Output()
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            return "", fmt.Errorf("git diff failed: %s", exitErr.Stderr)
        }
        return "", fmt.Errorf("git diff failed: %v", err)
    }
    
    return string(output), nil
}

// GetFileContent returns the content of a file at a specific revision
func (r *Repository) GetFileContent(ctx context.Context, revision, path string) ([]byte, error) {
    ref := revision
    if ref == "" {
        ref = "HEAD"
    }
    
    if strings.HasPrefix(revision, "stage-") {
        stage := revision[6:]
        ref = fmt.Sprintf(":%s:%s", stage, path)
    } else if revision != "" {
        ref = fmt.Sprintf("%s:%s", revision, path)
    } else {
        ref = fmt.Sprintf("HEAD:%s", path)
    }
    
    cmd := exec.CommandContext(ctx, "git", "show", "--no-color", "--no-ext-diff", ref)
    cmd.Dir = r.Path
    
    output, err := cmd.Output()
    if err != nil {
        if strings.Contains(err.Error(), "exists on disk, but not in") ||
            strings.Contains(err.Error(), "does not exist") {
            return nil, nil
        }
        
        if exitErr, ok := err.(*exec.ExitError); ok {
            return nil, fmt.Errorf("git show failed: %s", exitErr.Stderr)
        }
        return nil, fmt.Errorf("git show failed: %v", err)
    }
    
    return output, nil
}

// GetStagedContent returns the staged content of a file
func (r *Repository) GetStagedContent(ctx context.Context, path string) ([]byte, error) {
    return r.GetFileContent(ctx, ":0", path)
}

func (r *Repository) GetWorkingTreeContent(ctx context.Context, path string) ([]byte, error) {
    fullPath := filepath.Join(r.Path, path)
    return os.ReadFile(fullPath)
}

// GetChangedFiles returns the list of changed files with their diff
func (r *Repository) GetChangedFiles(ctx context.Context, stagedOnly bool) (map[string]string, error) {
    opts := DiffOptions{
        Cached:   stagedOnly,
        NameOnly: true,
    }
    
    diffOutput, err := r.GetDiff(ctx, "", opts)
    if err != nil {
        return nil, err
    }
    
    files := make(map[string]string)
    scanner := bufio.NewScanner(strings.NewReader(diffOutput))
    
    for scanner.Scan() {
        path := strings.TrimSpace(scanner.Text())
        if path != "" {
            diff, err := r.GetDiff(ctx, path, DiffOptions{Cached: stagedOnly})
            if err != nil {
                return nil, fmt.Errorf("failed to get diff for %s: %v", path, err)
            }
            files[path] = diff
        }
    }
    
    if err := scanner.Err(); err != nil {
        return nil, fmt.Errorf("failed to parse diff output: %v", err)
    }
    
    return files, nil
}