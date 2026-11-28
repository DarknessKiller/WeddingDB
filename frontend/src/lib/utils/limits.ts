// Input length caps mirrored from backend/internal/services/limits.go.
// Every form field that feeds an API-validated value uses these numbers, and a
// backend cap change belongs in both files (CODING_STANDARDS.md, "Frontend
// input limits").
export const LIMITS = {
	name: 255,
	email: 254,
	password: 72,
	guestPax: 50,
	guestNotes: 2000,
	checkInNote: 500,
	dietaryItem: 100,
	dietaryItems: 20,
	giftItem: 255,
	tableName: 100,
	// Mirrors services.MaxTableCapacity.
	tableCapacity: 100,
	// Mirrors services.MaxAngbaoAmt (not a text length: the largest amount).
	angbaoAmt: 10_000_000,
	venueName: 255,
	venueAddress: 500,
	kioskDescription: 500,
	url: 500,
	color: 20,
	// Mirrors services.MaxLayoutElements (not a text length: one saved layout).
	maxLayoutElements: 200
} as const;

// Passwords are ASCII only, so bcrypt's 72-byte ceiling equals 72 characters.
export const PASSWORD_PATTERN = '[!-~]{8,' + LIMITS.password + '}';
