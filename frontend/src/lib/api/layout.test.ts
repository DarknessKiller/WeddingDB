import { describe, it, expect, vi, beforeEach } from 'vitest';

const apiFetch = vi.hoisted(() => vi.fn());
vi.mock('./client', () => ({ apiFetch }));

const payload = { version: 7, hallWidth: 860, hallHeight: 1000, tables: [], elements: [] };

beforeEach(() => apiFetch.mockReset());

describe('saveLayout', () => {
	it('sends the loaded version at the top level', async () => {
		apiFetch.mockResolvedValue({ ok: true, status: 200 } as Response);
		const { saveLayout } = await import('./layout');

		await saveLayout('w1', payload);

		expect(JSON.parse(apiFetch.mock.calls[0][1].body).version).toBe(7);
	});

	it('throws LayoutConflictError on 409', async () => {
		apiFetch.mockResolvedValue({ ok: false, status: 409 } as Response);
		const { saveLayout, LayoutConflictError } = await import('./layout');

		await expect(saveLayout('w1', payload)).rejects.toBeInstanceOf(LayoutConflictError);
	});
});
