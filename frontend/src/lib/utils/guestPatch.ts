import type { Guest } from '#lib/types/index.ts';

export interface GuestFormValues {
	name: string;
	email: string;
	pax: number;
	rsvp: string;
	isVip: boolean;
	notes: string;
	dietary: string[];
	angbaoAmt: string;
	giftItem: string;
}

export interface GuestPatch {
	name?: string;
	email?: string;
	pax?: number;
	rsvp?: string;
	isVip?: boolean;
	notes?: string;
	dietary?: string[];
	angbaoAmt?: number | null;
	giftItem?: string | null;
}

/** Only the fields the user changed, in API names. Cleared strings are "",
 * cleared nullable fields are null, so the server applies the clear. */
export function guestPatch(guest: Guest, form: GuestFormValues): GuestPatch {
	const patch: GuestPatch = {};
	if (form.name !== guest.name) patch.name = form.name;
	if (form.email !== (guest.email ?? '')) patch.email = form.email;
	if (form.pax !== guest.pax) patch.pax = form.pax;
	if (form.rsvp !== guest.rsvp) patch.rsvp = form.rsvp;
	if (form.isVip !== guest.isVip) patch.isVip = form.isVip;
	if (form.notes !== guest.notes) patch.notes = form.notes;
	if (!sameList(form.dietary, guest.dietaryRequirements)) patch.dietary = form.dietary;
	const angbaoRaw = String(form.angbaoAmt ?? '').trim();
	const angbaoAmt = angbaoRaw === '' ? null : Number(angbaoRaw);
	if (angbaoAmt !== (guest.angbaoAmount ?? null)) patch.angbaoAmt = angbaoAmt;
	const giftItem = form.giftItem.trim() === '' ? null : form.giftItem;
	if (giftItem !== (guest.giftItem ?? null)) patch.giftItem = giftItem;
	return patch;
}

function sameList(a: string[], b: string[]): boolean {
	return a.length === b.length && a.every((v, i) => v === b[i]);
}
