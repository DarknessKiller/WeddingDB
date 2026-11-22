package repository

import "errors"

var (
	// ErrInvalidCursor is returned when a pagination cursor cannot be decoded.
	ErrInvalidCursor = errors.New("invalid cursor")

	// ErrTokenClaimed reports that another caller rotated the token first.
	ErrTokenClaimed = errors.New("refresh token already rotated")

	// ErrLastAdmin is returned when a delete or demotion would leave no admin.
	ErrLastAdmin = errors.New("cannot remove the last admin")
)
