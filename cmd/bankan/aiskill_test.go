package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runAISkill executes the ai-skill command with the given extra args and returns
// any error. outDir is always appended as the positional argument.
func runAISkill(t *testing.T, outDir string, extraArgs ...string) error {
	t.Helper()
	cmd := newAISkillCmd()
	cmd.SetArgs(append(extraArgs, outDir))
	// Suppress cobra usage output on errors.
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return cmd.Execute()
}

// readSkillMD reads <outDir>/bankan/SKILL.md and returns its contents.
func readSkillMD(t *testing.T, outDir string) string {
	t.Helper()
	p := filepath.Join(outDir, "bankan", "SKILL.md")
	data, err := os.ReadFile(p)
	require.NoError(t, err, "SKILL.md should exist at %s", p)
	return string(data)
}

func TestAISkillCmd_MissingType(t *testing.T) {
	outDir := t.TempDir()
	err := runAISkill(t, outDir /* no --type */)
	require.Error(t, err)
}

func TestAISkillCmd_InvalidType(t *testing.T) {
	outDir := t.TempDir()
	err := runAISkill(t, outDir, "--type", "unknown-agent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown --type")
}

// assertSkillContent verifies that content has valid frontmatter and covers the
// full CLI surface an agent needs to operate bankan correctly.
func assertSkillContent(t *testing.T, content string) {
	t.Helper()

	// Frontmatter.
	assert.True(t, strings.HasPrefix(content, "---\n"), "SKILL.md must start with YAML frontmatter")
	assert.Contains(t, content, "name: bankan")
	assert.Contains(t, content, "description:")

	// Lane commands.
	assert.Contains(t, content, "lane list")
	assert.Contains(t, content, "lane add")
	assert.Contains(t, content, "lane rename")
	assert.Contains(t, content, "lane remove")

	// Card commands — full set.
	assert.Contains(t, content, "card list")
	assert.Contains(t, content, "card show")
	assert.Contains(t, content, "card add")
	assert.Contains(t, content, "card edit")
	assert.Contains(t, content, "card move")
	assert.Contains(t, content, "card archive")
	assert.Contains(t, content, "card restore")
	assert.Contains(t, content, "card delete")
	assert.Contains(t, content, "--archived")   // listing archived cards
	assert.Contains(t, content, "--add-label")  // label management on edit
	assert.Contains(t, content, "--force")      // delete confirmation

	// Comment commands with correct positional syntax (not --card flag).
	assert.Contains(t, content, "comment list")
	assert.Contains(t, content, "comment add")
	assert.Contains(t, content, "comment edit")
	assert.NotContains(t, content, "comment list --card", "comment list takes a positional arg, not --card")
	assert.NotContains(t, content, "comment add --card", "comment add takes a positional arg, not --card")

	// Label commands — full set.
	assert.Contains(t, content, "label list")
	assert.Contains(t, content, "label add")
	assert.Contains(t, content, "label edit")
	assert.Contains(t, content, "label remove")

	// Board.
	assert.Contains(t, content, "board show")

	// Key rules an agent must know.
	assert.Contains(t, content, "case-sensitive")  // lane name gotcha
	assert.Contains(t, content, "label ID", "agent must know --label takes an ID not a name")

	// Stdin support — agents must know about the - pseudo-argument for multi-line content.
	assert.Contains(t, content, "stdin", "SKILL.md must document stdin via - pseudo-argument")
}

func TestAISkillCmd_ClaudeCode(t *testing.T) {
	outDir := t.TempDir()
	require.NoError(t, runAISkill(t, outDir, "--type", "claude-code"))
	assertSkillContent(t, readSkillMD(t, outDir))
}

func TestAISkillCmd_OpenCode(t *testing.T) {
	outDir := t.TempDir()
	require.NoError(t, runAISkill(t, outDir, "--type", "opencode"))
	assertSkillContent(t, readSkillMD(t, outDir))
}

func TestAISkillCmd_Codex(t *testing.T) {
	outDir := t.TempDir()
	require.NoError(t, runAISkill(t, outDir, "--type", "codex"))
	assertSkillContent(t, readSkillMD(t, outDir))
}

func TestAISkillCmd_OutputStructure(t *testing.T) {
	for _, tt := range []struct {
		skillTyp string
	}{
		{"claude-code"},
		{"opencode"},
		{"codex"},
	} {
		t.Run(tt.skillTyp, func(t *testing.T) {
			outDir := t.TempDir()
			err := runAISkill(t, outDir, "--type", tt.skillTyp)
			require.NoError(t, err)

			// The skill directory must exist.
			skillDir := filepath.Join(outDir, "bankan")
			info, err := os.Stat(skillDir)
			require.NoError(t, err, "bankan/ directory should exist")
			assert.True(t, info.IsDir())

			// SKILL.md must be a regular file.
			skillFile := filepath.Join(skillDir, "SKILL.md")
			fi, err := os.Stat(skillFile)
			require.NoError(t, err, "SKILL.md should exist")
			assert.False(t, fi.IsDir())
			assert.Greater(t, fi.Size(), int64(0))
		})
	}
}

func TestAISkillCmd_WithBinPath(t *testing.T) {
	for _, tt := range []struct {
		skillTyp string
	}{
		{"claude-code"},
		{"opencode"},
		{"codex"},
	} {
		t.Run(tt.skillTyp, func(t *testing.T) {
			outDir := t.TempDir()
			err := runAISkill(t, outDir, "--type", tt.skillTyp, "--with-bin-path")
			require.NoError(t, err)

			content := readSkillMD(t, outDir)
			// With --with-bin-path, the binary path (not just "bankan") is used.
			// It will be an absolute path, so it should contain a path separator.
			assert.Contains(t, content, string(filepath.Separator))
		})
	}
}

func TestAISkillCmd_OpenCodeCodexShareTemplate(t *testing.T) {
	// OpenCode and Codex share the same template; their SKILL.md body should be
	// identical when --with-bin-path is not used.
	openDir := t.TempDir()
	codexDir := t.TempDir()

	require.NoError(t, runAISkill(t, openDir, "--type", "opencode"))
	require.NoError(t, runAISkill(t, codexDir, "--type", "codex"))

	openContent := readSkillMD(t, openDir)
	codexContent := readSkillMD(t, codexDir)

	assert.Equal(t, openContent, codexContent, "opencode and codex should produce identical SKILL.md")
}
