import { describe, it, expect, vi, beforeEach } from 'vitest';

const enqueue = vi.hoisted(() => vi.fn());
const apiFetch = vi.hoisted(() => vi.fn());
vi.mock('#lib/offline/queue.ts', async (orig) => ({ ...(await orig<object>()), enqueue }));
vi.mock('./client', async (orig) => ({ ...(await orig<object>()), apiFetch }));
vi.mock('./client.ts', async (orig) => ({ ...(await orig<object>()), apiFetch }));

beforeEach(() => {
	enqueue.mockReset();
	apiFetch.mockReset();
});

describe('createGuest', () => {
	it('queues the same id it sent when the response is lost', async () => {
		apiFetch.mockRejectedValue(new TypeError('Failed to fetch'));
		const { createGuest } = await import('./guests');
		const g = await createGuest('w1', { name: 'A', pax: 1, rsvp: 'pending', isVip: false } as never);
		const sent = JSON.parse(apiFetch.mock.calls[0][1].body).id;
		expect(sent).toMatch(/^[0-9a-f-]{36}$/);
		expect(enqueue.mock.calls[0][1].guestId).toBe(sent);
		expect(g.id).toBe(sent);
	});
});

describe('updateGuest', () => {
	it('sends the loaded row version and reports 409 as a conflict', async () => {
		apiFetch.mockResolvedValue({ ok: false, status: 409, json: async () => ({ title: 'Guest changed on another device; reload and try again' }) });
		const { updateGuest, ConflictError } = await import('./guests');
		const data = { name: 'A', pax: 1, rsvp: 'pending', isVip: false, notes: '', dietary: [], updatedAt: '2026-01-01T00:00:00Z' };
		await expect(updateGuest('w1', 'g1', data as never)).rejects.toBeInstanceOf(ConflictError);
		const body = JSON.parse(apiFetch.mock.calls[0][1].body);
		expect(body.updatedAt).toBe('2026-01-01T00:00:00Z');
	});
});
