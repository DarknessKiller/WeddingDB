package services

import "errors"

var (
	// ErrAlreadyCheckedIn is returned when a guest is already checked in (FIFO conflict).
	ErrAlreadyCheckedIn = errors.New("guest already checked in")

	// ErrSeatOverlap is returned when a seat assignment overlaps with another guest.
	ErrSeatOverlap = errors.New("seat overlaps with another guest")
)

// SelectWedding sentinels; handlers map them to 401/403.
var (
	ErrAdminNotFound   = errors.New("admin not found")
	ErrNoWeddingAccess = errors.New("no access to this wedding")
)
