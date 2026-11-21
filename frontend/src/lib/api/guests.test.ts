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
