package bankan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testFM struct {
	Title string `yaml:"title"`
	Count int    `yaml:"count"`
}

func TestParse_ValidFrontmatter(t *testing.T) {
	input := "---\ntitle: Hello\ncount: 42\n---\nBody text here.\n"
	var fm testFM
	body, err := Parse([]byte(input), &fm)
	require.NoError(t, err)
	assert.Equal(t, "Hello", fm.Title)
	assert.Equal(t, 42, fm.Count)
	assert.Equal(t, "Body text here.\n", body)
}

func TestParse_EmptyBody(t *testing.T) {
	input := "---\ntitle: No body\ncount: 0\n---\n"
	var fm testFM
	body, err := Parse([]byte(input), &fm)
	require.NoError(t, err)
	assert.Equal(t, "No body", fm.Title)
	assert.Equal(t, "", body)
}

func TestParse_NoFrontmatter(t *testing.T) {
	input := "Just a plain markdown file.\n"
	var fm testFM
	_, err := Parse([]byte(input), &fm)
	assert.ErrorIs(t, err, errNoFrontmatter)
}

func TestParse_MissingClosingDelimiter(t *testing.T) {
	input := "---\ntitle: broken\n"
	var fm testFM
	_, err := Parse([]byte(input), &fm)
	assert.Error(t, err)
}

func TestSerialize_RoundTrip(t *testing.T) {
	fm := testFM{Title: "Round Trip", Count: 7}
	body := "Some **markdown** body.\n"

	data, err := Serialize(fm, body)
	require.NoError(t, err)

	var fm2 testFM
	body2, err := Parse(data, &fm2)
	require.NoError(t, err)

	assert.Equal(t, fm, fm2)
	assert.Equal(t, body, body2)
}

func TestSerialize_AddsTrailingNewline(t *testing.T) {
	fm := testFM{Title: "x", Count: 1}
	data, err := Serialize(fm, "no newline at end")
	require.NoError(t, err)
	assert.Equal(t, byte('\n'), data[len(data)-1])
}

func TestSerialize_EmptyBody(t *testing.T) {
	fm := testFM{Title: "empty", Count: 0}
	data, err := Serialize(fm, "")
	require.NoError(t, err)

	var fm2 testFM
	body, err := Parse(data, &fm2)
	require.NoError(t, err)
	assert.Equal(t, fm, fm2)
	assert.Equal(t, "", body)
}
