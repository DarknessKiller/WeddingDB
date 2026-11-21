import { describe, it, expect } from 'vitest';
import { parseCSVRows, parseGuestCSV } from './csv';

describe('parseCSVRows', () => {
	it('keeps commas, escaped quotes and newlines inside quoted fields', () => {
		const text = 'name,notes\r\n"Tan, Mei","said ""hi""\nthen left"\nBob,plain\n';
		expect(parseCSVRows(text)).toEqual([
			['name', 'notes'],
			['Tan, Mei', 'said "hi"\nthen left'],
			['Bob', 'plain']
		]);
	});

	it('strips a leading BOM and keeps a last line without newline', () => {
		expect(parseCSVRows('\uFEFFname\nAlice')).toEqual([['name'], ['Alice']]);
	});

	it('keeps empty fields', () => {
		expect(parseCSVRows('a,,c')).toEqual([['a', '', 'c']]);
	});
});

describe('parseGuestCSV', () => {
	it('maps columns and splits dietary on semicolons', () => {
		const { guests, problems } = parseGuestCSV('Name,Pax,VIP,Dietary,Table\n"Lee, Ann",3,yes,vegan; nuts,T1\n');
		expect(problems).toEqual([]);
		expect(guests).toEqual([expect.objectContaining({ name: 'Lee, Ann', pax: 3, isVip: true, dietary: ['vegan', 'nuts'], table: 'T1' })]);
	});

	it('moves rows over a LIMITS cap to problems with their row number', () => {
		const long = 'x'.repeat(256);
		const { guests, problems } = parseGuestCSV(`name,pax\nAlice,2\n${long},1\nBob,51\n`);
		expect(guests.map(g => g.name)).toEqual(['Alice']);
		expect(problems).toEqual([
			{ row: 3, name: long, errors: ['name over 255 characters'] },
			{ row: 4, name: 'Bob', errors: ['pax must be 1 to 50'] }
		]);
	});

	it('returns nothing without a name column', () => {
		expect(parseGuestCSV('email\na@b.c\n')).toEqual({ guests: [], problems: [] });
	});
});
