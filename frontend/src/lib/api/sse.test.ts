import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('#lib/stores/index.ts', () => ({
	getAuth: () => ({ accessToken: 'test-token-123' })
}));

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

	it('refreshes the access token when the stream is rejected with 401', async () => {
		const refreshAccessToken = vi.fn(async () => true);
		vi.doMock('#lib/api/client', () => ({ refreshAccessToken }));
		fetchMock.mockResolvedValueOnce({ ok: false, status: 401, body: null });

		const { getSSIClient } = await import('./sse');
		const client = getSSIClient();
		client.connect('wedding-123');

		await vi.waitFor(() => expect(refreshAccessToken).toHaveBeenCalledOnce());
		client.disconnect();
		vi.doUnmock('#lib/api/client');
	});

	it('reconnects when the stream goes silent past the idle timeout', async () => {
		vi.useFakeTimers();
		try {
			// Like a browser: aborting the request errors its body stream.
			fetchMock.mockImplementation(async (_url: string, init: RequestInit) => ({
				ok: true,
				status: 200,
				body: new ReadableStream({
					start(c) {
						init.signal?.addEventListener('abort', () => c.error(new DOMException('aborted', 'AbortError')));
					}
				})
			}));
			const { getSSIClient } = await import('./sse');
			const client = getSSIClient();
			const status = vi.fn();
			client.onStatus(status);
			client.connect('wedding-123');
			await vi.advanceTimersByTimeAsync(0);
			expect(status).toHaveBeenCalledWith('connected', false);

			await vi.advanceTimersByTimeAsync(45_000);
			expect(status).toHaveBeenCalledWith('reconnecting', false);
			await vi.advanceTimersByTimeAsync(1_000);
			expect(fetchMock).toHaveBeenCalledTimes(2);
			client.disconnect();
		} finally {
			vi.useRealTimers();
		}
	});
});
