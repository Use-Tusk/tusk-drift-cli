package agent

import (
	"testing"
)

func TestExtractCommandPrefixes(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected []string
	}{
		{
			name:     "simple npm install",
			command:  "npm install @tusk/drift-sdk",
			expected: []string{"npm install"},
		},
		{
			name:     "npm run",
			command:  "npm run dev",
			expected: []string{"npm run"},
		},
		{
			name:     "pip install",
			command:  "pip install -r requirements.txt",
			expected: []string{"pip install"},
		},
		{
			name:     "simple command without subcommand",
			command:  "cat package.json",
			expected: []string{"cat"},
		},
		{
			name:     "uvicorn (no subcommand)",
			command:  "uvicorn app.main:app --reload",
			expected: []string{"uvicorn"},
		},
		{
			name:     "chained commands",
			command:  "cd /app && npm install",
			expected: []string{"cd", "npm install"},
		},
		{
			name:     "chained with same prefix deduped",
			command:  "npm install foo && npm install bar",
			expected: []string{"npm install"},
		},
		{
			name:     "mixed chained commands",
			command:  "cd src && npm run build",
			expected: []string{"cd", "npm run"},
		},
		{
			name:     "git commands",
			command:  "git commit -m 'test'",
			expected: []string{"git commit"},
		},
		{
			name:     "docker commands",
			command:  "docker build -t myapp .",
			expected: []string{"docker build"},
		},
		{
			name:     "command with flags first",
			command:  "npm -v",
			expected: []string{"npm"},
		},
		{
			name:     "pipe operator",
			command:  "cat file.txt | grep pattern",
			expected: []string{"cat", "grep"},
		},
		{
			name:     "semicolon separator with trivial command filtered",
			command:  "echo hello; npm install",
			expected: []string{"npm install"},
		},
		{
			name:     "or operator",
			command:  "npm install || pip install fallback",
			expected: []string{"npm install", "pip install"},
		},
		{
			name:     "empty command",
			command:  "",
			expected: []string{},
		},
		{
			name:     "whitespace only",
			command:  "   ",
			expected: []string{},
		},
		{
			name:     "trivial true filtered out",
			command:  "npm install || true",
			expected: []string{"npm install"},
		},
		{
			name:     "trivial false filtered out",
			command:  "npm install || false",
			expected: []string{"npm install"},
		},
		{
			name:     "trivial echo filtered out",
			command:  "echo 'starting' && npm run build",
			expected: []string{"npm run"},
		},
		{
			name:     "trivial test filtered out",
			command:  "test -f file.txt && cat file.txt",
			expected: []string{"cat"},
		},
		{
			name:     "cd is NOT filtered (meaningful)",
			command:  "cd /app && npm install || true",
			expected: []string{"cd", "npm install"},
		},
		{
			name:     "only trivial commands returns empty",
			command:  "echo hello || true",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCommandPrefixes(tt.command)

			if len(result) != len(tt.expected) {
				t.Errorf("extractCommandPrefixes(%q) = %v, want %v", tt.command, result, tt.expected)
				return
			}

			// Check each expected prefix is in result (order may vary due to map iteration)
			expectedSet := make(map[string]bool)
			for _, e := range tt.expected {
				expectedSet[e] = true
			}
			for _, r := range result {
				if !expectedSet[r] {
					t.Errorf("extractCommandPrefixes(%q) = %v, want %v", tt.command, result, tt.expected)
					return
				}
			}
		})
	}
}

func TestAllCommandPrefixesAllowed(t *testing.T) {
	agent := &Agent{
		allowedCommandPrefixes: map[string]bool{
			"npm install": true,
			"cd":          true,
		},
	}

	tests := []struct {
		name     string
		prefixes []string
		expected bool
	}{
		{
			name:     "all allowed",
			prefixes: []string{"npm install", "cd"},
			expected: true,
		},
		{
			name:     "partial allowed",
			prefixes: []string{"npm install", "pip install"},
			expected: false,
		},
		{
			name:     "none allowed",
			prefixes: []string{"pip install"},
			expected: false,
		},
		{
			name:     "empty prefixes (trivial commands auto-allowed)",
			prefixes: []string{},
			expected: true,
		},
		{
			name:     "single allowed",
			prefixes: []string{"npm install"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.allCommandPrefixesAllowed(tt.prefixes)
			if result != tt.expected {
				t.Errorf("allCommandPrefixesAllowed(%v) = %v, want %v", tt.prefixes, result, tt.expected)
			}
		})
	}
}
