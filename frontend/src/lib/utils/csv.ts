import type { GuestImportData } from '#lib/api/guests.ts';

/**
 * Splits CSV text into rows of cells (RFC 4180): quoted cells may hold commas,
 * doubled quotes and line breaks. A leading UTF-8 BOM (Excel's "CSV UTF-8")
 * is dropped. Blank rows are kept, so rows[i] is spreadsheet row i + 1.
 */
export function parseCSVRows(text: string): string[][] {
	const src = text.charCodeAt(0) === 0xfeff ? text.slice(1) : text;
	const rows: string[][] = [];
	let row: string[] = [];
	let cell = '';
	let quoted = false;
	for (let i = 0; i < src.length; i++) {
		const c = src[i];
		if (quoted) {
			if (c === '"' && src[i + 1] === '"') { cell += '"'; i++; }
			else if (c === '"') quoted = false;
			else cell += c;
		} else if (c === '"') quoted = true;
		else if (c === ',') { row.push(cell); cell = ''; }
		else if (c === '\n' || c === '\r') {
			if (c === '\r' && src[i + 1] === '\n') i++;
			row.push(cell); rows.push(row); row = []; cell = '';
		} else cell += c;
	}
	if (cell !== '' || row.length > 0) { row.push(cell); rows.push(row); }
	return rows;
}

const isBlank = (r: string[]) => r.every(c => c.trim() === '');

// The app's own export prefixes formula-like cells with ' so spreadsheets
// don't run them; undo that on import.
function unguard(s: string): string {
	return /^'[=+\-@\t\r]/.test(s) ? s.slice(1) : s;
}

/** Parses a guest CSV with a "name" column; other columns are optional. */
export function parseGuestCSV(text: string): GuestImportData[] {
	const rows = parseCSVRows(text);
	const headerAt = rows.findIndex(r => !isBlank(r));
	if (headerAt === -1) return [];
	const header = rows[headerAt];
	const body = rows.slice(headerAt + 1);
	if (body.length === 0) return [];
	const headers = header.map(h => h.trim().toLowerCase());
	const col = (...names: string[]) => headers.findIndex(h => names.includes(h));
	const nameIdx = col('name');
	if (nameIdx === -1) return [];
	const emailIdx = col('email');
	const paxIdx = col('pax');
	const rsvpIdx = col('rsvp');
	const vipIdx = col('vip', 'isvip');
	const notesIdx = col('notes');
	const dietaryIdx = col('dietary');
	const tableIdx = col('table', 'tablename');
	const seatIdx = col('seat', 'seatnum', 'seat_number');

	return body.map((raw, i) => {
		const cols = raw.map(c => unguard(c.trim()));
		const vip = vipIdx >= 0 ? cols[vipIdx]?.toLowerCase() : '';
		return {
			name: cols[nameIdx] || '',
			email: emailIdx >= 0 ? cols[emailIdx] : undefined,
			pax: paxIdx >= 0 ? parseInt(cols[paxIdx]) || 1 : 1,
			// Hand-made sheets write "Confirmed"; the server only knows lowercase values.
			rsvp: rsvpIdx >= 0 ? cols[rsvpIdx]?.toLowerCase() : 'no_response',
			isVip: vip === 'yes' || vip === '1' || vip === 'true',
			notes: notesIdx >= 0 ? cols[notesIdx] : undefined,
			dietary: dietaryIdx >= 0 ? (cols[dietaryIdx] ?? '').split(';').map(d => d.trim()).filter(Boolean) : [],
			table: tableIdx >= 0 ? cols[tableIdx] || undefined : undefined,
			seat: seatIdx >= 0 ? parseInt(cols[seatIdx]) || undefined : undefined,
			// Spreadsheet row number, so an import error names the line to fix.
			row: headerAt + i + 2,
		};
	}).filter(g => g.name);
}
