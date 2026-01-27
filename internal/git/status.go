package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func (r *Repository) GetStatus(ctx context.Context, opts StatusOptions) ([]FileChange, error) {
	if r.IsBare {
		return nil, errors.New("cannot get status of bare repository")
	}

	// Build git status command
	args := []string{"status", "--porcelain=v1", "-z"}
	if opts.IncludeRenames {
		args = append(args, "--find-renames")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.Path

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git status failed: %s", exitErr.Stderr)
		}
		return nil, fmt.Errorf("git status failed: %v", err)
	}
	return parseStatusOutput(output, opts)
}

func parseStatusOutput(output []byte, opts StatusOptions) ([]FileChange, error) {
	var changes []FileChange

	entries := bytes.Split(output, []byte{0})

	for i := 0; i < len(entries); i++ {
		entry := string(entries[i])
		if entry == "" {
			continue
		}
		if len(entry) < 4 {
			continue
		}

		x := entry[0]
		y := entry[1]

		// Determine change type
		var changeType ChangeType
		var stage string
		var path, oldPath string

		switch {
		case x == 'R' || y == 'R':
			changeType = ChangeRenamed
			parts := strings.SplitN(entry[3:], " -> ", 2)
			if len(parts) == 2 {
				oldPath = parts[0]
				path = parts[1]
			} else {
				if i+2 < len(entries) {
					oldPath = entry[3:]
					path = string(entries[i+1])
					i++
				}
			}
		case x == 'C' || y == 'C':
			changeType = ChangeCopied
			parts := strings.SplitN(entry[3:], " -> ", 2)
			if len(parts) == 2 {
				oldPath = parts[0]
				path = parts[1]
			}
		default:
			path = entry[3:]
			changeType = getChangeType(x, y)
			if x == 'U' || y == 'U' || x == 'A' || y == 'A' || x == 'D' || y == 'D' {
				stage = getStage(x, y)
			}
		}
		if !shouldIncludeChange(x, y, opts) {
			continue
		}

		// Skip deleted files for review
		if changeType == ChangeDeleted {
			continue
		}

		changes = append(changes, FileChange{
			Path:       path,
			OldPath:    oldPath,
			ChangeType: changeType,
			Stage:      stage,
		})
	}

	return changes, nil
}

func getChangeType(x, y byte) ChangeType {
	switch {
	case x == 'A' || y == 'A':
		return ChangeAdded
	case x == 'M' || y == 'M':
		return ChangeModified
	case x == 'D' || y == 'D':
		return ChangeDeleted
	case x == 'T' || y == 'T':
		return ChangeTypeChan
	case x == 'U' || y == 'U':
		return ChangeUnmerged
	case x == '?' || y == '?':
		return ChangeUnknown
	default:
		return ChangeModified
	}
}

// getStage determines the stage area for unmerged files
func getStage(x, y byte) string {
	if x != ' ' {
		return fmt.Sprintf("stage-%c", x)
	}
	if y != ' ' {
		return fmt.Sprintf("stage-%c", y)
	}
	return ""
}

// determines if a change should be included based on options
func shouldIncludeChange(x, y byte, opts StatusOptions) bool {
	if !opts.StagedOnly && !opts.UnstagedOnly {
		return true
	}
	// This function encodes Git semantics, not app semantics.
	if opts.StagedOnly && x != ' ' && x != '?' && x != '!' && x != 'I' {
		return true
	}

	if opts.UnstagedOnly && y != ' ' && y != '?' && y != '!' && y != 'I' {
		return true
	}
	return false
}

// GetStagedChanges returns only staged changes
func (r *Repository) GetStagedChanges(ctx context.Context) ([]FileChange, error) {
	return r.GetStatus(ctx, StatusOptions{
		StagedOnly:     true,
		IncludeRenames: true,
	})
}

// returns only unstaged changes
func (r *Repository) GetUnstagedChanges(ctx context.Context) ([]FileChange, error) {
	return r.GetStatus(ctx, StatusOptions{
		UnstagedOnly:   true,
		IncludeRenames: true,
	})
}

// returns all changes (staged and unstaged)
func (r *Repository) GetAllChanges(ctx context.Context) ([]FileChange, error) {
	return r.GetStatus(ctx, StatusOptions{
		IncludeRenames: true,
	})
}
