package test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Examples returns the code blocks of the "Examples" section of the Markdown
// file at path, like the README of a plugin, so tests can check that they
// work.
func Examples(t *testing.T, path string) []string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var (
		examples  []string
		block     strings.Builder
		inSection bool
		inBlock   bool
	)

	for _, line := range strings.Split(string(content), "\n") {
		switch {
		case inBlock && strings.HasPrefix(line, "```"):
			examples = append(examples, block.String())
			block.Reset()
			inBlock = false
		case inBlock:
			block.WriteString(line + "\n")
		case strings.HasPrefix(line, "## "):
			inSection = strings.TrimSpace(line) == "## Examples"
		case inSection && strings.HasPrefix(line, "```"):
			inBlock = true
		}
	}

	return examples
}
