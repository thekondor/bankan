package service

import "fmt"

// ErrNotFound is returned when a resource cannot be found.
type ErrNotFound struct {
	Resource string
	ID       string
}

func (e *ErrNotFound) Error() string {
	if e.ID != "" {
		return fmt.Sprintf("%s %q not found", e.Resource, e.ID)
	}
	return fmt.Sprintf("%s not found", e.Resource)
}

// ErrForbidden is returned when an operation is not permitted.
type ErrForbidden struct {
	Reason string
}

func (e *ErrForbidden) Error() string { return e.Reason }

// ErrConflict is returned when there is a naming or ID collision.
type ErrConflict struct {
	Reason string
}

func (e *ErrConflict) Error() string { return e.Reason }

// ErrBadRequest is returned when inputs fail validation.
type ErrBadRequest struct {
	Reason string
}

func (e *ErrBadRequest) Error() string { return e.Reason }
