import type { Guest, RSVPStatus } from '#lib/types/index.ts';
import type { GuestCreateData, GuestResponse } from '#lib/api/guests.ts';

/** A guest row in API names. SSE events omit weddingId and the timestamps. */
export type GuestRow = Omit<GuestResponse, 'weddingId' | 'createdAt' | 'updatedAt'> &
	Partial<Pick<GuestResponse, 'createdAt' | 'updatedAt'>>;

/** Patch fields in API names. undefined keeps the old value, null clears it. */
export type GuestFields = Partial<GuestCreateData>;

/** A guest row the public (kiosk) endpoint returns: staff-only fields are absent. */
export type PublicGuestRow = Pick<GuestResponse, 'id' | 'name' | 'tableId' | 'seatNum' | 'pax' | 'isVip'> &
	Partial<Pick<GuestResponse, 'pinyin'>>;

const RSVP: RSVPStatus[] = ['confirmed', 'pending', 'declined', 'no_response'];
function rsvp(v: unknown): RSVPStatus {
	return RSVP.includes(v as RSVPStatus) ? (v as RSVPStatus) : 'no_response';
}

/** The one API-row-to-Guest mapping. */
export function toGuest(r: GuestRow): Guest {
	const createdAt = r.createdAt ? new Date(r.createdAt) : new Date();
	return {
		id: r.id,
		name: r.name ?? '',
		pinyin: r.pinyin,
		email: r.email,
		rsvp: rsvp(r.rsvp),
		pax: r.pax ?? 1,
		tableId: r.tableId ?? null,
		seatNumber: r.seatNum ?? null,
		checkedIn: !!r.checkedInAt,
		checkedInAt: r.checkedInAt ? new Date(r.checkedInAt) : undefined,
		notes: r.notes ?? '',
		dietaryRequirements: r.dietary ?? [],
		isVip: !!r.isVip,
		angbaoAmount: r.angbaoAmt ?? undefined,
		giftItem: r.giftItem ?? undefined,
		createdAt,
		updatedAt: r.updatedAt ? new Date(r.updatedAt) : createdAt
	};
}

/** Public row to Guest. Same defaults as toGuest; staff-only fields never carry a value. */
export function toPublicGuest(r: PublicGuestRow): Guest {
	return {
		id: r.id,
		name: r.name ?? '',
		pinyin: r.pinyin,
		rsvp: 'no_response', // RSVP is staff-only; public rows never carry it
		pax: r.pax ?? 1,
		tableId: r.tableId ?? null,
		seatNumber: r.seatNum ?? null,
		checkedIn: false, // attendance is staff-only; never expose it to the kiosk
		notes: '',
		dietaryRequirements: [],
		isVip: !!r.isVip,
		createdAt: new Date()
	};
}

/** A Guest read back from JSON (offline cache): only the dates need reviving. */
export function reviveGuest(g: Guest): Guest {
	return {
		...g,
		checkedInAt: g.checkedInAt ? new Date(g.checkedInAt) : undefined,
		createdAt: new Date(g.createdAt),
		updatedAt: g.updatedAt ? new Date(g.updatedAt) : undefined
	};
}

function pick<T>(v: T | undefined, old: T): T {
	return v === undefined ? old : v;
}

/** Applies API-named patch fields to a Guest. */
export function applyGuestPatch(g: Guest, p: GuestFields): Guest {
	return {
		...g,
		name: pick(p.name, g.name),
		email: pick(p.email, g.email),
		pax: pick(p.pax, g.pax),
		rsvp: p.rsvp === undefined ? g.rsvp : rsvp(p.rsvp),
		isVip: pick(p.isVip, g.isVip),
		notes: pick(p.notes, g.notes),
		dietaryRequirements: pick(p.dietary, g.dietaryRequirements),
		tableId: p.tableId === undefined ? g.tableId : p.tableId || null,
		seatNumber: p.seatNum === undefined ? g.seatNumber : p.seatNum ?? null,
		angbaoAmount: p.angbaoAmt === undefined ? g.angbaoAmount : p.angbaoAmt ?? undefined,
		giftItem: p.giftItem === undefined ? g.giftItem : p.giftItem ?? undefined
	};
}
