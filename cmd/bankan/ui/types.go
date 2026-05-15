package ui

import (
	"fmt"
	"sort"
	"strings"
	bankan "github.com/thekondor/bankan"
)

// ─── shared data types used across templates ──────────────────────────────

type BoardData struct {
	ID         string
	Name       string
	IsView     bool
	Color      string
	IsArchived bool // true when the view board has been archived
	IsHidden   bool // true when the board is hidden from the tab bar
}

type LaneWithCards struct {
	Lane      bankan.Lane
	Cards     []*bankan.Card
	IsVirtual bool // true for the fallback "Archived" section (cards whose original lane was deleted)
}

type BoardPageData struct {
	CurrentBoard       BoardData
	AllBoards          []BoardData   // active (non-archived, non-hidden) boards for the tab bar
	ArchivedViewBoards []BoardData   // archived view boards for the overflow dropdown
	HiddenBoards       []BoardData   // hidden boards for the overflow dropdown
	Lanes              []LaneWithCards
	Labels             []bankan.Label
	Token              string
	IsView             bool
	FilterLabel        string
	ShowArchived       bool // true when ?show_archived=true is active
	IsReadonly         bool // true when the current board is an archived view board
}

type CardDetailData struct {
	Card       *bankan.Card
	BoardID    string
	Labels     []bankan.Label
	Comments   []bankan.Comment
	Token      string
	IsView     bool
	IsReadonly bool // true when the board is an archived view board
}

// labelColor returns the hex color for a label ID from the labels slice.
func labelColor(labels []bankan.Label, id string) string {
	for _, l := range labels {
		if l.ID == id {
			return l.Color
		}
	}
	return "#666"
}

// labelName returns the name for a label ID.
func labelName(labels []bankan.Label, id string) string {
	for _, l := range labels {
		if l.ID == id {
			return l.Name
		}
	}
	return id
}

// sortedLabelIDs returns label IDs sorted by their resolved name (alphabetical).
func sortedLabelIDs(labelIDs []string, labels []bankan.Label) []string {
	sorted := make([]string, len(labelIDs))
	copy(sorted, labelIDs)
	sort.Slice(sorted, func(i, j int) bool {
		return labelName(labels, sorted[i]) < labelName(labels, sorted[j])
	})
	return sorted
}

// tintBg scales the board accent color so that its brightest channel equals
// targetMax, producing a very dark background that carries the board's hue.
// Returns the original hex unchanged if it cannot be parsed.
func tintBg(hex string, targetMax int) string {
	if len(hex) != 7 || hex[0] != '#' {
		return hex
	}
	var r, g, b int
	_, _ = fmt.Sscanf(hex[1:3], "%x", &r)
	_, _ = fmt.Sscanf(hex[3:5], "%x", &g)
	_, _ = fmt.Sscanf(hex[5:7], "%x", &b)
	max := r
	if g > max {
		max = g
	}
	if b > max {
		max = b
	}
	if max == 0 {
		// Pure black — distribute evenly so the background isn't pitch-black.
		v := targetMax / 3
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	}
	scale := float64(targetMax) / float64(max)
	r = int(float64(r) * scale)
	g = int(float64(g) * scale)
	b = int(float64(b) * scale)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// boardThemeStyle returns a full <style> block that overrides the CSS custom
// properties for a board-specific color theme. Returns "" if accentColor is empty.
func boardThemeStyle(accentColor string) string {
	if accentColor == "" {
		return ""
	}
	return fmt.Sprintf(
		"<style>:root{"+
			"--accent:%s;"+
			"--accent-hover:%s;"+
			"--bg-base:%s;"+
			"--bg-header:%s;"+
			"--bg-lane:%s;"+
			"--bg-card:%s;"+
			"--bg-card-hover:%s;"+
			"--bg-modal:%s;"+
			"--bg-input:%s;"+
			"--bg-btn:%s;"+
			"--bg-btn-hover:%s;"+
			"--border:%s;"+
			"--border-card:%s;"+
			"--border-focus:%s;"+
			"--border-card-left:%s;"+
			"--border-card-left-hover:%s"+
			"}</style>",
		accentColor,
		darkenHex(accentColor),
		tintBg(accentColor, 17),
		tintBg(accentColor, 22),
		tintBg(accentColor, 26),
		tintBg(accentColor, 32),
		tintBg(accentColor, 40),
		tintBg(accentColor, 31),
		tintBg(accentColor, 24),
		tintBg(accentColor, 44),
		tintBg(accentColor, 56),
		tintBg(accentColor, 46),
		tintBg(accentColor, 45),
		tintBg(accentColor, 74),
		tintBg(accentColor, 38),
		tintBg(accentColor, 55),
	)
}

// darkenHex returns a hex color darkened by ~15% (for hover states).
func darkenHex(hex string) string {
	if len(hex) != 7 || hex[0] != '#' {
		return hex
	}
	var r, g, b int
	_, _ = fmt.Sscanf(hex[1:3], "%x", &r)
	_, _ = fmt.Sscanf(hex[3:5], "%x", &g)
	_, _ = fmt.Sscanf(hex[5:7], "%x", &b)
	r = int(float64(r) * 0.85)
	g = int(float64(g) * 0.85)
	b = int(float64(b) * 0.85)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

const archivedLabelPrefix = "💼 "

func isArchivedLabel(l bankan.Label) bool {
	return strings.HasPrefix(l.Name, archivedLabelPrefix)
}

func hasActiveLabels(labels []bankan.Label) bool {
	for _, l := range labels {
		if !isArchivedLabel(l) {
			return true
		}
	}
	return false
}

func hasArchivedLabels(labels []bankan.Label) bool {
	for _, l := range labels {
		if isArchivedLabel(l) {
			return true
		}
	}
	return false
}

// cardBorderStyle returns the inline style for a card's left border color.
// If the card has a valid primary label, its color is used; otherwise returns "".
func cardBorderStyle(card *bankan.Card, labels []bankan.Label) string {
	if card.PrimaryLabel == "" {
		return ""
	}
	for _, l := range labels {
		if l.ID == card.PrimaryLabel {
			return "border-left-color:" + l.Color
		}
	}
	return ""
}

// cardLabelTitle returns a tooltip string for an assigned label chip.
// Returns "archived label" for archived labels, "" otherwise.
func cardLabelTitle(labels []bankan.Label, id string) string {
	for _, l := range labels {
		if l.ID == id && isArchivedLabel(l) {
			return "archived label"
		}
	}
	return ""
}

// primaryLabelTitle returns a tooltip for the primary label chip.
func primaryLabelTitle(labels []bankan.Label, id string) string {
	for _, l := range labels {
		if l.ID == id && isArchivedLabel(l) {
			return "Primary label · archived"
		}
	}
	return "Primary label"
}

// cardLabelStyle returns the inline CSS for an assigned label chip.
// Archived labels get opacity:0.4 to visually silence their color.
func cardLabelStyle(labels []bankan.Label, id string) string {
	color := labelColor(labels, id)
	style := "border-color:" + color + ";color:" + color + ";background:rgba(" + colorToRGB(color) + ",0.12)"
	for _, l := range labels {
		if l.ID == id && isArchivedLabel(l) {
			style += ";opacity:0.4"
			break
		}
	}
	return style
}

// colorToRGB converts #rrggbb to "r,g,b" for use in rgba().
func colorToRGB(hex string) string {
	if len(hex) != 7 || hex[0] != '#' {
		return "100,100,100"
	}
	r := hex[1:3]
	g := hex[3:5]
	b := hex[5:7]
	var ri, gi, bi int
	_, _ = fmt.Sscanf(r, "%x", &ri)
	_, _ = fmt.Sscanf(g, "%x", &gi)
	_, _ = fmt.Sscanf(b, "%x", &bi)
	return fmt.Sprintf("%d,%d,%d", ri, gi, bi)
}
