package output

import "scanr/internal/review"

// DetermineExitCode returns an exit code based on the review result:
// 2 = criticals present or nil result, 1 = warnings present, 0 = no issues
func DetermineExitCode(result *review.ReviewResult) int {
	if result == nil {
		return 2
	}
	if result.CriticalCount > 0 {
		return 2
	}
	if result.WarningCount > 0 {
		return 1
	}
	return 0
}
