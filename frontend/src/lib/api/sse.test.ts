import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { GuestEvent } from './sse';

// Mock EventSource globally
class MockEventSource {
	static instances: MockEventSource[] = [];
	url: string;
	onmessage: ((event: { data: string }) => void) | null = null;
	onerror: (() => void) | null = null;
	readyState = 0;
	private closed = false;

	constructor(url: string) {
		this.url = url;
		this.readyState = 1;
		MockEventSource.instances.push(this);
	}

	close() {
		this.closed = true;
		this.readyState = 2;
	}

	emit(data: GuestEvent) {
		if (this.onmessage && !this.closed) {
			this.onmessage({ data: JSON.stringify(data) });
		}
	}

	emitError() {
		if (this.onerror && !this.closed) {
			this.onerror();
		}
	}
}

vi.mock('$lib/stores', () => ({
	getAuth: () => ({ accessToken: 'test-token-123' })
}));

beforeEach(() => {
	MockEventSource.instances = [];
	(globalThis as any).EventSource = MockEventSource;
});

afterEach(() => {
	vi.restoreAllMocks();
	delete (globalThis as any).EventSource;
});

describe('SSEClient', () => {
	it('connects with token in query param', async () => {
		const { getSSIClient } = await import('./sse');
		const client = getSSIClient();
		client.connect('wedding-123');

		expect(MockEventSource.instances).toHaveLength(1);
		const es = MockEventSource.instances[0];
		expect(es.url).toContain('/api/weddings/wedding-123/events?token=test-token-123');

		client.disconnect();
	});

	it('receives and dispatches events to handlers', async () => {
		const { getSSIClient } = await import('./sse');
		const client = getSSIClient();
		client.connect('wedding-123');

		const handler = vi.fn();
		client.onEvent(handler);

		const event: GuestEvent = {
			type: 'checkin',
			guestId: 'g1',
			weddingId: 'wedding-123',
			guest: { id: 'g1', name: 'Alice', phone: '', email: '', pax: 1, rsvp: 'confirmed', isVip: false, notes: '', dietary: [], tableId: null, seatNum: null, checkedInAt: '2026-01-01T00:00:00Z', angbaoAmt: null, giftItem: null },
			timestamp: 1234567890
		};

		MockEventSource.instances[0].emit(event);

		expect(handler).toHaveBeenCalledOnce();
		expect(handler).toHaveBeenCalledWith(expect.objectContaining({ type: 'checkin', guestId: 'g1' }));

		client.disconnect();
	});

	it('unsubscribe removes handler', async () => {
		const { getSSIClient } = await import('./sse');
		const client = getSSIClient();
		client.connect('wedding-123');

		const handler = vi.fn();
		const unsub = client.onEvent(handler);
		unsub();

		const event: GuestEvent = {
			type: 'create',
			guestId: 'g2',
			weddingId: 'wedding-123',
			timestamp: 1234567890
		};
		MockEventSource.instances[0].emit(event);

		expect(handler).not.toHaveBeenCalled();
		client.disconnect();
	});

	it('disconnect closes EventSource and allows reconnect', async () => {
		const { getSSIClient } = await import('./sse');
		const client = getSSIClient();
		client.connect('wedding-123');
		expect(MockEventSource.instances).toHaveLength(1);

		client.disconnect();

		// New connect creates a fresh EventSource
		client.connect('wedding-123');
		expect(MockEventSource.instances).toHaveLength(2);
		client.disconnect();
	});
});
