import { LIMITS } from '#lib/utils/limits.ts';
import type { GuestImportData } from '#lib/api/guests.ts';

/** RFC 4180 records: quoted fields, "" escapes, commas and newlines inside quotes. */
export function parseCSVRows(text: string): string[][] {
	const rows: string[][] = [];
	let row: string[] = [];
	let field = '';
	let quoted = false;
	const src = text.replace(/^\uFEFF/, ''); // Excel writes a BOM
	for (let i = 0; i < src.length; i++) {
		const c = src[i];
		if (quoted) {
			if (c !== '"') field += c;
			else if (src[i + 1] === '"') { field += '"'; i++; }
			else quoted = false;
		} else if (c === '"') {
			quoted = true;
		} else if (c === ',') {
			row.push(field);
			field = '';
		} else if (c === '\n' || c === '\r') {
			if (c === '\r' && src[i + 1] === '\n') i++;
			row.push(field);
			rows.push(row);
			row = [];
			field = '';
		} else {
			field += c;
		}
	}
	if (field || row.length) {
		row.push(field);
		rows.push(row);
	}
	return rows;
}

export interface ImportProblem {
	row: number; // 1-based CSV record, header included
	name: string;
	errors: string[];
}

/** Map CSV records to guests. Rows breaking a LIMITS cap go to problems, not guests. */
export function parseGuestCSV(text: string): { guests: GuestImportData[]; problems: ImportProblem[] } {
	const records = parseCSVRows(text);
	if (records.length < 2) return { guests: [], problems: [] };
	const headers = records[0].map(h => h.trim().toLowerCase());
	const nameIdx = headers.findIndex(h => h === 'name');
	if (nameIdx === -1) return { guests: [], problems: [] };
	const emailIdx = headers.findIndex(h => h === 'email');
	const paxIdx = headers.findIndex(h => h === 'pax');
	const rsvpIdx = headers.findIndex(h => h === 'rsvp');
	const vipIdx = headers.findIndex(h => h === 'vip' || h === 'isvip');
	const notesIdx = headers.findIndex(h => h === 'notes');
	const dietaryIdx = headers.findIndex(h => h === 'dietary');
	const tableIdx = headers.findIndex(h => h === 'table' || h === 'tablename');
	const seatIdx = headers.findIndex(h => h === 'seat' || h === 'seatnum' || h === 'seat_number');

	const guests: GuestImportData[] = [];
	const problems: ImportProblem[] = [];
	records.slice(1).forEach((raw, i) => {
		const cols = raw.map(c => c.trim());
		const vip = vipIdx >= 0 ? cols[vipIdx]?.toLowerCase() : '';
		const g: GuestImportData = {
			name: cols[nameIdx] || '',
			email: emailIdx >= 0 ? cols[emailIdx] : undefined,
			pax: paxIdx >= 0 ? parseInt(cols[paxIdx]) || 1 : 1,
			rsvp: rsvpIdx >= 0 ? cols[rsvpIdx] : 'no_response',
			isVip: vip === 'yes' || vip === '1' || vip === 'true',
			notes: notesIdx >= 0 ? cols[notesIdx] : undefined,
			dietary: dietaryIdx >= 0 ? cols[dietaryIdx]?.split(';').map(d => d.trim()).filter(Boolean) : [],
			table: tableIdx >= 0 ? cols[tableIdx] : undefined,
			seat: seatIdx >= 0 ? parseInt(cols[seatIdx]) || undefined : undefined,
		};
		if (!g.name) return;
		const errors: string[] = [];
		if (g.name.length > LIMITS.name) errors.push(`name over ${LIMITS.name} characters`);
		if ((g.email?.length ?? 0) > LIMITS.email) errors.push(`email over ${LIMITS.email} characters`);
		if ((g.notes?.length ?? 0) > LIMITS.guestNotes) errors.push(`notes over ${LIMITS.guestNotes} characters`);
		if (g.pax! < 1 || g.pax! > LIMITS.guestPax) errors.push(`pax must be 1 to ${LIMITS.guestPax}`);
		if ((g.dietary?.length ?? 0) > LIMITS.dietaryItems) errors.push(`more than ${LIMITS.dietaryItems} dietary items`);
		if (g.dietary?.some(d => d.length > LIMITS.dietaryItem)) errors.push(`a dietary item over ${LIMITS.dietaryItem} characters`);
		if (errors.length) problems.push({ row: i + 2, name: g.name, errors });
		else guests.push(g);
	});
	return { guests, problems };
}
