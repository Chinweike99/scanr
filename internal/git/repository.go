package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Git Repository Detection

var (
	ErrorNotARepository = errors.New("not a git repository")
	ErrorGitNotFound    = errors.New("git command not found")
)

func DetectRepository(startPath string) (*Repository, error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %v", err)
	}

	if _, err := exec.LookPath("git"); err != nil {
		return nil, ErrorGitNotFound
	}

	current := absPath
	for {
		gitDir := filepath.Join(current, ".git")

		if fi, err := os.Stat(gitDir); err == nil && fi.IsDir() {
			return createRepository(current, gitDir)
		}

		if isBareRepository(current) {
			return createRepository(current, current)
		}

		//Move to parent directory
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil, ErrorNotARepository
}

// Checks if a directory is a bare git repository
func isBareRepository(path string) bool {
	checkFiles := []string{"HEAD", "config", "objects", "refs"}
	for _, file := range checkFiles {
		if _, err := os.Stat(filepath.Join(path, file)); os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func createRepository(path, gitDir string) (*Repository, error) {
	isBare := path == gitDir

	isShallow := false
	shallowFile := filepath.Join(gitDir, "shallow")
	if _, err := os.Stat(shallowFile); err == nil {
		isShallow = true
	}

	return &Repository{
		Path:      path,
		WorkTree:  path,
		GitDir:    gitDir,
		IsBare:    isBare,
		IsShallow: isShallow,
	}, nil
}

func IsRepository(path string) bool {
	repo, err := DetectRepository(path)
	return err == nil && repo != nil
}

func GetRepositoryRoot(startPath string) (string, error) {
	repo, err := DetectRepository(startPath)
	if err != nil {
		return "", err
	}
	return repo.Path, nil
}
