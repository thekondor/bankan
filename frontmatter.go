package bankan

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const frontmatterDelimiter = "---"

var errNoFrontmatter = errors.New("no YAML frontmatter found")

// Parse splits a markdown document into its YAML frontmatter and body.
// The frontmatter must be delimited by leading and trailing "---" lines.
// The parsed YAML is unmarshalled into v (must be a pointer).
// Returns the markdown body (everything after the closing ---\n).
func Parse(content []byte, v any) (body string, err error) {
	s := string(content)

	if !strings.HasPrefix(s, frontmatterDelimiter+"\n") {
		return "", errNoFrontmatter
	}

	// Find the closing delimiter (skip the opening one at index 0).
	rest := s[len(frontmatterDelimiter)+1:]
	closeIdx := strings.Index(rest, "\n"+frontmatterDelimiter+"\n")
	if closeIdx == -1 {
		// Try end-of-file variant (no trailing newline after closing ---)
		if idx := strings.Index(rest, "\n"+frontmatterDelimiter); idx != -1 && idx == len(rest)-len(frontmatterDelimiter)-1 {
			closeIdx = idx
		} else {
			return "", fmt.Errorf("frontmatter: closing delimiter not found")
		}
	}

	yamlSrc := rest[:closeIdx]
	if err := yaml.Unmarshal([]byte(yamlSrc), v); err != nil {
		return "", fmt.Errorf("frontmatter: yaml unmarshal: %w", err)
	}

	after := rest[closeIdx+1+len(frontmatterDelimiter):]
	// Strip a single leading newline that separates delimiter from body.
	body = strings.TrimPrefix(after, "\n")
	return body, nil
}

// Serialize marshals v to YAML, wraps it in --- delimiters, and appends body.
func Serialize(v any, body string) ([]byte, error) {
	var buf bytes.Buffer

	yamlBytes, err := yaml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("frontmatter: yaml marshal: %w", err)
	}

	buf.WriteString(frontmatterDelimiter + "\n")
	buf.Write(yamlBytes)
	buf.WriteString(frontmatterDelimiter + "\n")

	if body != "" {
		buf.WriteString(body)
		// Ensure file ends with newline.
		if !strings.HasSuffix(body, "\n") {
			buf.WriteByte('\n')
		}
	}

	return buf.Bytes(), nil
}
