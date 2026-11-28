import { z } from 'zod';
import { LIMITS } from './limits.ts';

/** Reservation form schema; the caps mirror the guest create endpoint. */
export const reservationSchema = z.object({
	name: z.string().min(1, 'Name is required').max(LIMITS.name),
	email: z.string().email('Invalid email').max(LIMITS.email).optional().or(z.literal('')),
	pax: z.number().min(1, 'At least 1 guest').max(LIMITS.guestPax, `Max ${LIMITS.guestPax} guests`),
	rsvp: z.enum(['confirmed', 'pending', 'declined']),
	tableId: z.string().nullable().optional(),
	seatNumbers: z.array(z.number()).optional(),
	dietary: z.array(z.string()).optional(),
	notes: z.string().max(LIMITS.guestNotes).optional(),
	isVip: z.boolean().optional(),
});

export type ReservationForm = z.infer<typeof reservationSchema>;

const ANGBAO_ERROR = `Angbao must be a whole number from 0 to ${LIMITS.angbaoAmt.toLocaleString('en')}`;

/** Empty, or one bare address. Stricter than services.ValidGuestEmail, never looser. */
export const guestEmailSchema = z.union([z.literal(''), z.email('Invalid email').max(LIMITS.email)]).optional();

/** Guest drawer schema; the caps mirror the guest create and update endpoints. */
export const guestFormSchema = z.object({
	name: z.string().trim().min(1, 'Name is required').max(LIMITS.name, `Max ${LIMITS.name} characters`),
	email: guestEmailSchema,
	pax: z.number({ error: 'Pax must be a whole number' }).int('Pax must be a whole number')
		.min(1, 'At least 1 guest').max(LIMITS.guestPax, `Max ${LIMITS.guestPax} guests`),
	// A number input reads '' or null when empty: both mean no angbao.
	angbaoAmt: z.union([z.literal(''), z.null(), z.undefined(), z.number().int(ANGBAO_ERROR).min(0, ANGBAO_ERROR).max(LIMITS.angbaoAmt, ANGBAO_ERROR)], { error: ANGBAO_ERROR }),
});

/** Per-field messages for the guest drawer form, empty when it may be saved. */
export function guestFormErrors(form: unknown): { name?: string[]; email?: string[]; pax?: string[]; angbaoAmt?: string[] } {
	const r = guestFormSchema.safeParse(form);
	return r.success ? {} : z.flattenError(r.error).fieldErrors;
}

// seatSelectionError returns the seat rule violation for a table reservation,
// or undefined when the selection covers the party or no table is chosen.
export function seatSelectionError(form: {
	tableId?: string | null;
	seatNumbers?: number[];
	pax: number;
}): string | undefined {
	if (!form.tableId) return undefined;
	if (!form.seatNumbers || form.seatNumbers.length === 0) return 'Select at least one seat';
	if (form.seatNumbers.length < form.pax) {
		return `Select ${form.pax} seat(s) for ${form.pax} guest(s)`;
	}
	return undefined;
}

// closestEmptySeats returns the first contiguous run of `count` free seats, so
// the server's seatNum..seatNum+pax-1 assignment matches the seats shown.
export function closestEmptySeats(capacity: number, occupied: Set<number>, count: number): number[] {
	if (count < 1) return [];
	for (let start = 1; start + count - 1 <= capacity; start++) {
		let free = true;
		for (let seat = start; seat < start + count; seat++) {
			if (occupied.has(seat)) { free = false; break; }
		}
		if (free) return Array.from({ length: count }, (_, i) => start + i);
	}
	return [];
}

// seatOccupant returns the guest whose booked range covers seatNum, if any.
// A party occupies seatNum..seatNum+pax-1, the same range the server checks.
export function seatOccupant<T extends { tableId: string | null; seatNumber: number | null; pax: number }>(
	guests: T[], tableId: string, seatNum: number
): T | undefined {
	return guests.find(g =>
		g.tableId === tableId &&
		g.seatNumber !== null &&
		seatNum >= g.seatNumber &&
		seatNum < g.seatNumber + g.pax
	);
}
