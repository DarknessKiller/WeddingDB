import { describe, expect, it } from 'vitest';
import { parseCsv, unquoteField } from './csv';

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
