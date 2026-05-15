package bankan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Card represents a kanban card (ticket). It maps to a single markdown file
// inside a lane directory (or _archive).
type Card struct {
	ID           string     `yaml:"id"`
	Title        string     `yaml:"title"`
	CreatedAt    time.Time  `yaml:"created_at"`
	UpdatedAt    time.Time  `yaml:"updated_at"`
	MovedAt      *time.Time `yaml:"moved_at,omitempty"`
	MovedFrom    string     `yaml:"moved_from,omitempty"`
	ArchivedAt         *time.Time `yaml:"archived_at,omitempty"`
	ArchivedFrom       string     `yaml:"archived_from,omitempty"`
	ArchivedLabelNames []string   `yaml:"archived_label_names,omitempty"` // label names snapshotted at archive time
	Labels             []string   `yaml:"labels,omitempty"`               // label IDs
	PrimaryLabel       string     `yaml:"primary_label,omitempty"`        // primary label ID (not in Labels)

	// Body is the markdown content below the frontmatter (runtime only).
	Body string `yaml:"-"`
	// Lane is the current lane display name (runtime only).
	Lane string `yaml:"-"`
	// FilePath is the absolute path to the card's .md file (runtime only).
	FilePath string `yaml:"-"`
}

// cardFileRe matches filenames of the form NNN-<id>-<slug>.md
// where NNN is a 3-digit order prefix. The slug must not contain '.' so that
// "001-ab12c-fix.comments.md" does not match.
var cardFileRe = regexp.MustCompile(`^(\d{3})-([a-z0-9]{5})-([^.]+)\.md$`)

// parseCardFilename splits "NNN-<id>-<slug>.md" into its parts.
func parseCardFilename(base string) (order int, id, slug string, ok bool) {
	m := cardFileRe.FindStringSubmatch(base)
	if m == nil {
		return 0, "", "", false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, "", "", false
	}
	return n, m[2], m[3], true
}

// cardFilename builds the filename for a card.
func cardFilename(order int, id, slug string) string {
	return fmt.Sprintf("%03d-%s-%s.md", order, id, slug)
}

// commentFilename returns the comments file name corresponding to a card file.
// e.g. "001-ab12c-fix-bug.md" → "001-ab12c-fix-bug.comments.md"
func commentFilename(cardBase string) string {
	withoutExt := strings.TrimSuffix(cardBase, ".md")
	return withoutExt + ".comments.md"
}

// nextOrderInLane returns max(existing order prefixes) + 1, or 1 if empty.
func nextOrderInLane(laneDir string) (int, error) {
	entries, err := os.ReadDir(laneDir)
	if err != nil {
		return 0, fmt.Errorf("next order: %w", err)
	}
	max := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		order, _, _, ok := parseCardFilename(e.Name())
		if ok && order > max {
			max = order
		}
	}
	return max + 1, nil
}

// collectCardIDs scans all lanes and _archive for existing card IDs.
func collectCardIDs(b *Board) (map[string]struct{}, error) {
	ids := make(map[string]struct{})

	add := func(dir string) error {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			_, id, _, ok := parseCardFilename(e.Name())
			if ok {
				ids[id] = struct{}{}
			}
		}
		return nil
	}

	lanes, err := ReadLanes(b.Dir)
	if err != nil {
		return nil, err
	}
	for _, l := range lanes {
		if err := add(l.Dir); err != nil {
			return nil, err
		}
	}
	if err := add(archiveDir(b.Dir)); err != nil {
		return nil, err
	}
	return ids, nil
}

// ReadCard reads and parses a card from filePath.
func ReadCard(filePath string) (*Card, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read card: %w", err)
	}

	var c Card
	body, err := Parse(data, &c)
	if err != nil {
		return nil, fmt.Errorf("read card %s: %w", filePath, err)
	}
	c.Body = body
	c.FilePath = filePath
	return &c, nil
}

// WriteCard serializes and writes the card to c.FilePath, updating UpdatedAt.
func WriteCard(c *Card) error {
	c.UpdatedAt = time.Now().UTC()
	data, err := Serialize(c, c.Body)
	if err != nil {
		return fmt.Errorf("write card: %w", err)
	}
	if err := os.WriteFile(c.FilePath, data, 0o644); err != nil {
		return fmt.Errorf("write card: %w", err)
	}
	return nil
}

// AddCard creates a new card in the given lane.
func AddCard(b *Board, lane Lane, title, body string, labelIDs []string) (*Card, error) {
	if strings.TrimSpace(title) == "" {
		return nil, errors.New("add card: title must not be empty")
	}

	// Validate label IDs exist on the board.
	for _, lid := range labelIDs {
		if _, ok := FindLabelByID(b.Labels, lid); !ok {
			return nil, fmt.Errorf("add card: label %q not found on board", lid)
		}
	}

	existingIDs, err := collectCardIDs(b)
	if err != nil {
		return nil, fmt.Errorf("add card: %w", err)
	}
	id := NewUniqueID(existingIDs)

	order, err := nextOrderInLane(lane.Dir)
	if err != nil {
		return nil, fmt.Errorf("add card: %w", err)
	}

	slug := slugify(title)
	if slug == "" {
		slug = id
	}

	now := time.Now().UTC()
	c := &Card{
		ID:        id,
		Title:     strings.TrimSpace(title),
		CreatedAt: now,
		UpdatedAt: now,
		Labels:    labelIDs,
		Body:      body,
		Lane:      lane.Name,
		FilePath:  filepath.Join(lane.Dir, cardFilename(order, id, slug)),
	}

	data, err := Serialize(c, c.Body)
	if err != nil {
		return nil, fmt.Errorf("add card: serialize: %w", err)
	}
	if err := os.WriteFile(c.FilePath, data, 0o644); err != nil {
		return nil, fmt.Errorf("add card: write: %w", err)
	}
	return c, nil
}

// DuplicateCard creates a copy of source in the same lane. The new card gets
// title "[dup] <source title>", the same body and labels, and a fresh ID.
// Comments are not copied.
func DuplicateCard(b *Board, source *Card) (*Card, error) {
	lanes, err := ReadLanes(b.Dir)
	if err != nil {
		return nil, fmt.Errorf("duplicate card: %w", err)
	}
	var lane Lane
	found := false
	for _, l := range lanes {
		if l.Name == source.Lane {
			lane = l
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("duplicate card: source lane %q not found", source.Lane)
	}
	labelIDs := make([]string, len(source.Labels))
	copy(labelIDs, source.Labels)
	c, err := AddCard(b, lane, "[dup] "+source.Title, source.Body, labelIDs)
	if err != nil {
		return nil, fmt.Errorf("duplicate card: %w", err)
	}
	if source.PrimaryLabel != "" {
		c.PrimaryLabel = source.PrimaryLabel
		if err := WriteCard(c); err != nil {
			return nil, fmt.Errorf("duplicate card: set primary label: %w", err)
		}
	}
	return c, nil
}

// ListCards returns all cards in a lane, sorted by their order prefix.
func ListCards(lane Lane) ([]*Card, error) {
	entries, err := os.ReadDir(lane.Dir)
	if err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
	}

	type entry struct {
		order int
		path  string
	}
	var files []entry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		order, _, _, ok := parseCardFilename(e.Name())
		if !ok {
			continue
		}
		files = append(files, entry{order, filepath.Join(lane.Dir, e.Name())})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].order < files[j].order })

	cards := make([]*Card, 0, len(files))
	for _, f := range files {
		c, err := ReadCard(f.path)
		if err != nil {
			return nil, err
		}
		c.Lane = lane.Name
		cards = append(cards, c)
	}
	return cards, nil
}

// FindCard searches all lanes (and archive if searchArchive is true) for a
// card with the given ID. Returns an error if not found.
func FindCard(b *Board, id string, searchArchive bool) (*Card, error) {
	lanes, err := ReadLanes(b.Dir)
	if err != nil {
		return nil, err
	}

	for _, lane := range lanes {
		c, err := findCardInDir(lane.Dir, lane.Name, id)
		if err != nil {
			return nil, err
		}
		if c != nil {
			return c, nil
		}
	}

	if searchArchive {
		c, err := findArchivedCard(archiveDir(b.Dir), id)
		if err != nil {
			return nil, err
		}
		if c != nil {
			return c, nil
		}
	}

	return nil, fmt.Errorf("card %q not found", id)
}

func findCardInDir(dir, laneName, id string) (*Card, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find card: read dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		_, fid, _, ok := parseCardFilename(e.Name())
		if !ok || fid != id {
			continue
		}
		c, err := ReadCard(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		c.Lane = laneName
		return c, nil
	}
	return nil, nil
}

// archCardRe matches archive card filenames: "<id>-<slug>.md"
var archCardRe = regexp.MustCompile(`^([a-z0-9]{5})-([^.]+)\.md$`)

func findArchivedCard(archDir, id string) (*Card, error) {
	entries, err := os.ReadDir(archDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find archived card: read dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := archCardRe.FindStringSubmatch(e.Name())
		if m == nil || m[1] != id {
			continue
		}
		c, err := ReadCard(filepath.Join(archDir, e.Name()))
		if err != nil {
			return nil, err
		}
		c.Lane = archiveDirName
		return c, nil
	}
	return nil, nil
}

// MoveCard moves a card (and its comments file) from its current lane to
// toLane. It records moved_at and moved_from on the card.
func MoveCard(b *Board, c *Card, toLane Lane) error {
	if c.Lane == toLane.Name {
		return nil // nothing to do
	}

	srcBase := filepath.Base(c.FilePath)
	_, id, slug, ok := parseCardFilename(srcBase)
	if !ok {
		return fmt.Errorf("move card: unexpected filename %q", srcBase)
	}

	order, err := nextOrderInLane(toLane.Dir)
	if err != nil {
		return fmt.Errorf("move card: %w", err)
	}

	newBase := cardFilename(order, id, slug)
	newPath := filepath.Join(toLane.Dir, newBase)

	now := time.Now().UTC()
	prevLane := c.Lane
	c.MovedAt = &now
	c.MovedFrom = prevLane
	c.Lane = toLane.Name
	c.UpdatedAt = now

	// Write updated frontmatter to the new path before moving the file.
	data, err := Serialize(c, c.Body)
	if err != nil {
		return fmt.Errorf("move card: serialize: %w", err)
	}
	if err := os.WriteFile(newPath, data, 0o644); err != nil {
		return fmt.Errorf("move card: write new: %w", err)
	}

	// Move comments file if it exists.
	srcComments := filepath.Join(filepath.Dir(c.FilePath), commentFilename(srcBase))
	dstComments := filepath.Join(toLane.Dir, commentFilename(newBase))
	if _, err := os.Stat(srcComments); err == nil {
		if err := os.Rename(srcComments, dstComments); err != nil {
			return fmt.Errorf("move card: move comments: %w", err)
		}
	}

	// Remove old card file.
	if err := os.Remove(c.FilePath); err != nil {
		return fmt.Errorf("move card: remove old: %w", err)
	}

	c.FilePath = newPath
	return nil
}

// ArchiveCard moves a card (and comments) to the _archive directory, stripping
// the order prefix from the filename.
func ArchiveCard(b *Board, c *Card) error {
	if c.ArchivedAt != nil {
		return errors.New("archive card: card is already archived")
	}

	archDir := archiveDir(b.Dir)
	if err := os.MkdirAll(archDir, 0o755); err != nil {
		return fmt.Errorf("archive card: mkdir: %w", err)
	}

	srcBase := filepath.Base(c.FilePath)
	_, id, slug, ok := parseCardFilename(srcBase)
	if !ok {
		return fmt.Errorf("archive card: unexpected filename %q", srcBase)
	}

	// Archive files have no order prefix.
	archBase := fmt.Sprintf("%s-%s.md", id, slug)
	archPath := filepath.Join(archDir, archBase)

	now := time.Now().UTC()
	c.ArchivedAt = &now
	c.ArchivedFrom = c.Lane
	c.UpdatedAt = now
	c.Lane = archiveDirName

	// Snapshot label names so they remain readable after a label is deleted.
	c.ArchivedLabelNames = nil
	for _, lid := range c.Labels {
		if l, ok := FindLabelByID(b.Labels, lid); ok {
			c.ArchivedLabelNames = append(c.ArchivedLabelNames, l.Name)
		}
	}

	data, err := Serialize(c, c.Body)
	if err != nil {
		return fmt.Errorf("archive card: serialize: %w", err)
	}
	if err := os.WriteFile(archPath, data, 0o644); err != nil {
		return fmt.Errorf("archive card: write: %w", err)
	}

	// Move comments.
	srcComments := filepath.Join(filepath.Dir(c.FilePath), commentFilename(srcBase))
	dstComments := filepath.Join(archDir, fmt.Sprintf("%s-%s.comments.md", id, slug))
	if _, err := os.Stat(srcComments); err == nil {
		if err := os.Rename(srcComments, dstComments); err != nil {
			return fmt.Errorf("archive card: move comments: %w", err)
		}
	}

	if err := os.Remove(c.FilePath); err != nil {
		return fmt.Errorf("archive card: remove original: %w", err)
	}

	c.FilePath = archPath
	return nil
}

// RestoreCard moves a card from _archive back to a lane.
func RestoreCard(b *Board, c *Card, toLane Lane) error {
	if c.ArchivedAt == nil {
		return errors.New("restore card: card is not archived")
	}

	srcBase := filepath.Base(c.FilePath)
	// Archive filenames are "<id>-<slug>.md" (no order prefix).
	withoutExt := strings.TrimSuffix(srcBase, ".md")
	parts := strings.SplitN(withoutExt, "-", 2)
	if len(parts) != 2 {
		return fmt.Errorf("restore card: unexpected archive filename %q", srcBase)
	}
	id, slug := parts[0], parts[1]

	order, err := nextOrderInLane(toLane.Dir)
	if err != nil {
		return fmt.Errorf("restore card: %w", err)
	}

	newBase := cardFilename(order, id, slug)
	newPath := filepath.Join(toLane.Dir, newBase)

	now := time.Now().UTC()
	c.ArchivedAt = nil
	c.ArchivedFrom = ""
	c.ArchivedLabelNames = nil
	c.Lane = toLane.Name
	c.UpdatedAt = now

	data, err := Serialize(c, c.Body)
	if err != nil {
		return fmt.Errorf("restore card: serialize: %w", err)
	}
	if err := os.WriteFile(newPath, data, 0o644); err != nil {
		return fmt.Errorf("restore card: write: %w", err)
	}

	// Restore comments if they exist.
	srcComments := filepath.Join(archiveDir(b.Dir), fmt.Sprintf("%s-%s.comments.md", id, slug))
	dstComments := filepath.Join(toLane.Dir, commentFilename(newBase))
	if _, err := os.Stat(srcComments); err == nil {
		if err := os.Rename(srcComments, dstComments); err != nil {
			return fmt.Errorf("restore card: move comments: %w", err)
		}
	}

	if err := os.Remove(c.FilePath); err != nil {
		return fmt.Errorf("restore card: remove archive copy: %w", err)
	}

	c.FilePath = newPath
	return nil
}

// reorderCardsInLane is the shared reorder kernel. It takes an already-ordered
// slice of cards, redistributes the existing set of order-prefixes to reflect
// the caller's desired ordering, and renames files on disk using a two-phase
// temp-rename to avoid name collisions.
func reorderCardsInLane(laneDir string, ordered []*Card) error {
	type info struct {
		card  *Card
		order int
		id    string
		slug  string
	}
	entries := make([]info, len(ordered))
	prefixes := make([]int, len(ordered))
	for i, c := range ordered {
		o, id, slug, _ := parseCardFilename(filepath.Base(c.FilePath))
		entries[i] = info{c, o, id, slug}
		prefixes[i] = o
	}
	sort.Ints(prefixes)

	type fileRename struct {
		from        string
		to          string
		fromComment string
		toComment   string
	}
	var renames []fileRename
	for i, e := range entries {
		wantOrder := prefixes[i]
		if e.order == wantOrder {
			continue
		}
		oldBase := filepath.Base(e.card.FilePath)
		newBase := cardFilename(wantOrder, e.id, e.slug)
		renames = append(renames, fileRename{
			from:        e.card.FilePath,
			to:          filepath.Join(laneDir, newBase),
			fromComment: filepath.Join(laneDir, commentFilename(oldBase)),
			toComment:   filepath.Join(laneDir, commentFilename(newBase)),
		})
	}

	type tempEntry struct {
		tmp        string
		dst        string
		tmpComment string
		dstComment string
		hasComment bool
	}
	temps := make([]tempEntry, 0, len(renames))
	for i, r := range renames {
		tmp := filepath.Join(laneDir, fmt.Sprintf("_reorder_card_%d_", i))
		tmpComment := tmp + ".comments"
		if err := os.Rename(r.from, tmp); err != nil {
			return fmt.Errorf("reorder: temp rename: %w", err)
		}
		hasComment := false
		if _, err := os.Stat(r.fromComment); err == nil {
			if err := os.Rename(r.fromComment, tmpComment); err != nil {
				return fmt.Errorf("reorder: temp rename comments: %w", err)
			}
			hasComment = true
		}
		temps = append(temps, tempEntry{tmp, r.to, tmpComment, r.toComment, hasComment})
	}
	for _, t := range temps {
		if err := os.Rename(t.tmp, t.dst); err != nil {
			return fmt.Errorf("reorder: final rename: %w", err)
		}
		if t.hasComment {
			if err := os.Rename(t.tmpComment, t.dstComment); err != nil {
				return fmt.Errorf("reorder: final rename comments: %w", err)
			}
		}
	}
	return nil
}

// ReorderCard changes the position of movedCardID within lane.
// newIndex is 0-based among all cards in the lane sorted by order prefix.
func ReorderCard(lane Lane, movedCardID string, newIndex int) error {
	cards, err := ListCards(lane)
	if err != nil {
		return fmt.Errorf("reorder card: %w", err)
	}
	if newIndex < 0 || newIndex >= len(cards) {
		return fmt.Errorf("reorder card: index %d out of range [0, %d)", newIndex, len(cards))
	}
	oldIndex := -1
	for i, c := range cards {
		if c.ID == movedCardID {
			oldIndex = i
			break
		}
	}
	if oldIndex == -1 {
		return fmt.Errorf("reorder card: card %q not found in lane %q", movedCardID, lane.Name)
	}
	if oldIndex == newIndex {
		return nil
	}

	moved := cards[oldIndex]
	newOrder := make([]*Card, 0, len(cards))
	for i, c := range cards {
		if i == oldIndex {
			continue
		}
		newOrder = append(newOrder, c)
	}
	newOrder = append(newOrder[:newIndex:newIndex], append([]*Card{moved}, newOrder[newIndex:]...)...)

	return reorderCardsInLane(lane.Dir, newOrder)
}

// ReorderCardAmongLabeled changes the position of movedCardID within the
// subset of cards in lane that carry filterLabelID. Non-labelled cards keep
// their positions relative to each other. newIndex is 0-based within the
// labelled subset.
func ReorderCardAmongLabeled(lane Lane, movedCardID, filterLabelID string, newIndex int) error {
	cards, err := ListCards(lane)
	if err != nil {
		return fmt.Errorf("reorder card among labeled: %w", err)
	}

	var labeled []*Card
	for _, c := range cards {
		for _, lid := range c.Labels {
			if lid == filterLabelID {
				labeled = append(labeled, c)
				break
			}
		}
	}

	if newIndex < 0 || newIndex >= len(labeled) {
		return fmt.Errorf("reorder card among labeled: index %d out of range [0, %d)", newIndex, len(labeled))
	}
	oldIndex := -1
	for i, c := range labeled {
		if c.ID == movedCardID {
			oldIndex = i
			break
		}
	}
	if oldIndex == -1 {
		return fmt.Errorf("reorder card among labeled: card %q not found in labelled subset", movedCardID)
	}
	if oldIndex == newIndex {
		return nil
	}

	moved := labeled[oldIndex]
	newLabeled := make([]*Card, 0, len(labeled))
	for i, c := range labeled {
		if i == oldIndex {
			continue
		}
		newLabeled = append(newLabeled, c)
	}
	newLabeled = append(newLabeled[:newIndex:newIndex], append([]*Card{moved}, newLabeled[newIndex:]...)...)

	return reorderCardsInLane(lane.Dir, newLabeled)
}

// DeleteCard permanently removes a card file and its comments file.
func DeleteCard(c *Card) error {
	// Remove comments first (best-effort).
	srcBase := filepath.Base(c.FilePath)
	commentsPath := filepath.Join(filepath.Dir(c.FilePath), commentFilename(srcBase))
	_ = os.Remove(commentsPath) // ignore if missing

	if err := os.Remove(c.FilePath); err != nil {
		return fmt.Errorf("delete card: %w", err)
	}
	return nil
}

// ListArchivedCards returns all cards from the _archive directory.
func ListArchivedCards(b *Board) ([]*Card, error) {
	archDir := archiveDir(b.Dir)
	entries, err := os.ReadDir(archDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list archived cards: %w", err)
	}

	var cards []*Card
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".comments.md") {
			continue
		}
		if !archCardRe.MatchString(e.Name()) {
			continue
		}
		c, err := ReadCard(filepath.Join(archDir, e.Name()))
		if err != nil {
			return nil, err
		}
		c.Lane = archiveDirName
		cards = append(cards, c)
	}
	return cards, nil
}
