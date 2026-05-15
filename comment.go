package bankan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Comment is a single entry in a card's comments file.
type Comment struct {
	ID        string
	CreatedAt time.Time
	Author    string
	Body      string
}

// commentSectionSep is the horizontal rule used between comment sections.
const commentSectionSep = "\n---\n"

// commentHeadingPrefix is the H2 marker for comment headings.
const commentHeadingPrefix = "## "

// commentHeaderSep is the separator within the H2 heading line.
const commentHeaderSep = " · "

// commentsFilePath returns the comments file path for a given card file path.
func commentsFilePath(cardFilePath string) string {
	dir := filepath.Dir(cardFilePath)
	base := filepath.Base(cardFilePath)
	return filepath.Join(dir, commentFilename(base))
}

// parseCommentHeader parses a comment H2 heading line of the form:
//
//	## <id> · <RFC3339> · <author>
func parseCommentHeader(line string) (id string, ts time.Time, author string, ok bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, commentHeadingPrefix) {
		return "", time.Time{}, "", false
	}
	line = strings.TrimPrefix(line, commentHeadingPrefix)

	parts := strings.SplitN(line, commentHeaderSep, 3)
	if len(parts) != 3 {
		return "", time.Time{}, "", false
	}

	t, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[1]))
	if err != nil {
		return "", time.Time{}, "", false
	}

	return strings.TrimSpace(parts[0]), t, strings.TrimSpace(parts[2]), true
}

// formatCommentHeader formats the H2 heading for a comment.
func formatCommentHeader(c Comment) string {
	return fmt.Sprintf("%s%s%s%s%s%s",
		commentHeadingPrefix,
		c.ID,
		commentHeaderSep,
		c.CreatedAt.UTC().Format(time.RFC3339),
		commentHeaderSep,
		c.Author,
	)
}

// ReadComments reads and parses all comments from the card's comments file.
// Returns an empty slice (no error) if the file does not exist.
func ReadComments(cardFilePath string) ([]Comment, error) {
	path := commentsFilePath(cardFilePath)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read comments: %w", err)
	}
	return parseComments(string(data))
}

// parseComments parses the raw content of a comments file into a slice of Comment.
// Each comment block begins with a line matching the H2 heading format.
// Lines starting with "---" (HR separators) are treated as block delimiters and ignored.
// All other lines between headings are accumulated as the comment body.
func parseComments(content string) ([]Comment, error) {
	lines := strings.Split(content, "\n")

	var comments []Comment
	var current *Comment
	var bodyLines []string

	flush := func() {
		if current == nil {
			return
		}
		current.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
		comments = append(comments, *current)
		current = nil
		bodyLines = nil
	}

	for _, line := range lines {
		id, ts, author, ok := parseCommentHeader(line)
		if ok {
			flush()
			current = &Comment{ID: id, CreatedAt: ts, Author: author}
			continue
		}
		if current == nil {
			// Lines before the first comment heading (e.g. "# Comments: id") are skipped.
			continue
		}
		// Skip HR separator lines.
		if strings.TrimSpace(line) == "---" {
			continue
		}
		bodyLines = append(bodyLines, line)
	}
	flush()

	return comments, nil
}

// SerializeComments renders a full comments file from a slice of Comments,
// including the file header.
func SerializeComments(cardID string, comments []Comment) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Comments: %s\n", cardID)

	for i, c := range comments {
		sb.WriteString("\n")
		sb.WriteString(formatCommentHeader(c))
		sb.WriteString("\n\n")
		body := strings.TrimSpace(c.Body)
		if body != "" {
			sb.WriteString(body)
			sb.WriteString("\n")
		}
		if i < len(comments)-1 {
			sb.WriteString(commentSectionSep)
		}
	}
	return sb.String()
}

// UpdateComment replaces the body of the comment identified by commentID.
// Returns the updated Comment or an error if the comment is not found.
func UpdateComment(cardFilePath, commentID, body string) (*Comment, error) {
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("update comment: body must not be empty")
	}

	existing, err := ReadComments(cardFilePath)
	if err != nil {
		return nil, err
	}

	idx := -1
	for i, c := range existing {
		if c.ID == commentID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, fmt.Errorf("update comment: comment %q not found", commentID)
	}

	existing[idx].Body = strings.TrimSpace(body)

	c, err := ReadCard(cardFilePath)
	if err != nil {
		return nil, fmt.Errorf("update comment: read card: %w", err)
	}

	content := SerializeComments(c.ID, existing)
	path := commentsFilePath(cardFilePath)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("update comment: write: %w", err)
	}
	return &existing[idx], nil
}

// AddComment appends a new comment to the card's comments file.
// The file is created if it does not exist.
func AddComment(cardFilePath, author, body string) (*Comment, error) {
	if strings.TrimSpace(author) == "" {
		return nil, fmt.Errorf("add comment: author must not be empty")
	}
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("add comment: body must not be empty")
	}

	existing, err := ReadComments(cardFilePath)
	if err != nil {
		return nil, err
	}

	// Collect existing comment IDs for uniqueness.
	existingIDs := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		existingIDs[c.ID] = struct{}{}
	}

	// Read the card to get its ID for the file header.
	c, err := ReadCard(cardFilePath)
	if err != nil {
		return nil, fmt.Errorf("add comment: read card: %w", err)
	}

	comment := Comment{
		ID:        NewUniqueID(existingIDs),
		CreatedAt: time.Now().UTC(),
		Author:    strings.TrimSpace(author),
		Body:      strings.TrimSpace(body),
	}

	all := append(existing, comment)
	content := SerializeComments(c.ID, all)

	path := commentsFilePath(cardFilePath)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("add comment: write: %w", err)
	}
	return &comment, nil
}
