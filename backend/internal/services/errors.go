package services

import "errors"

var (
	// ErrAlreadyCheckedIn is returned when a guest is already checked in (FIFO conflict).
	ErrAlreadyCheckedIn = errors.New("guest already checked in")

	// ErrSeatOverlap is returned when a seat assignment overlaps with another guest.
	ErrSeatOverlap = errors.New("seat overlaps with another guest")

	// ErrGuestNotFound is returned when the guest does not exist in the wedding.
	ErrGuestNotFound = errors.New("guest not found")

	// ErrGuestPatchInvalid is returned when a sparse guest write carries a
	// value the API refuses.
	ErrGuestPatchInvalid = errors.New("invalid guest patch")

	ErrTableNotFound   = errors.New("table not found")
	ErrInvalidGuestPax = errors.New("guest pax must be at least 1")
	ErrSeatRange       = errors.New("seat range exceeds table capacity")
	ErrSSECapacity     = errors.New("too many streams for wedding")
	ErrSSEUserCapacity = errors.New("too many streams for user")
)

// ErrTokenRejected wraps every reason a presented access or refresh token is
// refused (bad, expired, revoked, blacklisted, stale version, replayed).
// Handlers map it to 401; any other auth error is an outage and maps to 503.
var ErrTokenRejected = errors.New("invalid or expired token")

// SelectWedding sentinels; handlers map them to 401/403.
var (
	ErrAdminNotFound   = errors.New("admin not found")
	ErrNoWeddingAccess = errors.New("no access to this wedding")
)
