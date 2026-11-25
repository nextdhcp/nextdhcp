package test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// shellLanguages are the languages of code blocks that show shell commands.
var shellLanguages = map[string]bool{
	"sh":      true,
	"bash":    true,
	"shell":   true,
	"console": true,
}

// Examples returns the code blocks of the Markdown file at path, like the
// README of a plugin, that are examples, so tests can check that they work.
// That's all code blocks except shell commands (blocks marked as sh, bash,
// shell or console) and the syntax that blocks directly in a "Syntax"
// section describe.
func Examples(t *testing.T, path string) []string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var (
		examples     []string
		block        strings.Builder
		inBlock      bool
		isExample    bool
		section      string // the current level 2 heading
		inSubsection bool   // whether a level 3 heading follows it
	)

	for _, line := range strings.Split(string(content), "\n") {
		switch {
		case inBlock && strings.HasPrefix(line, "```"):
			if isExample {
				examples = append(examples, block.String())
			}
			block.Reset()
			inBlock = false
		case inBlock:
			block.WriteString(line + "\n")
		case strings.HasPrefix(line, "## "):
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			inSubsection = false
		case strings.HasPrefix(line, "### "):
			inSubsection = true
		case strings.HasPrefix(line, "```"):
			language := strings.TrimSpace(strings.TrimPrefix(line, "```"))
			isExample = !shellLanguages[language] && (section != "Syntax" || inSubsection)
			inBlock = true
		}
	}

	return examples
}
