import { describe, expect, it } from 'vitest';
import { csvToGuests, parseCsv, resolveImportTables, unquoteField } from './csv';

describe('parseCsv', () => {
	it('keeps commas and quotes inside quoted fields', () => {
		const rows = parseCsv('Name,Notes\n"Doe, Jane","No beef, extra rice"\n"He said ""hi""",ok');
		expect(rows[1][0]).toBe('Doe, Jane');
		expect(rows[1][1]).toBe('No beef, extra rice');
		expect(rows[2][0]).toBe('He said "hi"');
	});

	it('accepts CRLF and drops blank trailing rows', () => {
		expect(parseCsv('a,b\r\n1,2\r\n')).toEqual([
			['a', 'b'],
			['1', '2']
		]);
	});

	it('reads a newline inside a quoted field', () => {
		expect(parseCsv('a\n"line1\nline2"')[1][0]).toBe('line1\nline2');
	});
});

describe('unquoteField', () => {
	it('removes only the export formula guard', () => {
		expect(unquoteField("'=HYPERLINK(1)")).toBe('=HYPERLINK(1)');
		expect(unquoteField("'Quoted Name")).toBe("'Quoted Name");
	});
});

describe('csvToGuests', () => {
	const header = 'name,email,pax,rsvp,vip,notes,table,seat';

	it('clamps pax and normalizes known RSVP spellings', () => {
		const { guests } = csvToGuests(parseCsv(header + '\nAlice,a@example.com,51,Yes,true,ok,Table 1,3'));
		expect(guests[0].pax).toBe(50);
		expect(guests[0].rsvp).toBe('confirmed');
		expect(guests[0].isVip).toBe(true);
	});

	it('drops rows the API would reject for length', () => {
		const long = 'x'.repeat(256);
		const { guests, dropped } = csvToGuests(parseCsv(header + '\n' + long + ',,,,,,'));
		expect(guests).toHaveLength(0);
		expect(dropped).toBe(1);
	});
});

describe('csvToGuests dietary and table resolution', () => {
	it('drops a row with a dietary item over the limit', () => {
		const { guests, dropped } = csvToGuests(parseCsv('name,dietary\nAlice,' + 'x'.repeat(101)));
		expect(guests).toHaveLength(0);
		expect(dropped).toBe(1);
	});

	it('drops the seat when the table name does not resolve', () => {
		const { rows, unresolved } = resolveImportTables(
			[
				{ table: 'Table 1', seat: 3 },
				{ table: 'Gone', seat: 4 }
			],
			[{ id: 't1', name: 'table 1' }]
		);
		expect(rows[0]).toMatchObject({ tableId: 't1', seat: 3 });
		expect(rows[1]).toMatchObject({ tableId: undefined, seat: undefined });
		expect(unresolved).toBe(1);
	});
});
