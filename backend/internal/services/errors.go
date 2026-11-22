package services

import "errors"

var (
	// ErrAlreadyCheckedIn is returned when a guest is already checked in (FIFO conflict).
	ErrAlreadyCheckedIn = errors.New("guest already checked in")

	// ErrCheckInChanged is returned when a check-out targets a check-in that
	// was replaced after the caller loaded its list.
	ErrCheckInChanged = errors.New("guest was checked in again since it was loaded")

	// ErrSeatOverlap is returned when a seat assignment overlaps with another guest.
	ErrSeatOverlap = errors.New("seat overlaps with another guest")

	// ErrGuestNotFound is returned when the guest does not exist in the wedding.
	ErrGuestNotFound = errors.New("guest not found")

	// ErrWeddingNotFound is returned when a report names an unknown wedding.
	ErrWeddingNotFound = errors.New("wedding not found")

	// ErrInvalidToken is returned when a refresh token is unknown, expired, or
	// replayed. Every other Refresh error is an infrastructure fault, so the
	// handler answers 503 and the client keeps its session and offline queue.
	ErrInvalidToken = errors.New("invalid refresh token")
)

// SelectWedding sentinels; handlers map them to 401/403.
var (
	ErrAdminNotFound   = errors.New("admin not found")
	ErrNoWeddingAccess = errors.New("no access to this wedding")
)
