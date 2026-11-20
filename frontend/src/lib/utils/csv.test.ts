import { describe, it, expect } from 'vitest';
import { parseCSVRows, parseGuestCSV } from './csv.ts';

describe('parseCSVRows', () => {
	it('keeps commas, doubled quotes and line breaks inside quoted cells', () => {
		expect(parseCSVRows('a,"Tan, Mr & Mrs","say ""hi""","two\nlines"\r\nb,c,d,e\n')).toEqual([
			['a', 'Tan, Mr & Mrs', 'say "hi"', 'two\nlines'],
			['b', 'c', 'd', 'e'],
		]);
	});

	it('drops the Excel UTF-8 BOM', () => {
		expect(parseCSVRows('\uFEFFname\n陈大文')).toEqual([['name'], ['陈大文']]);
	});
});

describe('parseGuestCSV', () => {
	it('reads the app export: quoted headers, formula guard, multi-line notes', () => {
		const csv = '"Name","Email","Table","Seat","Pax","RSVP","VIP","Checked In","Angbao","Gift","Notes"\n' +
			'"Tan, Mr & Mrs","","T1","3","2","confirmed","Yes","","","","line one\nline two"\n' +
			'"\'=Eve","","","","1","pending","No","","","",""';
		const got = parseGuestCSV(csv);
		expect(got).toHaveLength(2);
		expect(got[0]).toMatchObject({ name: 'Tan, Mr & Mrs', table: 'T1', seat: 3, pax: 2, rsvp: 'confirmed', isVip: true, notes: 'line one\nline two' });
		expect(got[1].name).toBe('=Eve');
		expect(got[1].table).toBeUndefined();
	});

	it('returns nothing without a name column', () => {
		expect(parseGuestCSV('email\na@b.c')).toEqual([]);
	});
});
