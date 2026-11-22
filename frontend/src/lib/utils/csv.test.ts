import { describe, it, expect } from 'vitest';
import { csvField } from './csv.ts';

describe('csvField', () => {
	it('quotes plain values and escapes internal quotes', () => {
		expect(csvField('Ann')).toBe('"Ann"');
		expect(csvField('Ann "Annie" Lee')).toBe('"Ann ""Annie"" Lee"');
		expect(csvField(null)).toBe('""');
	});

	it.each(['=SUM(A1)', '+1', '-1', '@cmd', '\tcmd', '\rcmd'])(
		'prefixes a formula character in %j with a single quote',
		(v) => {
			expect(csvField(v)).toBe(`"'${v.replace(/"/g, '""')}"`);
		}
	);

	it('leaves a formula character that is not first alone', () => {
		expect(csvField('a=1')).toBe('"a=1"');
	});
});
