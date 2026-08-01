package services

import "errors"

var (
	// ErrAlreadyCheckedIn is returned when a guest is already checked in (FIFO conflict).
	ErrAlreadyCheckedIn = errors.New("guest already checked in")

	// ErrSeatOverlap is returned when a seat assignment overlaps with another guest.
	ErrSeatOverlap = errors.New("seat overlaps with another guest")
)
