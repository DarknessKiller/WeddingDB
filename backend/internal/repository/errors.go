package repository

import "errors"

var (
	// ErrInvalidCursor is returned when a pagination cursor cannot be decoded.
	ErrInvalidCursor = errors.New("invalid cursor")

	// ErrTokenClaimed reports that another caller rotated the token first.
	ErrTokenClaimed = errors.New("refresh token already rotated")

	// ErrLastAdmin is returned when a delete or demotion would leave no admin.
	ErrLastAdmin = errors.New("cannot remove the last admin")

	// ErrCapacityBelowSeated refuses a table shrink that would strand a guest
	// already seated past the new capacity.
	ErrCapacityBelowSeated = errors.New("capacity is below an already seated guest")
	// Layout save failures keep their entity so the handler can name it.
	ErrLayoutTableNotFound   = errors.New("layout table not found")
	ErrLayoutElementNotFound = errors.New("layout element not found")
)
