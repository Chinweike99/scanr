package reviewer

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"strings"
	"time"
)

type PromptTemplate struct {
	system    string
	User      string
	MaxTokens int
}

// ReviewRequest contains all data needed for a review
type ReviewRequest struct {
	File       fs.FileInfo
	Relative   string
	Content    string
	Language   string
	Context    string
	Guidelines []string
	MaxIssues  int
	FocusAreas []string
}

// languageTemplates contains language-specific review guidelines
var languageTemplates = map[string][]string{
	"go": {
		"Check for proper error handling (no ignored errors)",
		"Verify resource cleanup (defer statements for files, connections)",
		"Check for goroutine leaks and proper context usage",
		"Validate slice/map concurrency safety",
		"Check for interface implementation correctness",
		"Verify pointer/receiver usage consistency",
		"Check for proper package organization",
		"Validate error wrapping with context",
		"Check for unnecessary allocations in loops",
		"Verify test coverage and table-driven tests",
	},
	"python": {
		"Check for exception handling (no bare except clauses)",
		"Verify resource management (context managers for files)",
		"Check for proper type hints (PEP 484)",
		"Validate list/dict comprehension efficiency",
		"Check for mutable default arguments",
		"Verify proper use of async/await patterns",
		"Check for proper module imports",
		"Validate docstring formatting (PEP 257)",
		"Check for proper virtual environment usage",
		"Verify test structure and pytest usage",
	},
	"javascript": {
		"Check for promise handling and async/await patterns",
		"Verify error handling in async functions",
		"Check for proper module imports (ES6 vs CommonJS)",
		"Validate TypeScript type annotations if applicable",
		"Check for memory leaks with event listeners",
		"Verify proper use of const/let vs var",
		"Check for security issues (XSS, injection)",
		"Validate package.json dependencies",
		"Check for proper testing framework usage",
		"Verify browser compatibility if needed",
	},
	"typescript": {
		"Check for strict TypeScript configuration",
		"Verify type safety and proper generics usage",
		"Check for any type usage (should be minimized)",
		"Validate interface/type definitions",
		"Check for proper module resolution",
		"Verify enum usage vs union types",
		"Check for proper error typing",
		"Validate tsconfig.json settings",
		"Check for unused imports/variables",
		"Verify test type definitions",
	},
	"java": {
		"Check for exception handling (no swallowed exceptions)",
		"Verify resource management (try-with-resources)",
		"Check for proper use of final where applicable",
		"Validate null safety and Optional usage",
		"Check for proper access modifiers",
		"Verify equals/hashCode implementations",
		"Check for thread safety in shared data",
		"Validate package structure and naming",
		"Check for proper logging instead of print statements",
		"Verify JUnit test structure",
	},
	"csharp": {
		"Check for proper exception handling",
		"Verify resource management (using statements)",
		"Check for null safety and nullable references",
		"Validate async/await patterns",
		"Check for proper access modifiers",
		"Verify IDisposable implementation if needed",
		"Check for thread safety in shared data",
		"Validate namespace organization",
		"Check for proper logging",
		"Verify unit test structure (xUnit/NUnit)",
	},
}

func GetLanguageGuidelines(language string) []string {
	if guidelines, ok := languageTemplates[language]; ok {
		return guidelines
	}

	return []string{
		"Check for security vulnerabilities",
		"Verify error handling and edge cases",
		"Check for performance issues",
		"Validate code readability and maintainability",
		"Check for proper documentation",
		"Verify coding standards compliance",
		"Check for duplication and code smells",
		"Validate test coverage where applicable",
	}
}

// BuildPrompt builds the complete prompt for AI review
func BuildPrompt(req ReviewRequest) (string, error) {
	tmpl := getPromptTemplate(req.Language)

	data := map[string]interface{}{
		"FileName":   req.Relative,
		"Language":   req.Language,
		"Content":    req.Content,
		"Context":    req.Context,
		"Guidelines": req.Guidelines,
		"MaxIssues":  req.MaxIssues,
		"FocusAreas": req.FocusAreas,
		"Timestamp":  time.Now().Format(time.RFC3339),
		"LineCount":  strings.Count(req.Content, "\n") + 1,
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute prompt template: %w", err)
	}
	return buf.String(), nil
}

func getPromptTemplate(language string) *template.Template {
	systemPrompt := `You are an expert code reviewer analyzing code for production readiness.
			Your task is to identify critical issues, warnings, and provide constructive feedback.
			Focus on security, reliability, performance, and maintainability.

			CRITICAL ISSUES (exit code 2):
			- Security vulnerabilities (hardcoded secrets, injection risks)
			- Resource leaks (files, connections not closed)
			- Unhandled errors/exceptions
			- Concurrency issues (race conditions, deadlocks)
			- Memory safety issues (buffer overflows, null dereferences)
			- Infinite loops or unbounded recursion
			- Missing cleanup in error paths

			WARNINGS (exit code 1):
			- Code complexity (long functions, deep nesting)
			- Code duplication
			- Naming inconsistencies
			- Dead code or unused imports
			- Missing tests for critical paths
			- Performance anti-patterns
			- Inconsistent error handling

			INFO (exit code 0):
			- Documentation improvements
			- Code style suggestions
			- Refactoring opportunities
			- Test improvement suggestions
			- Performance optimizations

			Format your response as a JSON array of issues with the following structure for each issue:
			{
			"title": "Brief descriptive title",
			"description": "Detailed explanation",
			"severity": "critical|warning|info",
			"line": <line_number>,
			"column": <column_number_if_available>,
			"category": "security|performance|maintainability|reliability|style|documentation",
			"suggestions": ["suggestion1", "suggestion2"],
			"confidence": <0.0_to_1.0>
			}

			Return ONLY the JSON array, no additional text.`

	// Language-specific user prompt templates
	// Language-specific user prompt template
	userPrompt := `Review the following {{.Language}} code file: {{.FileName}}

			File Content:
			` + "```" + `{{.Language}}
			{{.Content}}
			` + "```" + `

			{{if .Context}}Git Context/Diff:
			` + "```" + `diff
			{{.Context}}
			` + "```" + `
			{{end}}

			Review Guidelines:
			{{range $index, $guideline := .Guidelines}}- {{$guideline}}
			{{end}}

			{{if .FocusAreas}}Focus Areas:
			{{range $index, $area := .FocusAreas}}- {{$area}}
			{{end}}{{end}}

			Please analyze this code and identify any issues. Be specific and provide actionable suggestions.
			If no issues are found, return an empty array [].`

	tmplStr := systemPrompt + "\n\n" + userPrompt
	tmpl, err := template.New("prompt").Parse(tmplStr)
	if err != nil {
		return template.Must(template.New("fallback").Parse(tmplStr))
	}
	return tmpl
}

// BuildCompactPrompt builds a more compact prompt for smaller context windows
func BuildCompactPrompt(req ReviewRequest) (string, error) {
	contactTemplate := `Analyze this {{.Language}} code for issuses:
	File: {{.FileName}}
	Lines: {{.LineCount}}

	Code:
` + "```" + `{{.Language}}
{{.Content}}
` + "```" + `

	Review focus: security, bugs, performance, maintainability.
	Return JSON array of issues with title, description, severity (critical/warning/info), line, category, suggestions, confidence.
	Empty array if no issues.
	`
	tmpl := template.Must(template.New("compact").Parse(contactTemplate))

	data := map[string]interface{}{
		"FileName":  req.Relative,
		"Language":  req.Language,
		"Content":   req.Content,
		"LineCount": strings.Count(req.Content, "\n") + 1,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute compact prompt: %w", err)
	}

	return buf.String(), nil

}
