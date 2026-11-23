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
