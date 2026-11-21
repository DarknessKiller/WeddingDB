import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('#lib/stores/index.ts', () => ({
	getAuth: () => ({ accessToken: 'test-token-123' })
}));

const refreshSession = vi.hoisted(() => vi.fn());
vi.mock('#lib/api/client.ts', () => ({ refreshSession }));

const fetchMock = vi.fn();

beforeEach(() => {
	vi.resetModules();
	fetchMock.mockReset();
	refreshSession.mockReset();
	fetchMock.mockResolvedValue({
		ok: true,
		status: 200,
		body: new ReadableStream({ start() {} })
	});
	vi.stubGlobal('fetch', fetchMock);
});

describe('SSEClient', () => {
	it('streams with bearer header and no token in URL', async () => {
		const { getSSIClient } = await import('./sse');
		const client = getSSIClient();
		client.connect('wedding-123');
		await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledOnce());

		expect(fetchMock.mock.calls[0][0]).toBe('/api/weddings/wedding-123/events');
		expect(fetchMock.mock.calls[0][1].headers.Authorization).toBe('Bearer test-token-123');
		expect(fetchMock.mock.calls[0][0]).not.toContain('token=');
		client.disconnect();
	});

	it('parses and dispatches SSE data events', async () => {
		const event = { type: 'checkin', guestId: 'g1', weddingId: 'wedding-123', timestamp: 1234567890 };
		fetchMock.mockResolvedValueOnce({
			ok: true,
			status: 200,
			body: new ReadableStream({
				start(controller) {
					controller.enqueue(new TextEncoder().encode(`data: ${JSON.stringify(event)}\n\n`));
				}
			})
		});
		const { getSSIClient } = await import('./sse');
		const client = getSSIClient();
		const handler = vi.fn();
		client.onEvent(handler);
		client.connect('wedding-123');

		await vi.waitFor(() => expect(handler).toHaveBeenCalledWith(event));
		client.disconnect();
	});

	it('refreshes the token on 401 before reconnecting', async () => {
		vi.useFakeTimers();
		refreshSession.mockResolvedValue('refreshed');
		fetchMock.mockResolvedValueOnce({ ok: false, status: 401, body: null });
		const { getSSIClient } = await import('./sse');
		const client = getSSIClient();
		client.connect('wedding-123');

		await vi.waitFor(() => expect(refreshSession).toHaveBeenCalledOnce());
		await vi.advanceTimersByTimeAsync(1000);
		expect(fetchMock).toHaveBeenCalledTimes(2);
		client.disconnect();
		vi.useRealTimers();
	});

	it('stops reconnecting when the refresh ends the session', async () => {
		vi.useFakeTimers();
		refreshSession.mockResolvedValue('ended');
		fetchMock.mockResolvedValueOnce({ ok: false, status: 401, body: null });
		const { getSSIClient } = await import('./sse');
		const client = getSSIClient();
		client.connect('wedding-123');

		await vi.waitFor(() => expect(refreshSession).toHaveBeenCalledOnce());
		await vi.advanceTimersByTimeAsync(60000);
		expect(fetchMock).toHaveBeenCalledTimes(1);
		vi.useRealTimers();
	});
});
