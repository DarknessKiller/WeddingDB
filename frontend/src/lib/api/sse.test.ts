import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('#lib/stores/index.ts', () => ({
	getAuth: () => ({ accessToken: 'test-token-123' })
}));

const refreshIfNeeded = vi.fn(async () => true);
vi.mock('#lib/api/client.ts', () => ({ refreshIfNeeded: () => refreshIfNeeded() }));

const fetchMock = vi.fn();

beforeEach(() => {
	vi.resetModules();
	fetchMock.mockReset();
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
		fetchMock.mockResolvedValueOnce({ ok: false, status: 401, body: null });
		const { getSSIClient } = await import('./sse');
		const client = getSSIClient();
		client.connect('wedding-123');
		await vi.waitFor(() => expect(refreshIfNeeded).toHaveBeenCalledOnce());
		client.disconnect();
	});

	it('flags the post-refresh connect as a reconnect so the caller resyncs', async () => {
		fetchMock
			.mockResolvedValueOnce({ ok: false, status: 401, body: null })
			.mockResolvedValue({ ok: true, status: 200, body: new ReadableStream({ start() {} }) });
		const { getSSIClient } = await import('./sse');
		const client = getSSIClient();
		const status = vi.fn();
		client.onStatus(status);
		client.connect('wedding-123');

		await vi.waitFor(() => expect(status).toHaveBeenCalledWith('connected', true));
		client.disconnect();
	});

	it('reconnects right after a successful refresh instead of waiting out the backoff', async () => {
		fetchMock
			.mockResolvedValueOnce({ ok: false, status: 401, body: null })
			.mockResolvedValue({ ok: true, status: 200, body: new ReadableStream({ start() {} }) });
		const { getSSIClient } = await import('./sse');
		const client = getSSIClient();
		client.connect('wedding-123');

		await vi.waitFor(() => expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(2), { timeout: 500 });
		client.disconnect();
	});
});
