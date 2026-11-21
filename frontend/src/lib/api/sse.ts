import { getAuth } from '#lib/stores/index.ts';
import { refreshAccessToken, reselectWedding } from '#lib/api/client.ts';

export interface GuestEvent {
	type: 'create' | 'update' | 'delete' | 'checkin' | 'checkout' | 'seat_assign';
	guestId: string;
	weddingId: string;
	guest?: {
		id: string;
		name: string;
		pinyin?: string;
		email: string;
		pax: number;
		rsvp: string;
		isVip: boolean;
		notes: string;
		dietary: string[];
		tableId: string | null;
		seatNum: number | null;
		checkedInAt: string | null;
		angbaoAmt: number | null;
		giftItem: string | null;
	};
	timestamp: number;
}

type EventHandler = (event: GuestEvent) => void;

// The server pings every 20s. Silence past this means the connection died
// without closing (e.g. the phone switched WiFi access point).
const IDLE_TIMEOUT_MS = 45_000;
type StatusHandler = (status: 'connected' | 'reconnecting', reconnect: boolean) => void;

class SSEClient {
	private controller: AbortController | null = null;
	private handlers: Set<EventHandler> = new Set();
	private statusHandlers: Set<StatusHandler> = new Set();
	private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
	private reconnectDelay = 1000;
	private maxReconnectDelay = 30000;
	private wid = '';

	connect(weddingId: string) {
		this.controller?.abort();
		this.controller = null;
		if (this.reconnectTimeout) clearTimeout(this.reconnectTimeout);
		this.reconnectTimeout = null;
		this.wid = weddingId;
		const { accessToken } = getAuth();
		if (!accessToken) return;
		const controller = new AbortController();
		this.controller = controller;
		void this.stream(weddingId, accessToken, controller);
	}

	private async stream(weddingId: string, accessToken: string, controller: AbortController) {
		let idle = false;
		let idleTimer: ReturnType<typeof setTimeout> | undefined;
		const resetIdle = () => {
			clearTimeout(idleTimer);
			idleTimer = setTimeout(() => {
				idle = true;
				controller.abort();
			}, IDLE_TIMEOUT_MS);
		};
		resetIdle();
		try {
			const response = await fetch(`/api/weddings/${weddingId}/events`, {
				headers: { Authorization: `Bearer ${accessToken}` },
				signal: controller.signal
			});
			if (response.status === 401) {
				// Refresh once, then let the reconnect path retry with the new token.
				await refreshAccessToken();
				throw new Error('SSE unauthorized');
			}
			if (response.status === 403) {
				// Another tab rescoped the shared token to its wedding: re-pick ours, then reconnect.
				await reselectWedding(weddingId);
				throw new Error('SSE forbidden');
			}
			if (!response.ok || !response.body) throw new Error(`SSE request failed: ${response.status}`);

			const reconnect = this.reconnectDelay > 1000;
			this.openedAt = Date.now();
			this.statusHandlers.forEach((handler) => handler('connected', reconnect));
			const reader = response.body.getReader();
			const decoder = new TextDecoder();
			let buffer = '';
			while (true) {
				const { value, done } = await reader.read();
				resetIdle();
				buffer += decoder.decode(value, { stream: !done });
				const messages = buffer.split(/\r?\n\r?\n/);
				buffer = messages.pop() ?? '';
				for (const message of messages) {
					const data = message.split(/\r?\n/).filter((line) => line.startsWith('data:')).map((line) => line.slice(5).trimStart()).join('\n');
					if (!data) continue;
					try {
						const event: GuestEvent = JSON.parse(data);
						this.handlers.forEach((handler) => handler(event));
					} catch (e) {
						console.error('SSE: Failed to parse event', e);
					}
				}
				if (done) break;
			}
		} catch {
			// A deliberate abort (disconnect or a new connect) is not a drop; an idle timeout is.
			if (controller.signal.aborted && !idle) return;
			this.statusHandlers.forEach((handler) => handler('reconnecting', false));
		} finally {
			clearTimeout(idleTimer);
			if (this.controller === controller) {
				this.controller = null;
				if (this.openedAt && Date.now() - this.openedAt >= 30_000) this.reconnectDelay = 1000;
				this.openedAt = 0;
				this.scheduleReconnect();
			}
		}
	}

	// When the current stream opened. Backoff only resets once a stream has
	// stayed up, so a server that accepts then closes (per-user cap eviction)
	// cannot drive a reconnect every second.
	private openedAt = 0;

	disconnect() {
		this.controller?.abort();
		this.controller = null;
		if (this.reconnectTimeout) clearTimeout(this.reconnectTimeout);
		this.reconnectTimeout = null;
		this.reconnectDelay = 1000;
	}

	onEvent(handler: EventHandler): () => void {
		this.handlers.add(handler);
		return () => this.handlers.delete(handler);
	}

	onStatus(handler: StatusHandler): () => void {
		this.statusHandlers.add(handler);
		return () => this.statusHandlers.delete(handler);
	}

	private scheduleReconnect() {
		if (this.reconnectTimeout) clearTimeout(this.reconnectTimeout);
		this.reconnectTimeout = setTimeout(() => {
			this.reconnectTimeout = null;
			this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxReconnectDelay);
			this.connect(this.wid);
		}, this.reconnectDelay);
	}
}

let sseInstance: SSEClient | null = null;

export function getSSIClient(): SSEClient {
	if (!sseInstance) sseInstance = new SSEClient();
	return sseInstance;
}

export function connectSSE(weddingId: string): SSEClient {
	const client = getSSIClient();
	client.connect(weddingId);
	return client;
}

export function disconnectSSE() {
	sseInstance?.disconnect();
	sseInstance = null;
}
