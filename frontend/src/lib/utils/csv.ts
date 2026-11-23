import { LIMITS } from './limits.ts';

// parseCsv reads RFC 4180 text: quotes protect commas, newlines and "" escapes.
// Rows are returned verbatim so callers keep their own column mapping.
export function parseCsv(text: string): string[][] {
	const rows: string[][] = [];
	let row: string[] = [];
	let field = '';
	let quoted = false;
	for (let i = 0; i < text.length; i++) {
		const ch = text[i];
		if (quoted) {
			if (ch === '"') {
				if (text[i + 1] === '"') {
					field += '"';
					i++;
				} else {
					quoted = false;
				}
			} else {
				field += ch;
			}
			continue;
		}
		if (ch === '"' && field === '') {
			quoted = true;
		} else if (ch === ',') {
			row.push(field);
			field = '';
		} else if (ch === '\n') {
			row.push(field);
			rows.push(row);
			row = [];
			field = '';
		} else if (ch !== '\r') {
			field += ch;
		}
	}
	if (field !== '' || row.length > 0) {
		row.push(field);
		rows.push(row);
	}
	return rows.filter((r) => r.some((c) => c !== ''));
}

// unquoteField undoes the apostrophe guard exportCsv puts before a formula.
export function unquoteField(value: string): string {
	return value.replace(/^'([=+\-@\t\r])/, '$1');
}

const RSVP_VALUES = new Set(['confirmed', 'pending', 'declined', 'no_response']);
const RSVP_ALIASES: Record<string, string> = {
	yes: 'confirmed',
	attending: 'confirmed',
	true: 'confirmed',
	no: 'declined',
	maybe: 'pending'
};

export function normalizeRsvp(value: string | undefined): string {
	const v = (value ?? '').trim().toLowerCase();
	if (!v) return 'no_response';
	const mapped = RSVP_ALIASES[v] ?? v;
	return RSVP_VALUES.has(mapped) ? mapped : 'no_response';
}

export interface CsvGuestRow {
	name: string;
	email?: string;
	pax: number;
	rsvp: string;
	isVip: boolean;
	notes?: string;
	dietary: string[];
	table?: string;
	seat?: number;
}

// csvToGuests maps parsed rows to the import shape, dropping rows the API would
// reject (length caps) and clamping pax to the server's party limit.
export function csvToGuests(rows: string[][]): { guests: CsvGuestRow[]; dropped: number } {
	if (rows.length < 2) return { guests: [], dropped: 0 };
	const headers = rows[0].map((h) => h.trim().toLowerCase());
	const idx = (names: string[]) => headers.findIndex((h) => names.includes(h));
	const nameIdx = idx(['name']);
	if (nameIdx === -1) return { guests: [], dropped: 0 };
	const emailIdx = idx(['email']);
	const paxIdx = idx(['pax']);
	const rsvpIdx = idx(['rsvp']);
	const vipIdx = idx(['vip', 'isvip']);
	const notesIdx = idx(['notes']);
	const dietaryIdx = idx(['dietary']);
	const tableIdx = idx(['table', 'tablename']);
	const seatIdx = idx(['seat', 'seatnum', 'seat_number']);

	const guests: CsvGuestRow[] = [];
	let dropped = 0;
	for (const cells of rows.slice(1)) {
		const cols = cells.map((c) => unquoteField(c.trim()));
		const pick = (i: number) => (i >= 0 ? cols[i] : undefined) ?? '';
		const name = pick(nameIdx);
		const email = pick(emailIdx);
		const notes = pick(notesIdx);
		if (
			!name ||
			name.length > LIMITS.name ||
			email.length > LIMITS.email ||
			notes.length > LIMITS.guestNotes
		) {
			dropped++;
			continue;
		}
		const pax = Math.min(LIMITS.guestPax, Math.max(1, parseInt(pick(paxIdx)) || 1));
		const vip = pick(vipIdx).toLowerCase();
		guests.push({
			name,
			email: email || undefined,
			pax,
			rsvp: normalizeRsvp(pick(rsvpIdx)),
			isVip: vip === 'yes' || vip === '1' || vip === 'true',
			notes: notes || undefined,
			dietary: pick(dietaryIdx)
				.split(';')
				.map((d) => d.trim())
				.filter(Boolean),
			table: pick(tableIdx) || undefined,
			seat: parseInt(pick(seatIdx)) || undefined
		});
	}
	return { guests, dropped };
}
