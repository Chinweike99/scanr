package fs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Scans filesysytem for reviewable files
type Scanner struct {
	rootDir     string
	languages   map[string][]string
	maxFileSize int64
	maxLines    int
	ignoreDirs  map[string]bool
	mu          sync.RWMutex
	scannedDir  map[string]bool
}

// Respresents file to be reviewed
type FileInfo struct {
	Path      string
	Size      int64
	Lines     int
	Languages string
	Relative  string
}

// Config holds scanner configuration
type Config struct {
	RootDir     string
	Languages   []string
	MaxFileSize int64
	MaxLines    int
	IgnoreDirs  []string
}

// Default configuration
const (
	DefaultMaxFileSize = 1024 * 1024
	DefaultMaxLines    = 1000
)

var (
	DefaultIgnoreDirs = []string{
		".git",
		"node_modules",
		"vendor",
		"__pycache__",
		".next",
		"dist",
		"build",
		"target",
		"coverage",
		".vscode",
		".idea",
		".md",
	}
)

// Creates a new filesystem scanner
func NewScanner(cfg Config) (*Scanner, error) {
	if cfg.RootDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %v", err)
		}
		cfg.RootDir = cwd
	}

	// Normalize root directory path
	rootDir, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, fmt.Errorf("invalid root directory: %v", err)
	}

	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("root directory does not exist: %s", rootDir)
	}

	// Get Language extensions
	langExts, err := getLanguageExtensions(cfg.Languages)
	if err != nil {
		return nil, err
	}

	// Set Defaults
	if cfg.MaxFileSize <= 0 {
		cfg.MaxFileSize = DefaultMaxFileSize
	}

	if cfg.MaxLines <= 0 {
		cfg.MaxLines = DefaultMaxLines
	}

	// Build ignore directores map
	igonoreDir := make(map[string]bool)
	for _, dir := range DefaultIgnoreDirs {
		igonoreDir[dir] = true
	}

	for _, dir := range cfg.IgnoreDirs {
		igonoreDir[dir] = true
	}

	return &Scanner{
		rootDir:     rootDir,
		languages:   langExts,
		maxFileSize: cfg.MaxFileSize,
		maxLines:    cfg.MaxLines,
		ignoreDirs:  igonoreDir,
		scannedDir:  make(map[string]bool),
	}, nil

}

// getLanguageExtensions maps language names to their file extensions
func getLanguageExtensions(languages []string) (map[string][]string, error) {
	if len(languages) == 0 {
		return nil, errors.New("no languages specified")
	}

	result := make(map[string][]string)
	for _, lang := range languages {
		exts, ok := SupportedExtensions[lang]
		if !ok {
			return nil, fmt.Errorf("unsupported language: %s", lang)
		}
		result[lang] = exts
	}
	return result, nil
}

// SupportedExtensions maps language keys to their file extensions
var SupportedExtensions = map[string][]string{
	"go":         {".go"},
	"java":       {".java"},
	"typescript": {".ts", ".tsx"},
	"javascript": {".js", ".jsx", ".mjs", ".cjs"},
	"python":     {".py"},
	"csharp":     {".cs"},
	"dotnet":     {".cs", ".vb", ".fs"},
}

// Scan scans the filesystem for reviewable files
func (s *Scanner) Scan(ctx context.Context, maxFiles int) ([]FileInfo, error) {
	// Load .gitignore patterns
	gitignorePatterns, err := s.loadGitIgnorePatterns()
	if err != nil {
		return nil, fmt.Errorf("failed to load .gitignore: %v", err)
	}

	var files []FileInfo
	var mu sync.Mutex
	var scanErr error

	// Create a semaphore to limit concurrent file scanning
	sem := make(chan struct{}, 10)

	err = filepath.WalkDir(s.rootDir, func(path string, d fs.DirEntry, err error) error {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return err
		}

		// Skip directories that should be ignored
		if d.IsDir() {
			return s.handleDirectory(path, d)
		}

		// Check if we've reached the maximum number of files
		mu.Lock()
		if len(files) >= maxFiles && maxFiles > 0 {
			mu.Unlock()
			return fs.SkipAll
		}
		mu.Unlock()

		// Check if file should be ignored
		if s.shouldIgnore(path, gitignorePatterns) {
			return nil
		}

		// Check file extension
		ext := strings.ToLower(filepath.Ext(path))
		lang := s.getLanguageForExtension(ext)
		if lang == "" {
			return nil
		}

		// Get file info and check size
		info, err := d.Info()
		if err != nil {
			// Skip files we can't stat
			return nil
		}

		// Check file size
		if info.Size() > s.maxFileSize {
			return nil
		}

		// Process file (with concurrency limit)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()

			// Count lines in file
			lines, err := s.countLines(path)
			if err != nil {
				// Skip files we can't read
				return
			}

			// Check line limit
			if lines > s.maxLines {
				return
			}

			relativePath, err := filepath.Rel(s.rootDir, path)
			if err != nil {
				// Fall back to absolute path
				relativePath = path
			}

			fileInfo := FileInfo{
				Path:      path,
				Size:      info.Size(),
				Lines:     lines,
				Languages: lang,
				Relative:  relativePath,
			}

			mu.Lock()
			if len(files) < maxFiles || maxFiles <= 0 {
				files = append(files, fileInfo)
			}
			mu.Unlock()
		}()

		return nil
	})

	// Wait for all goroutines to complete
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}

	if err != nil && !errors.Is(err, fs.SkipAll) {
		return nil, fmt.Errorf("walk error: %v", err)
	}

	if scanErr != nil {
		return nil, scanErr
	}

	return files, nil
}

// loadGitignorePatterns loads and parses .gitignore files
func (s *Scanner) loadGitIgnorePatterns() ([]string, error) {
	var patterns []string

	// Walk up the directory tree to find all .gitignore files
	dir := s.rootDir
	for {
		gitignorePath := filepath.Join(dir, ".gitignore")
		if _, err := s.parseGitIgnoreFile(gitignorePath, patterns); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached roor
		}
		dir = parent
	}
	// Add patterns from root .gitignore
	roorGitignore := filepath.Join(s.rootDir, ".gitignore")
	return s.parseGitIgnoreFile(roorGitignore, patterns)
}

// parseGitignoreFile parses a .gitignore file
func (s *Scanner) parseGitIgnoreFile(path string, existingPatterns []string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return existingPatterns, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		//Handles Navigation
		if strings.HasPrefix(line, "!") {
			// For now, we'll just skip negated patterns
			continue
		}

		// Handle diretory patterns ending with /
		if strings.HasSuffix(line, "/") {
			line = strings.TrimSuffix(line, "/") + "/*"
		}
		pattern := strings.ReplaceAll(line, "**/", "*")
		pattern = strings.ReplaceAll(pattern, "*", "*")

		existingPatterns = append(existingPatterns, pattern)
	}

	if err := scanner.Err(); err != nil {
		return existingPatterns, fmt.Errorf("error reading .gitignore: %v", err)
	}
	return existingPatterns, nil
}

// returns the language for a given file extension
func (s *Scanner) getLanguageForExtension(ext string) string {
	for lang, exts := range s.languages {
		for _, e := range exts {
			if ext == e {
				return lang
			}
		}
	}
	return ""
}

// countLines: counts the number of lines in a file
func (s *Scanner) countLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, nil
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
		if count > s.maxLines {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func countLinesFromReader(r io.Reader) (int, error) {
	count := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

// handleDirectory decides whether to skip a directory
func (s *Scanner) handleDirectory(path string, d fs.DirEntry) error {
	base := filepath.Base(path)

	// Check if directory should be ignored
	if s.ignoreDirs[base] {
		return fs.SkipDir
	}

	s.mu.Lock()
	if s.scannedDir[path] {
		s.mu.Unlock()
		return fs.SkipDir
	}
	s.scannedDir[path] = true
	s.mu.Unlock()

	return nil
}

// shouldIgnore checks if a file should be ignored based on .gitignore patterns
func (s *Scanner) shouldIgnore(path string, patterns []string) bool {
	relPath, err := filepath.Rel(s.rootDir, path)
	if err != nil {
		return true // Can't get relative path, skip it
	}

	// Check against .gitignore patterns
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}

		matched, err := filepath.Match(pattern, relPath)
		if err == nil && matched {
			return true
		}

		// Also try matching against the base name
		matched, err = filepath.Match(pattern, filepath.Base(relPath))
		if err == nil && matched {
			return true
		}
	}

	return false
}
