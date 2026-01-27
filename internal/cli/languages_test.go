package cli

import "testing"

func TestParseLanguageFlag(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
		wantErr  bool
	}{
		{
			name:     "single language by name",
			input:    "go",
			expected: []string{"go"},
			wantErr:  false,
		},
		{
			name:     "multiple languages by name",
			input:    "go,python,javascript",
			expected: []string{"go", "python", "javascript"},
			wantErr:  false,
		},
		{
			name:     "with spaces",
			input:    "go, python , javascript",
			expected: []string{"go", "python", "javascript"},
			wantErr:  false,
		},
		{
			name:     "single number",
			input:    "1",
			expected: []string{"go"},
			wantErr:  false,
		},
		{
			name:     "multiple numbers",
			input:    "1,3,5",
			expected: []string{"go", "typescript", "python"},
			wantErr:  false,
		},
		{
			name:     "mixed numbers and names",
			input:    "1,java,3",
			expected: []string{"go", "java", "typescript"},
			wantErr:  false,
		},
		{
			name:     "case insensitive names",
			input:    "Go,PYTHON,TypeScript",
			expected: []string{"go", "python", "typescript"},
			wantErr:  false,
		},
		{
			name:     "duplicate removal",
			input:    "go,go,go",
			expected: []string{"go"},
			wantErr:  false,
		},
		{
			name:     "invalid language name",
			input:    "go,invalidlang",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid number",
			input:    "99",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "empty input",
			input:    "",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "empty between commas",
			input:    "go,,python",
			expected: []string{"go", "python"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLanguageFlag(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseLanguageFlag(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseLanguageFlag(%q) unexpected error: %v", tt.input, err)
				return
			}

			if len(got) != len(tt.expected) {
				t.Errorf("parseLanguageFlag(%q) got %v, want %v", tt.input, got, tt.expected)
				return
			}

			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("parseLanguageFlag(%q) got %v, want %v", tt.input, got, tt.expected)
					break
				}
			}
		})
	}
}

func TestGetLanguageByNumber(t *testing.T) {
	tests := []struct {
		number   int
		expected string
		wantErr  bool
	}{
		{1, "go", false},
		{2, "java", false},
		{3, "typescript", false},
		{4, "javascript", false},
		{5, "python", false},
		{6, "csharp", false},
		{7, "dotnet", false},
		{0, "", true},
		{8, "", true},
		{-1, "", true},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.number+'0')), func(t *testing.T) {
			got, err := getLanguageByNumber(tt.number)

			if tt.wantErr {
				if err == nil {
					t.Errorf("getLanguageByNumber(%d) expected error, got nil", tt.number)
				}
				return
			}

			if err != nil {
				t.Errorf("getLanguageByNumber(%d) unexpected error: %v", tt.number, err)
				return
			}

			if got != tt.expected {
				t.Errorf("getLanguageByNumber(%d) = %q, want %q", tt.number, got, tt.expected)
			}
		})
	}
}

func TestDeduplicate(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no duplicates",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "all duplicates",
			input:    []string{"a", "a", "a"},
			expected: []string{"a"},
		},
		{
			name:     "mixed duplicates",
			input:    []string{"a", "b", "a", "c", "b"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deduplicate(tt.input)

			if len(got) != len(tt.expected) {
				t.Errorf("deduplicate(%v) = %v, want %v", tt.input, got, tt.expected)
				return
			}

			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("deduplicate(%v) = %v, want %v", tt.input, got, tt.expected)
					break
				}
			}
		})
	}
}
