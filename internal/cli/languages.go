package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var SupportedLanguages = map[string][]string{
	"go":         {".go"},
	"java":       {".java"},
	"typescript": {".ts", ".tsx"},
	"javascript": {".js", ".jsx"},
	"python":     {".py"},
	"csharp":     {".cs"},
	"dotnet":     {".cs", ".vb", ".fs"},
}

type LanguageDisplay struct {
	ID   int
	Name string
	key  string
}

var LanguageList = []LanguageDisplay{
	{1, "Go", "go"},
	{2, "Java", "java"},
	{3, "TypeScript", "typescript"},
	{4, "JavaScript", "javascript"},
	{5, "Python", "python"},
	{6, "C#", "csharp"},
	{7, ".NET", "dotnet"},
}

// ParseLanguages processes the --lang flag or prompts interactively
func ParseLanguages(langInput string) ([]string, error) {
	langInput = strings.TrimSpace(langInput)

	if langInput != "" {
		return parseLanguageFlag(langInput)
	}

	return promptForLanguage()

}

// ParseLanguageFlag parses comma-separated language names or keys
func parseLanguageFlag(input string) ([]string, error) {
	parts := strings.Split(strings.ToLower(input), ",")
	var languages []string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if num, err := strconv.Atoi(part); err == nil {
			lang, err := getLanguageByNumber(num)
			if err != nil {
				return nil, fmt.Errorf("Invalid language number %d: %v", num, err)
			}
			languages = append(languages, lang)
			continue
		}

		// Check if language exists
		if _, exists := SupportedLanguages[part]; !exists {
			found := false
			for _, lang := range LanguageList {
				if strings.EqualFold(lang.Name, part) || strings.EqualFold(lang.key, part) {
					languages = append(languages, lang.key)
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("Unsupported Language: %s", part)
			}
		} else {
			languages = append(languages, part)
		}
	}

	languages = duplicate(languages)

	if len(languages) == 0 {
		return nil, fmt.Errorf("No valid languages selected")
	}
	return languages, nil
}

/**
* Display interactive language selection
 */
func promptForLanguage() ([]string, error) {
	fmt.Println("Select languages to review (comma-separated): ")
	for _, lang := range LanguageList {
		fmt.Printf("[%d] %s\n", lang.ID, lang.Name)
	}
	fmt.Print("> ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %v", err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("no languages selected")
	}
	return parseLanguageFlag(input)
}

/**
* Removes duplicate strings from slice
 */

func duplicate(slice []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

/**
*  Getlanguages by Number
 */
func getLanguageByNumber(num int) (string, error) {
	for _, lang := range LanguageList {
		if lang.ID == num {
			return lang.key, nil
		}
	}
	return "", fmt.Errorf("language number %d not found", num)
}
