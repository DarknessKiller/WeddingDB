import { describe, it, expect, vi, beforeEach } from 'vitest';
import { registerEscape } from '#lib/utils/focus.ts';

// Minimal window: the escape stack only needs add/removeEventListener plus a way
// to fire the capture-phase keydown.
const listeners = new Set<(e: KeyboardEvent) => void>();
vi.stubGlobal('window', {
	addEventListener: (_t: string, fn: (e: KeyboardEvent) => void) => void listeners.add(fn),
	removeEventListener: (_t: string, fn: (e: KeyboardEvent) => void) => void listeners.delete(fn)
});

function pressEscape() {
	const e = { key: 'Escape', stopImmediatePropagation: vi.fn() } as unknown as KeyboardEvent;
	for (const fn of [...listeners]) fn(e);
	return e;
}

describe('registerEscape', () => {
	beforeEach(() => listeners.clear());

	it('only the dialog opened last handles Escape', () => {
		const under = vi.fn();
		const over = vi.fn();
		const releaseUnder = registerEscape(under);
		const releaseOver = registerEscape(over);

		pressEscape();
		expect(over).toHaveBeenCalledTimes(1);
		expect(under).not.toHaveBeenCalled();

		releaseOver();
		pressEscape();
		expect(under).toHaveBeenCalledTimes(1);

		releaseUnder();
		expect(listeners.size).toBe(0);
	});

	it('swallows the key so page-level Escape handlers stay quiet', () => {
		const release = registerEscape(() => {});
		const e = pressEscape();
		expect(e.stopImmediatePropagation).toHaveBeenCalled();
		release();
	});
});
