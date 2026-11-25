import { LIMITS } from './limits.ts';
import { guestEmailSchema } from './reservation.ts';

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

// charLen counts code points, matching the backend's rune caps.
function charLen(value: string): number {
	return [...value].length;
}

export function normalizeRsvp(value: string | undefined): string {
	const v = (value ?? '').trim().toLowerCase();
	if (!v) return 'no_response';
	const mapped = RSVP_ALIASES[v] ?? v;
	return RSVP_VALUES.has(mapped) ? mapped : 'no_response';
}

export interface CsvGuestRow {
	id?: string;
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

// resolveImportTables matches a CSV table name to a known table and drops the
// seat when the table did not resolve: the API rejects a seat without a table.
export function resolveImportTables<T extends { table?: string; seat?: number }>(
	rows: T[],
	tables: { id: string; name: string }[]
): { rows: (T & { tableId?: string; seat?: number })[]; unresolved: number } {
	let unresolved = 0;
	const resolved = rows.map((g) => {
		if (!g.table) {
			if (g.seat !== undefined) unresolved++;
			return { ...g, seat: undefined };
		}
		const match = tables.find((t) => t.name.toLowerCase() === g.table!.toLowerCase());
		if (!match) unresolved++;
		return { ...g, tableId: match?.id, seat: match ? g.seat : undefined };
	});
	return { rows: resolved, unresolved };
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
		const dietary = pick(dietaryIdx)
			.split(';')
			.map((d) => d.trim())
			.filter(Boolean);
		if (
			!name ||
			charLen(name) > LIMITS.name ||
			charLen(email) > LIMITS.email ||
			!guestEmailSchema.safeParse(email).success ||
			charLen(notes) > LIMITS.guestNotes ||
			dietary.length > LIMITS.dietaryItems ||
			dietary.some((d) => charLen(d) > LIMITS.dietaryItem)
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
			dietary,
			table: pick(tableIdx) || undefined,
			seat: parseInt(pick(seatIdx)) > 0 ? parseInt(pick(seatIdx)) : undefined
		});
	}
	return { guests, dropped };
}
