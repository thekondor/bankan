package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	bankan "github.com/thekondor/bankan"
	"github.com/thekondor/bankan/cmd/bankan/server"
	"github.com/thekondor/bankan/cmd/bankan/skill"
	"github.com/thekondor/bankan/internal/service"
)

// ─── board resolution ─────────────────────────────────────────────────────────

// resolveReg resolves a board (or view board) directory from boardFlag,
// creates a single-entry Registry, and returns the registry and board ID.
// When boardFlag is empty it walks up from the current working directory.
func resolveReg(boardFlag string) (*service.Registry, string, error) {
	dir, err := resolveDir(boardFlag)
	if err != nil {
		return nil, "", err
	}
	reg, id, err := service.NewSingleRegistry(dir)
	if err != nil {
		return nil, "", err
	}
	return reg, id, nil
}

// resolveDir finds the board/view-board directory from boardFlag or cwd walk-up.
func resolveDir(boardFlag string) (string, error) {
	if boardFlag != "" {
		abs, err := filepath.Abs(boardFlag)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// Try view board walk-up first.
	if bankan.IsViewBoard(cwd) {
		return cwd, nil
	}
	// Try board walk-up.
	b, err := bankan.FindBoard(cwd)
	if err == nil {
		return b.Dir, nil
	}
	// Fall back to view board walk-up.
	vb, vErr := bankan.FindViewBoard(cwd)
	if vErr == nil {
		return vb.Dir, nil
	}
	return "", err // return original board-not-found error
}

// gitUsername reads user.name from git config, falling back to $USER.
func gitUsername() string {
	out, err := exec.Command("git", "config", "user.name").Output()
	if err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			return name
		}
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "unknown"
}

// readTextInput resolves the text value for a flag that accepts "-" as a
// sentinel meaning "read from stdin until EOF".
func readTextInput(val string, r io.Reader) (string, error) {
	if val != "-" {
		return val, nil
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return string(data), nil
}

// resolveAbs converts dir to an absolute path.
func resolveAbs(dir string) (string, error) {
	if filepath.IsAbs(dir) {
		return dir, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if dir == "." {
		return cwd, nil
	}
	return filepath.Join(cwd, dir), nil
}

// ─── root ────────────────────────────────────────────────────────────────────

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "bankan",
		Short: "Local-first kanban board manager",
		Long: `bankan manages kanban boards stored as plain markdown files.

A board is a directory containing board.md plus lane directories and card files.
A view board is a label-filtered subset of a parent board, stored in view.md.
Boards can live inside any project directory and be tracked with git.`,
	}
	return root
}

// ─── board ───────────────────────────────────────────────────────────────────

func newBoardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "board",
		Short: "Board operations",
	}
	cmd.AddCommand(newBoardInitCmd(), newBoardShowCmd(), newBoardViewCmd(), newBoardReorderCmd(), newBoardHideCmd(), newBoardUnhideCmd())
	return cmd
}

func newBoardReorderCmd() *cobra.Command {
	var rootDir string
	cmd := &cobra.Command{
		Use:   "reorder <id1> <id2> ...",
		Short: "Set the display order of boards in the tab bar",
		Long: `Set the display order of all boards by listing their IDs in the desired order.
All registered board IDs must be listed exactly once.

The --root flag identifies the container directory (the same one passed to bankan serve).
If omitted, the current directory is used.

Example:
  bankan board reorder --root /my/boards feature-sprint main-board`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if rootDir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				rootDir = cwd
			}
			abs, err := filepath.Abs(rootDir)
			if err != nil {
				return err
			}
			reg, err := service.NewRegistry([]string{abs}, abs)
			if err != nil {
				return fmt.Errorf("load boards: %w", err)
			}
			if err := reg.ReorderBoards(args); err != nil {
				return err
			}
			fmt.Printf("Board order updated: %s\n", strings.Join(args, ", "))
			return nil
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Container directory holding the boards (default: current directory)")
	return cmd
}

func newBoardHideCmd() *cobra.Command {
	var rootDir string
	cmd := &cobra.Command{
		Use:   "hide <id>",
		Short: "Hide a board from the tab bar",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if rootDir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				rootDir = cwd
			}
			abs, err := filepath.Abs(rootDir)
			if err != nil {
				return err
			}
			reg, err := service.NewRegistry([]string{abs}, abs)
			if err != nil {
				return fmt.Errorf("load boards: %w", err)
			}
			if err := reg.HideBoard(args[0]); err != nil {
				return err
			}
			fmt.Printf("Board %q hidden from tab bar.\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Container directory holding the boards (default: current directory)")
	return cmd
}

func newBoardUnhideCmd() *cobra.Command {
	var rootDir string
	cmd := &cobra.Command{
		Use:   "unhide <id>",
		Short: "Restore a hidden board to the tab bar",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if rootDir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				rootDir = cwd
			}
			abs, err := filepath.Abs(rootDir)
			if err != nil {
				return err
			}
			reg, err := service.NewRegistry([]string{abs}, abs)
			if err != nil {
				return fmt.Errorf("load boards: %w", err)
			}
			if err := reg.ShowBoard(args[0]); err != nil {
				return err
			}
			fmt.Printf("Board %q restored to tab bar.\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Container directory holding the boards (default: current directory)")
	return cmd
}

func newBoardInitCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Initialise a new board in a directory (default: current directory)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			if name == "" {
				abs, err := resolveAbs(dir)
				if err != nil {
					return err
				}
				name = bankan.Deslugify(bankan.Slugify(filepath.Base(abs)))
			}
			b, err := bankan.InitBoard(dir, name)
			if err != nil {
				return err
			}
			fmt.Printf("Board %q initialised at %s\n", b.Name, b.Dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Board name (default: directory name)")
	return cmd
}

func newBoardShowCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show board metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if reg.IsViewBoard(id) {
				vb, parent, err := reg.GetViewBoard(id)
				if err != nil {
					return err
				}
				return printViewBoardInfo(vb, parent)
			}
			b, err := reg.GetBoard(id)
			if err != nil {
				return err
			}
			lanes, _ := reg.ListLanes(id)
			return printBoardInfo(b, lanes)
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board or view board directory")
	return cmd
}

func printBoardInfo(b *bankan.Board, lanes []bankan.Lane) error {
	fmt.Printf("Board:   %s\n", b.Name)
	fmt.Printf("Dir:     %s\n", b.Dir)
	fmt.Printf("Created: %s\n", b.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Lanes:   %d\n", len(lanes))
	fmt.Printf("Labels:  %d\n", len(b.Labels))
	if b.Body != "" {
		fmt.Printf("\n%s\n", strings.TrimSpace(b.Body))
	}
	return nil
}

func printViewBoardInfo(vb *bankan.ViewBoard, parent *bankan.Board) error {
	fmt.Printf("View board:   %s\n", vb.Name)
	fmt.Printf("Dir:          %s\n", vb.Dir)
	fmt.Printf("Parent:       %s (%s)\n", parent.Name, vb.Parent)
	fmt.Printf("Filter label: %s\n", vb.FilterLabel)
	fmt.Printf("Created:      %s\n", vb.CreatedAt.Format("2006-01-02 15:04:05"))
	if vb.ArchivedAt != nil {
		fmt.Printf("Archived:     %s\n", vb.ArchivedAt.Format("2006-01-02 15:04:05"))
	}
	if vb.Body != "" {
		fmt.Printf("\n%s\n", strings.TrimSpace(vb.Body))
	}
	return nil
}

// ─── board view ──────────────────────────────────────────────────────────────

func newBoardViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view",
		Short: "View board operations (label-filtered subset of a parent board)",
	}
	cmd.AddCommand(
		newBoardViewCreateCmd(),
		newBoardViewSyncCmd(),
		newBoardViewShowCmd(),
		newBoardViewArchiveCmd(),
	)
	return cmd
}

func newBoardViewCreateCmd() *cobra.Command {
	var (
		parentDir string
		labelID   string
		name      string
	)
	cmd := &cobra.Command{
		Use:   "create <dir>",
		Short: "Create a new view board filtered by a parent board label",
		Long: `Creates a view board at <dir> that shows only cards with --label from
the parent board at --parent. The view board clones the parent's lanes and
can be synced later with 'board view sync'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			viewDir := args[0]
			if name == "" {
				abs, err := resolveAbs(viewDir)
				if err != nil {
					return err
				}
				name = bankan.Deslugify(bankan.Slugify(filepath.Base(abs)))
			}
			vb, err := bankan.InitViewBoard(viewDir, name, parentDir, labelID)
			if err != nil {
				return err
			}
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return fmt.Errorf("load parent for initial sync: %w", err)
			}
			if err := bankan.SyncViewBoard(vb, parent); err != nil {
				return fmt.Errorf("initial sync: %w", err)
			}
			fmt.Printf("View board %q created at %s\n", vb.Name, vb.Dir)
			fmt.Printf("Filter label: %s  Parent: %s\n", vb.FilterLabel, vb.Parent)
			return nil
		},
	}
	cmd.Flags().StringVar(&parentDir, "parent", "", "Path to the parent board directory")
	cmd.Flags().StringVar(&labelID, "label", "", "Label ID on the parent board to filter by")
	cmd.Flags().StringVar(&name, "name", "", "View board name (default: directory name)")
	_ = cmd.MarkFlagRequired("parent")
	_ = cmd.MarkFlagRequired("label")
	return cmd
}

func newBoardViewSyncCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync view board with parent: add new stubs, remove orphaned stubs",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if !reg.IsViewBoard(id) {
				return fmt.Errorf("not a view board; use 'board view' commands for view boards")
			}
			vb, _, err := reg.GetViewBoard(id)
			if err != nil {
				return err
			}
			if err := reg.SyncViewBoard(id); err != nil {
				return err
			}
			fmt.Printf("View board %q synced with parent.\n", vb.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to view board directory")
	return cmd
}

func newBoardViewShowCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show view board metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if !reg.IsViewBoard(id) {
				return fmt.Errorf("not a view board; use 'board show' for regular boards")
			}
			vb, parent, err := reg.GetViewBoard(id)
			if err != nil {
				return err
			}
			return printViewBoardInfo(vb, parent)
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to view board directory")
	return cmd
}

func newBoardViewArchiveCmd() *cobra.Command {
	var (
		boardDir     string
		archiveLabel bool
	)
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Archive the view board (does not affect parent board cards)",
		Long: `Archives the view board by setting its archived_at timestamp.
The board becomes read-only and disappears from the active board list in the UI.

Use --archive-label to also prefix the filter label name with '💼 ',
marking it as belonging to an archived view board.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if !reg.IsViewBoard(id) {
				return fmt.Errorf("not a view board")
			}
			vb, _, err := reg.GetViewBoard(id)
			if err != nil {
				return err
			}
			if err := reg.ArchiveViewBoard(id, archiveLabel); err != nil {
				return err
			}
			msg := fmt.Sprintf("View board %q archived.", vb.Name)
			if archiveLabel {
				msg += " Filter label prefixed with 💼."
			}
			fmt.Println(msg)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to view board directory")
	cmd.Flags().BoolVar(&archiveLabel, "archive-label", false, "Also prefix the filter label name with '💼 ' on the parent board")
	return cmd
}

// ─── lane ────────────────────────────────────────────────────────────────────

func newLaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lane",
		Short: "Lane operations",
	}
	cmd.AddCommand(newLaneAddCmd(), newLaneListCmd(), newLaneRenameCmd(), newLaneRemoveCmd())
	return cmd
}

func newLaneAddCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new lane",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			l, err := reg.AddLane(id, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Lane %q added (order %d)\n", l.Name, l.Order)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	return cmd
}

func newLaneListCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all lanes",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			lanes, err := reg.ListLanes(id)
			if err != nil {
				return err
			}
			if len(lanes) == 0 {
				fmt.Println("No lanes.")
				return nil
			}
			for _, l := range lanes {
				cards, _ := reg.ListCards(id, l.Name)
				fmt.Printf("  %02d  %-24s  %d card(s)\n", l.Order, l.Name, len(cards))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	return cmd
}

func newLaneRenameCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "rename <old-name> <new-name>",
		Short: "Rename a lane",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if err := reg.RenameLane(id, args[0], args[1]); err != nil {
				return err
			}
			suffix := ""
			if reg.IsViewBoard(id) {
				suffix = " in view board"
			}
			fmt.Printf("Lane %q renamed to %q%s\n", args[0], args[1], suffix)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	return cmd
}

func newLaneRemoveCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an empty lane",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if err := reg.RemoveLane(id, args[0]); err != nil {
				return err
			}
			suffix := ""
			if reg.IsViewBoard(id) {
				suffix = " from view board"
			}
			fmt.Printf("Lane %q removed%s\n", args[0], suffix)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	return cmd
}

// ─── card ────────────────────────────────────────────────────────────────────

func newCardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "card",
		Short: "Card operations",
	}
	cmd.AddCommand(
		newCardAddCmd(),
		newCardListCmd(),
		newCardShowCmd(),
		newCardEditCmd(),
		newCardMoveCmd(),
		newCardReorderCmd(),
		newCardArchiveCmd(),
		newCardRestoreCmd(),
		newCardDeleteCmd(),
		newCardDuplicateCmd(),
		newCardSearchCmd(),
	)
	return cmd
}

func newCardAddCmd() *cobra.Command {
	var (
		boardDir string
		laneName string
		title    string
		body     string
		labels   []string
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new card to a lane",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			body, err = readTextInput(body, cmd.InOrStdin())
			if err != nil {
				return err
			}
			c, err := reg.AddCard(id, laneName, title, body, labels)
			if err != nil {
				return err
			}
			suffix := ""
			if reg.IsViewBoard(id) {
				suffix = " (and parent board)"
			}
			fmt.Printf("Card %s created: %q in lane %q%s\n", c.ID, c.Title, laneName, suffix)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	cmd.Flags().StringVar(&laneName, "lane", "", "Target lane name")
	cmd.Flags().StringVar(&title, "title", "", "Card title")
	cmd.Flags().StringVar(&body, "body", "", "Card body (markdown); use - to read from stdin")
	cmd.Flags().StringArrayVar(&labels, "label", nil, "Label ID to attach (repeatable)")
	_ = cmd.MarkFlagRequired("lane")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func newCardListCmd() *cobra.Command {
	var (
		boardDir string
		laneName string
		labelID  string
		archived bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cards",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}

			if archived {
				if reg.IsViewBoard(id) {
					return fmt.Errorf("--archived is not applicable for view boards")
				}
				cards, err := reg.ListArchivedCards(id)
				if err != nil {
					return err
				}
				printCardList("[archive]", cards, labelID)
				return nil
			}

			lanes, err := reg.ListLanes(id)
			if err != nil {
				return err
			}
			for _, lane := range lanes {
				if laneName != "" && !strings.EqualFold(lane.Name, laneName) {
					continue
				}
				cards, err := reg.ListCards(id, lane.Name)
				if err != nil {
					return err
				}
				header := lane.Name
				if reg.IsViewBoard(id) {
					header += " [view]"
				}
				printCardList(header, cards, labelID)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	cmd.Flags().StringVar(&laneName, "lane", "", "Filter by lane name")
	cmd.Flags().StringVar(&labelID, "label", "", "Filter by label ID")
	cmd.Flags().BoolVar(&archived, "archived", false, "Show archived cards (not applicable for view boards)")
	return cmd
}

func printCardList(header string, cards []*bankan.Card, filterLabel string) {
	shown := 0
	for _, c := range cards {
		if filterLabel != "" && !containsString(c.Labels, filterLabel) {
			continue
		}
		if shown == 0 {
			fmt.Printf("\n[%s]\n", header)
		}
		labels := ""
		if len(c.Labels) > 0 {
			labels = "  labels:" + strings.Join(c.Labels, ",")
		}
		fmt.Printf("  %s  %s%s\n", c.ID, c.Title, labels)
		shown++
	}
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func newCardShowCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show card details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			c, err := reg.GetCard(id, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("ID:         %s\n", c.ID)
			fmt.Printf("Title:      %s\n", c.Title)
			fmt.Printf("Lane:       %s\n", c.Lane)
			fmt.Printf("Created:    %s\n", c.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("Updated:    %s\n", c.UpdatedAt.Format("2006-01-02 15:04:05"))
			if c.MovedAt != nil {
				fmt.Printf("Moved:      %s (from %q)\n", c.MovedAt.Format("2006-01-02 15:04:05"), c.MovedFrom)
			}
			if c.ArchivedAt != nil {
				fmt.Printf("Archived:   %s (from %q)\n", c.ArchivedAt.Format("2006-01-02 15:04:05"), c.ArchivedFrom)
			}
			if len(c.Labels) > 0 {
				fmt.Printf("Labels:     %s\n", strings.Join(c.Labels, ", "))
			}
			if strings.TrimSpace(c.Body) != "" {
				fmt.Printf("\n--- body ---\n%s\n", strings.TrimSpace(c.Body))
			}
			comments, _ := reg.ListComments(id, c.ID)
			if len(comments) > 0 {
				fmt.Printf("\n--- comments (%d) ---\n", len(comments))
				for _, cm := range comments {
					fmt.Printf("  [%s] %s · %s\n  %s\n\n",
						cm.ID,
						cm.CreatedAt.Format("2006-01-02 15:04"),
						cm.Author,
						cm.Body,
					)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	return cmd
}

func newCardEditCmd() *cobra.Command {
	var (
		boardDir     string
		title        string
		body         string
		addLabels    []string
		removeLabels []string
		primaryLabel string
	)
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit card title, body, or labels",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			update := service.CardUpdate{
				AddLabels:    addLabels,
				RemoveLabels: removeLabels,
			}
			if cmd.Flags().Changed("title") {
				update.Title = &title
			}
			if cmd.Flags().Changed("body") {
				body, err = readTextInput(body, cmd.InOrStdin())
				if err != nil {
					return err
				}
				update.Body = &body
			}
			if cmd.Flags().Changed("primary-label") {
				update.PrimaryLabel = &primaryLabel
			}
			c, err := reg.UpdateCard(id, args[0], update)
			if err != nil {
				return err
			}
			fmt.Printf("Card %s updated.\n", c.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	cmd.Flags().StringVar(&title, "title", "", "New title")
	cmd.Flags().StringVar(&body, "body", "", "New body (markdown); use - to read from stdin")
	cmd.Flags().StringArrayVar(&addLabels, "add-label", nil, "Label ID to add (repeatable)")
	cmd.Flags().StringArrayVar(&removeLabels, "remove-label", nil, "Label ID to remove (repeatable)")
	cmd.Flags().StringVar(&primaryLabel, "primary-label", "", "Primary label ID (empty string clears it)")
	return cmd
}

func newCardMoveCmd() *cobra.Command {
	var (
		boardDir string
		laneName string
	)
	cmd := &cobra.Command{
		Use:   "move <id>",
		Short: "Move a card to a different lane",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if err := reg.MoveCard(id, args[0], laneName); err != nil {
				return err
			}
			suffix := ""
			if reg.IsViewBoard(id) {
				suffix = " (view board)"
			}
			fmt.Printf("Card %s moved to %q%s\n", args[0], laneName, suffix)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	cmd.Flags().StringVar(&laneName, "lane", "", "Destination lane name")
	_ = cmd.MarkFlagRequired("lane")
	return cmd
}

func newCardArchiveCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "archive <id>",
		Short: "Archive a card (in view boards: removes filter label from parent card)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if err := reg.ArchiveCard(id, args[0]); err != nil {
				return err
			}
			if reg.IsViewBoard(id) {
				fmt.Printf("Card %s removed from view (filter label removed from parent card).\n", args[0])
			} else {
				fmt.Printf("Card %s archived.\n", args[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	return cmd
}

func newCardRestoreCmd() *cobra.Command {
	var (
		boardDir string
		laneName string
	)
	cmd := &cobra.Command{
		Use:   "restore <id>",
		Short: "Restore an archived card to a lane (not available in view boards)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if err := reg.RestoreCard(id, args[0], laneName); err != nil {
				return err
			}
			fmt.Printf("Card %s restored to %q\n", args[0], laneName)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	cmd.Flags().StringVar(&laneName, "lane", "", "Destination lane name")
	_ = cmd.MarkFlagRequired("lane")
	return cmd
}

func newCardDeleteCmd() *cobra.Command {
	var (
		boardDir string
		force    bool
	)
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a card (in view boards: removes filter label from parent card)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if !force {
				if reg.IsViewBoard(id) {
					c, err := reg.GetCard(id, args[0])
					if err != nil {
						return err
					}
					vb, _, err := reg.GetViewBoard(id)
					if err != nil {
						return err
					}
					fmt.Printf("Remove card %s (%q) from view? The parent card will lose label %q.\nPass --force to confirm.\n",
						c.ID, c.Title, vb.FilterLabel)
				} else {
					c, err := reg.GetCard(id, args[0])
					if err != nil {
						return err
					}
					fmt.Printf("Delete card %s (%q)? This cannot be undone. Pass --force to confirm.\n", c.ID, c.Title)
				}
				return nil
			}
			if err := reg.DeleteCard(id, args[0]); err != nil {
				return err
			}
			if reg.IsViewBoard(id) {
				fmt.Printf("Card %s removed from view (filter label removed from parent card).\n", args[0])
			} else {
				fmt.Printf("Card %s deleted.\n", args[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	cmd.Flags().BoolVar(&force, "force", false, "Confirm the operation")
	return cmd
}

func newCardReorderCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "reorder <card-id> <new-index>",
		Short: "Change the position of a card within its current lane (0-based index)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			newIndex, err := strconv.Atoi(args[1])
			if err != nil || newIndex < 0 {
				return fmt.Errorf("new-index must be a non-negative integer")
			}
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if err := reg.ReorderCard(id, args[0], newIndex); err != nil {
				return err
			}
			fmt.Printf("Card %s reordered to position %d\n", args[0], newIndex)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	return cmd
}

func newCardDuplicateCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "duplicate <card-id>",
		Short: "Duplicate a card in the same lane",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			c, err := reg.DuplicateCard(id, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Card %s (%q) duplicated from %s\n", c.ID, c.Title, args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	return cmd
}

func newCardSearchCmd() *cobra.Command {
	var (
		rootDir         string
		includeArchived bool
	)
	cmd := &cobra.Command{
		Use:   "search <id>",
		Short: "Find which board(s) contain a card by ID (active boards only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cardID := args[0]
			if rootDir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				rootDir = cwd
			}
			abs, err := filepath.Abs(rootDir)
			if err != nil {
				return err
			}
			reg, err := service.NewRegistry([]string{abs}, abs)
			if err != nil {
				return fmt.Errorf("load boards: %w", err)
			}
			results, err := reg.SearchCard(cardID, includeArchived)
			if err != nil {
				return err
			}
			if len(results) == 0 {
				return fmt.Errorf("card %q not found in any active board", cardID)
			}
			if len(results) > 1 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: card %q found in %d boards\n", cardID, len(results))
			}
			for _, r := range results {
				fmt.Printf("Board: %s  Lane: %s  Title: %s\n", r.BoardName, r.LaneName, r.CardTitle)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Container directory holding all boards (default: cwd)")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Also search archived view boards")
	return cmd
}

// ─── comment ─────────────────────────────────────────────────────────────────

func newCommentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Comment operations",
	}
	cmd.AddCommand(newCommentAddCmd(), newCommentEditCmd(), newCommentListCmd())
	return cmd
}

func newCommentAddCmd() *cobra.Command {
	var (
		boardDir string
		text     string
		author   string
	)
	cmd := &cobra.Command{
		Use:   "add <card-id>",
		Short: "Add a comment to a card",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if author == "" {
				author = gitUsername()
			}
			text, err = readTextInput(text, cmd.InOrStdin())
			if err != nil {
				return err
			}
			cm, err := reg.AddComment(id, args[0], author, text)
			if err != nil {
				return err
			}
			fmt.Printf("Comment %s added by %s\n", cm.ID, cm.Author)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	cmd.Flags().StringVar(&text, "text", "", "Comment body (markdown); use - to read from stdin")
	cmd.Flags().StringVar(&author, "author", "", "Author name (default: git config user.name)")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

func newCommentEditCmd() *cobra.Command {
	var (
		boardDir  string
		text      string
	)
	cmd := &cobra.Command{
		Use:   "edit <comment-id> --card <card-id>",
		Short: "Edit a comment body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cardID, _ := cmd.Flags().GetString("card")
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			text, err = readTextInput(text, cmd.InOrStdin())
			if err != nil {
				return err
			}
			cm, err := reg.UpdateComment(id, cardID, args[0], text)
			if err != nil {
				return err
			}
			fmt.Printf("Comment %s updated\n", cm.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	cmd.Flags().String("card", "", "Card ID that owns the comment")
	cmd.Flags().StringVar(&text, "text", "", "New comment body (markdown); use - to read from stdin")
	_ = cmd.MarkFlagRequired("card")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

func newCommentListCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "list <card-id>",
		Short: "List comments on a card",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			comments, err := reg.ListComments(id, args[0])
			if err != nil {
				return err
			}
			if len(comments) == 0 {
				fmt.Println("No comments.")
				return nil
			}
			for _, cm := range comments {
				fmt.Printf("[%s] %s · %s\n%s\n\n",
					cm.ID,
					cm.CreatedAt.Format("2006-01-02 15:04"),
					cm.Author,
					cm.Body,
				)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	return cmd
}

// ─── label ───────────────────────────────────────────────────────────────────

func newLabelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Label operations",
	}
	cmd.AddCommand(newLabelAddCmd(), newLabelListCmd(), newLabelEditCmd(), newLabelRemoveCmd())
	return cmd
}

func newLabelAddCmd() *cobra.Command {
	var (
		boardDir string
		name     string
		color    string
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a label to the board",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			l, err := reg.AddLabel(id, name, color)
			if err != nil {
				return err
			}
			fmt.Printf("Label %s (%q, %s) added\n", l.ID, l.Name, l.Color)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	cmd.Flags().StringVar(&name, "name", "", "Label name")
	cmd.Flags().StringVar(&color, "color", "", "Hex color, e.g. #ef4444")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("color")
	return cmd
}

func newLabelListCmd() *cobra.Command {
	var boardDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all labels",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			isView := reg.IsViewBoard(id)
			if isView {
				vb, _, err := reg.GetViewBoard(id)
				if err != nil {
					return err
				}
				fmt.Printf("(parent board labels — filter label: %s)\n", vb.FilterLabel)
			}
			labels, err := reg.ListLabels(id)
			if err != nil {
				return err
			}
			if len(labels) == 0 {
				fmt.Println("No labels.")
				return nil
			}
			var filterLabelID string
			if isView {
				vb, _, _ := reg.GetViewBoard(id)
				filterLabelID = vb.FilterLabel
			}
			for _, l := range labels {
				marker := ""
				if isView && l.ID == filterLabelID {
					marker = "  [filter]"
				}
				fmt.Printf("  %s  %-20s  %s%s\n", l.ID, l.Name, l.Color, marker)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	return cmd
}

func newLabelEditCmd() *cobra.Command {
	var (
		boardDir string
		name     string
		color    string
	)
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit a label's name or color",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			update := service.LabelUpdate{}
			if cmd.Flags().Changed("name") {
				update.Name = &name
			}
			if cmd.Flags().Changed("color") {
				update.Color = &color
			}
			l, err := reg.UpdateLabel(id, args[0], update)
			if err != nil {
				return err
			}
			fmt.Printf("Label %s updated.\n", l.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	cmd.Flags().StringVar(&name, "name", "", "New label name")
	cmd.Flags().StringVar(&color, "color", "", "New hex color")
	return cmd
}

func newLabelRemoveCmd() *cobra.Command {
	var boardDir string
	var force bool
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Archive a label (default) or permanently delete it (--force)",
		Long: `By default the label is archived: it is prefixed with "` + "💼" + `" so it
remains visible on existing cards but is excluded from pickers.
Use --force to permanently delete the label instead.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, id, err := resolveReg(boardDir)
			if err != nil {
				return err
			}
			if err := reg.RemoveLabel(id, args[0], force); err != nil {
				return err
			}
			suffix := ""
			if reg.IsViewBoard(id) {
				suffix = " from parent board"
			}
			if force {
				fmt.Printf("Label %s removed%s.\n", args[0], suffix)
			} else {
				fmt.Printf("Label %s archived%s.\n", args[0], suffix)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")
	cmd.Flags().BoolVar(&force, "force", false, "Permanently delete instead of archiving")
	return cmd
}

// ─── ai-skill ────────────────────────────────────────────────────────────────

type skillType string

const (
	skillTypeClaudeCode skillType = "claude-code"
	skillTypeOpenCode   skillType = "opencode"
	skillTypeCodex      skillType = "codex"
)

var validSkillTypes = []skillType{skillTypeClaudeCode, skillTypeOpenCode, skillTypeCodex}

// skillInstallHint returns the recommended target path for each skill type.
func skillInstallHint(st skillType) string {
	switch st {
	case skillTypeClaudeCode:
		return ".claude/skills/bankan/"
	case skillTypeOpenCode:
		return ".opencode/skills/bankan/  (or .claude/skills/bankan/ or .agents/skills/bankan/)"
	case skillTypeCodex:
		return ".agents/skills/bankan/"
	default:
		return ""
	}
}

func newAISkillCmd() *cobra.Command {
	var withBinPath bool
	var skillTypeFlag string

	cmd := &cobra.Command{
		Use:   "ai-skill --type <type> <output-dir>",
		Short: "Generate an AI agent skill file for bankan",
		Long: `Writes a concise bankan CLI reference as a skill file into <output-dir>/bankan/SKILL.md.

Supported skill types:
  claude-code   Claude Code compatible skill (SKILL.md with description frontmatter)
  opencode      OpenCode compatible skill (SKILL.md with name+description frontmatter)
  codex         Codex compatible skill (SKILL.md with name+description frontmatter)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if skillTypeFlag == "" {
				return fmt.Errorf("ai-skill: --type is required (claude-code, opencode, codex)")
			}
			st := skillType(skillTypeFlag)
			validType := false
			for _, v := range validSkillTypes {
				if st == v {
					validType = true
					break
				}
			}
			if !validType {
				return fmt.Errorf("ai-skill: unknown --type %q; must be one of: claude-code, opencode, codex", skillTypeFlag)
			}

			cmdName := "bankan"
			if withBinPath {
				exe, err := os.Executable()
				if err != nil {
					return fmt.Errorf("ai-skill: resolve binary path: %w", err)
				}
				cmdName, err = filepath.Abs(exe)
				if err != nil {
					return fmt.Errorf("ai-skill: absolute binary path: %w", err)
				}
			}

			tmplFile := "templates/bankan.md.tmpl"
			if st == skillTypeOpenCode || st == skillTypeCodex {
				tmplFile = "templates/bankan-agent-skill.md.tmpl"
			}

			tmplBytes, err := skill.TemplateFS.ReadFile(tmplFile)
			if err != nil {
				return fmt.Errorf("ai-skill: read template: %w", err)
			}

			tmpl, err := template.New("bankan").Parse(string(tmplBytes))
			if err != nil {
				return fmt.Errorf("ai-skill: parse template: %w", err)
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, struct{ Cmd string }{Cmd: cmdName}); err != nil {
				return fmt.Errorf("ai-skill: render template: %w", err)
			}

			skillDir := filepath.Join(args[0], "bankan")
			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				return fmt.Errorf("ai-skill: create skill directory: %w", err)
			}

			outPath := filepath.Join(skillDir, "SKILL.md")
			if err := os.WriteFile(outPath, buf.Bytes(), 0o644); err != nil {
				return fmt.Errorf("ai-skill: write skill file: %w", err)
			}

			abs, err := filepath.Abs(skillDir)
			if err != nil {
				abs = skillDir
			}
			fmt.Printf("Skill written to %s\n", abs)
			fmt.Printf("Install: copy the bankan/ directory to %s\n", skillInstallHint(st))
			return nil
		},
	}
	cmd.Flags().BoolVar(&withBinPath, "with-bin-path", false, "Hardcode absolute path to bankan binary in the skill")
	cmd.Flags().StringVar(&skillTypeFlag, "type", "", "Skill type to generate: claude-code, opencode, codex (required)")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

// ─── main ────────────────────────────────────────────────────────────────────

func main() {
	fmt.Fprintln(os.Stderr, "*** bankan (https://github.com/thekondor/bankan)")
	root := newRootCmd()
	root.AddCommand(
		newBoardCmd(),
		newLaneCmd(),
		newCardCmd(),
		newCommentCmd(),
		newLabelCmd(),
		newServeCmd(),
		newAISkillCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// ─── serve ────────────────────────────────────────────────────────────────────

// parseWorkspaceArg parses a single positional "serve" argument.
// Format: "name:path" or bare "path".
func parseWorkspaceArg(arg string) service.WorkspaceArg {
	if idx := strings.Index(arg, ":"); idx > 0 {
		name := arg[:idx]
		path := arg[idx+1:]
		if path != "" {
			return service.WorkspaceArg{Name: name, Dir: path}
		}
	}
	return service.WorkspaceArg{Dir: arg}
}

func newServeCmd() *cobra.Command {
	var (
		port    int
		bind    string
		token   string
		noToken bool
	)
	cmd := &cobra.Command{
		Use:   "serve [name:dir...]",
		Short: "Start the REST API and HTMX UI server",
		Long: `Start bankan as an HTTP server exposing a REST API and a web UI.

Each positional argument specifies a workspace root directory.
Format: "name:path" to set an explicit workspace name, or bare "path" to
derive the name from the last two path components.

Each root directory is scanned one level deep for boards and used as the
root for new board creation. If it contains board.md or view.md directly,
it is registered as a single-board workspace.

Examples:
  bankan serve /my/project
  bankan serve my-team:/path/to/boards ops:/other/boards
  bankan serve --port 9090 --no-token /my/boards`,
		RunE: func(cmd *cobra.Command, args []string) error {
			wsArgs := make([]service.WorkspaceArg, 0, len(args))
			for _, a := range args {
				wsArgs = append(wsArgs, parseWorkspaceArg(a))
			}

			workspaces, err := service.NewWorkspaces(wsArgs)
			if err != nil {
				return fmt.Errorf("load workspaces: %w", err)
			}

			for _, ws := range workspaces {
				ids := ws.Reg.BoardIDs()
				if len(ids) == 0 {
					fmt.Fprintf(os.Stderr, "Warning: workspace %q has no boards.\n", ws.Name)
				} else {
					fmt.Printf("Workspace %q: %d board(s): %s\n", ws.Name, len(ids), strings.Join(ids, ", "))
				}
			}

			logger := log.New(os.Stdout, "[bankan] ", log.LstdFlags)
			srv, err := server.New(workspaces, server.Config{
				Bind:    bind,
				Port:    port,
				Token:   token,
				NoToken: noToken,
			}, logger)
			if err != nil {
				return err
			}

			if !noToken {
				fmt.Printf("Bankan token: %s\n", srv.Token())
				fmt.Println("Include header 'X-Bankan-Token: <token>' on all mutating requests.")
			} else {
				fmt.Println("Token protection disabled (--no-token).")
			}

			addr := srv.Addr()
			fmt.Printf("Listening on http://%s\n", addr)
			return http.ListenAndServe(addr, srv)
		},
	}
	cmd.Flags().IntVar(&port, "port", 8080, "HTTP port to listen on")
	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "Address to bind (use 0.0.0.0 for all interfaces)")
	cmd.Flags().StringVar(&token, "token", "", "Pre-set token (default: random 32-byte hex token)")
	cmd.Flags().BoolVar(&noToken, "no-token", false, "Disable token protection (anyone can write)")
	return cmd
}
