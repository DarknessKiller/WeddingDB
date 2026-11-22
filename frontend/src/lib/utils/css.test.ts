import { describe, it, expect } from 'vitest';
import { cssImageUrl, isSafeImageUrl } from './css.ts';

describe('isSafeImageUrl', () => {
	it('accepts http(s) URLs, same-origin upload paths and the empty value', () => {
		expect(isSafeImageUrl('https://example.test/a.png')).toBe(true);
		expect(isSafeImageUrl('http://example.test/a.png')).toBe(true);
		expect(isSafeImageUrl('/uploads/x.png')).toBe(true);
		expect(isSafeImageUrl('')).toBe(true);
	});

	it.each(['javascript:alert(1)', 'data:image/png;base64,AAAA', '//evil.example/x', 'not a url'])(
		'rejects %j',
		(v) => expect(isSafeImageUrl(v)).toBe(false)
	);
});

describe('cssImageUrl', () => {
	it('emits a quoted url() and escapes quotes and backslashes', () => {
		expect(cssImageUrl('https://example.test/a.png')).toBe('url("https://example.test/a.png")');
		expect(cssImageUrl('/uploads/x.png')).toBe('url("/uploads/x.png")');
		expect(cssImageUrl('https://example.test/a"b\\c.png')).toBe('url("https://example.test/a\\"b\\\\c.png")');
	});

	it('emits none for a rejected URL or an empty value', () => {
		expect(cssImageUrl('javascript:alert(1)')).toBe('none');
		expect(cssImageUrl('')).toBe('none');
	});
});
