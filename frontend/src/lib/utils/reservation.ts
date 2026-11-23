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
