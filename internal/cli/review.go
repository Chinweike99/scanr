package cli

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"scanr/internal/config"
	"scanr/internal/fs"
	"scanr/internal/git"
	"strings"
)

/**
* RunReview is the main entry point for the review command
 */
func RunReview(ctx context.Context, cfg *config.Config) (int, error) {
	languages, err := ParseLanguages(cfg.Languages)
	if err != nil {
		return 2, fmt.Errorf("failed to parse language: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return 2, fmt.Errorf("failed to get current working directory: %v", err)
	}

	 // Detect git repository
    repo, err := git.DetectRepository(cwd)
    if err != nil {
        log.Printf("Warning: Not a git repository (%v), scanning all files", err)
        return scanAllFiles(ctx, cwd, languages, cfg)
    }

	log.Printf("Found git repository at: %s", repo.Path)

	// Get git changes based on staged flag
    var changes []git.FileChange
    if cfg.StagedOnly {
        changes, err = repo.GetStagedChanges(ctx)
        if err != nil {
            return 2, fmt.Errorf("failed to get staged changes: %v", err)
        }
        log.Printf("Found %d staged file(s)", len(changes))
    } else {
        changes, err = repo.GetAllChanges(ctx)
        if err != nil {
            return 2, fmt.Errorf("failed to get changes: %v", err)
        }
        log.Printf("Found %d changed file(s)", len(changes))
    }

	// Filter changes by language and convert to FileInfo
    files, err := filterAndConvertChanges(ctx, repo, changes, languages, cfg.MaxFiles)
    if err != nil {
        return 2, fmt.Errorf("failed to process changes: %v", err)
    }


	// Create review options
	opts := &config.ReviewOptions{
		Languages:   languages,
		StagedOnly:  cfg.StagedOnly,
		MaxFiles:    cfg.MaxFiles,
		Format:      cfg.Format,
		Interactive: cfg.Languages == "",
		Files:       files,
		Repository:  repo,
	}

	// // This would be replaced with actual review pipeline in later phases
	fmt.Printf("Review configuration:\n")
	fmt.Printf("  Languages: %v\n", opts.Languages)
	fmt.Printf("  Staged only: %v\n", opts.StagedOnly)
	fmt.Printf("  Max files: %d\n", opts.MaxFiles)
	fmt.Printf("  Format: %s\n", opts.Format)
	fmt.Printf("  Interactive: %v\n", opts.Interactive)
	fmt.Printf("  Files: %v\n", opts.Files)
	fmt.Printf("  Interactive: %v\n", opts.Repository)

	displayFoundFiles(files, cfg.Format)

	// This will be determined by actual review results in later phases
	return 0, nil
}



// scanAllFiles handles non-git repository scanning
func scanAllFiles(ctx context.Context, cwd string, languages []string, cfg *config.Config) (int, error) {
    log.Println("Scanning all files (not a git repository)")
    
    // Create filesystem scanner
    scanner, err := fs.NewScanner(fs.Config{
        RootDir:     cwd,
        Languages:   languages,
        MaxFileSize: 1024 * 1024,
        MaxLines:    1000,
        IgnoreDirs:  []string{},
    })
    if err != nil {
        return 2, fmt.Errorf("failed to create scanner: %v", err)
    }
    
    // Scan for files
    files, err := scanner.Scan(ctx, cfg.MaxFiles)
    if err != nil {
        return 2, fmt.Errorf("failed to scan files: %v", err)
    }
 
    opts := &config.ReviewOptions{
        Languages:   languages,
        StagedOnly:  false,
        MaxFiles:    cfg.MaxFiles,
        Format:      cfg.Format,
        Interactive: cfg.Languages == "",
        Files:       files,
    }

	// // This would be replaced with actual review pipeline in later phases
	fmt.Printf("Review configuration:\n")
	fmt.Printf("  Languages: %v\n", opts.Languages)
	fmt.Printf("  Staged only: %v\n", opts.StagedOnly)
	fmt.Printf("  Max files: %d\n", opts.MaxFiles)
	fmt.Printf("  Format: %s\n", opts.Format)
	fmt.Printf("  Interactive: %v\n", opts.Interactive)
	fmt.Printf("  Files: %v\n", opts.Files)
	fmt.Printf("  Interactive: %v\n", opts.Repository)


    displayFoundFiles(files, cfg.Format)
    
    return 0, nil
}


// filterAndConvertChanges filters git changes by language and converts to FileInfo
func filterAndConvertChanges(ctx context.Context, repo *git.Repository, 
    changes []git.FileChange, languages []string, maxFiles int) ([]fs.FileInfo, error) {
    
    // Build language extensions map for filtering
    langExts := make(map[string]bool)
    for _, lang := range languages {
        exts, ok := fs.SupportedExtensions[lang]
        if !ok {
            continue
        }
        for _, ext := range exts {
            langExts[ext] = true
        }
    }
    
    var files []fs.FileInfo
    fileCount := 0
    
    for _, change := range changes {
        // Skip deleted files
        if change.ChangeType == git.ChangeDeleted {
            continue
        }
        ext := strings.ToLower(filepath.Ext(change.Path))
        if !langExts[ext] {
            continue
        }
        
        fullPath := filepath.Join(repo.Path, change.Path)
        info, err := os.Stat(fullPath)
        if err != nil {
            continue
        }
        
        // Check size limit
        if info.Size() > 1024*1024 {
            continue
        }
        
        lines, err := countFileLines(fullPath)
        if err != nil {
            continue
        }
        
        if lines > 1000 {
            continue
        }
        
        language := ""
        for lang, exts := range fs.SupportedExtensions {
            for _, e := range exts {
                if ext == e {
                    language = lang
                    break
                }
            }
            if language != "" {
                break
            }
        }
        
        if language == "" {
            continue
        }
        
        files = append(files, fs.FileInfo{
            Path:     fullPath,
            Size:     info.Size(),
            Lines:    lines,
            Languages: language,
            Relative: change.Path,
        })
        
        fileCount++
        if maxFiles > 0 && fileCount >= maxFiles {
            break
        }
    }
    
    return files, nil
}


// countFileLines counts lines in a file
func countFileLines(path string) (int, error) {
    file, err := os.Open(path)
    if err != nil {
        return 0, err
    }
    defer file.Close()
    
    count := 0
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        count++
        if count > 1000 {
            break
        }
    }
    
    return count, scanner.Err()
}




func displayFoundFiles(files []fs.FileInfo, format string) {
	if len(files) == 0 {
		log.Println("No files found to review")
		return
	}

	switch format {
	case "json":
		fmt.Println("[")
		for i, file := range files {
			fmt.Printf("  {\n")
			fmt.Printf("    \"path\": %q,\n", file.Path)
			fmt.Printf("    \"relative\": %q,\n", file.Relative)
			fmt.Printf("    \"size\": %d,\n", file.Size)
			fmt.Printf("    \"lines\": %d,\n", file.Lines)
			fmt.Printf("    \"language\": %q\n", file.Languages)
			fmt.Printf("  }")
			if i < len(files)-1 {
				fmt.Println(",")
			} else {
				fmt.Println()
			}
		}
		fmt.Println("]")
	default:
		fmt.Printf("Found %d file(s) to review:\n", len(files))
		for _, file := range files {
			size := formatFileSize(file.Size)
			fmt.Printf("  %s (%s, %d lines, %s)\n",
				file.Relative, file.Languages, file.Lines, size)
		}
	}
}

func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
