package service

import (
	"fmt"

	bankan "github.com/thekondor/bankan"
)

// ListComments returns all comments on card cardID in board id.
// For view boards the card is resolved from the parent.
func (r *Registry) ListComments(id, cardID string) ([]bankan.Comment, error) {
	var comments []bankan.Comment
	err := r.withReadLock(id, func(dir string, isView bool) error {
		filePath, err := resolveCardFilePath(dir, isView, cardID)
		if err != nil {
			return err
		}
		cs, err := bankan.ReadComments(filePath)
		if err != nil {
			return fmt.Errorf("read comments for card %q: %w", cardID, err)
		}
		comments = cs
		return nil
	})
	return comments, err
}

// AddComment appends a comment to card cardID in board id.
func (r *Registry) AddComment(id, cardID, author, body string) (*bankan.Comment, error) {
	var comment *bankan.Comment
	err := r.withWriteLock(id, func(dir string, isView bool) error {
		filePath, err := resolveCardFilePath(dir, isView, cardID)
		if err != nil {
			return err
		}
		cm, err := bankan.AddComment(filePath, author, body)
		if err != nil {
			return fmt.Errorf("add comment to card %q: %w", cardID, err)
		}
		comment = cm
		return nil
	})
	return comment, err
}

// UpdateComment replaces the body of commentID on card cardID in board id.
func (r *Registry) UpdateComment(id, cardID, commentID, body string) (*bankan.Comment, error) {
	var comment *bankan.Comment
	err := r.withWriteLock(id, func(dir string, isView bool) error {
		filePath, err := resolveCardFilePath(dir, isView, cardID)
		if err != nil {
			return err
		}
		cm, err := bankan.UpdateComment(filePath, commentID, body)
		if err != nil {
			return fmt.Errorf("update comment %q on card %q: %w", commentID, cardID, err)
		}
		comment = cm
		return nil
	})
	return comment, err
}

// resolveCardFilePath finds the file path for card cardID, accounting for view boards.
func resolveCardFilePath(dir string, isView bool, cardID string) (string, error) {
	if isView {
		vb, err := bankan.ReadViewBoard(dir)
		if err != nil {
			return "", err
		}
		parent, err := bankan.ParentBoard(vb)
		if err != nil {
			return "", err
		}
		stub, _, err := bankan.FindViewCardStub(vb, cardID)
		if err != nil {
			return "", &ErrNotFound{Resource: "card", ID: cardID}
		}
		c, err := bankan.ResolveViewCard(stub, parent)
		if err != nil {
			return "", err
		}
		return c.FilePath, nil
	}
	b, err := bankan.ReadBoard(dir)
	if err != nil {
		return "", err
	}
	c, err := bankan.FindCard(b, cardID, true)
	if err != nil {
		return "", &ErrNotFound{Resource: "card", ID: cardID}
	}
	return c.FilePath, nil
}
