package services

import "errors"

var (
	// ErrAlreadyCheckedIn is returned when a guest is already checked in (FIFO conflict).
	ErrAlreadyCheckedIn = errors.New("guest already checked in")

	// ErrSeatOverlap is returned when a seat assignment overlaps with another guest.
	ErrSeatOverlap = errors.New("seat overlaps with another guest")

	// ErrGuestNotFound is returned when the guest does not exist in the wedding.
	ErrGuestNotFound = errors.New("guest not found")

	// ErrStaleGuest is returned when an edit was based on a guest row that
	// someone else changed since the editor loaded it.
	ErrStaleGuest = errors.New("guest was changed by someone else")

	// Seat and table rule violations. The messages are shown to staff as is.
	ErrSeatRange     = errors.New("seat range exceeds table capacity")
	ErrTableFull     = errors.New("table capacity exceeded")
	ErrTableNotFound = errors.New("table not found")

	errInvalidTableID = errors.New("invalid tableId")
)

// SelectWedding sentinels; handlers map them to 401/403.
var (
	ErrAdminNotFound   = errors.New("admin not found")
	ErrNoWeddingAccess = errors.New("no access to this wedding")
	ErrSessionNotFound = errors.New("refresh token not found or expired")
)

// Login/Refresh sentinels; handlers map them to 401 and anything else to 500.
var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
)
