import { getAuth } from '#lib/stores/index.ts';
import { refreshIfNeeded } from '#lib/api/client.ts';
import type { GuestRow } from '#lib/utils/guestTransform.ts';

export interface GuestEvent {
	type: 'create' | 'update' | 'delete' | 'checkin' | 'checkout' | 'seat_assign';
	guestId: string;
	weddingId: string;
	guest?: GuestRow;
	timestamp: number;
}

type EventHandler = (event: GuestEvent) => void;
type StatusHandler = (status: 'connected' | 'reconnecting', reconnect: boolean) => void;

// The server pings every 20s. Silence past this means a half-open stream
// (roamed access point, no FIN), so drop it and reconnect.
const STALL_MS = 45_000;

class SSEClient {
	private controller: AbortController | null = null;
	private handlers: Set<EventHandler> = new Set();
	private statusHandlers: Set<StatusHandler> = new Set();
	private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
	private reconnectDelay = 1000;
	private maxReconnectDelay = 30000;
	// A 401 refresh leaves a gap with no stream. Keep it flagged so the next
	// successful connect still reports `reconnect` and the caller runs resync.
	private tokenGap = false;
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
		let stalled = false;
		let watchdog: ReturnType<typeof setTimeout> | undefined;
		const arm = () => {
			clearTimeout(watchdog);
			watchdog = setTimeout(() => { stalled = true; controller.abort(); }, STALL_MS);
		};
		try {
			const response = await fetch(`/api/weddings/${weddingId}/events`, {
				headers: { Authorization: `Bearer ${accessToken}` },
				signal: controller.signal
			});
			// An expired token would fail every retry: refresh it and reconnect
			// now, not after the backoff the 401 left behind.
			if (response.status === 401) {
				const refreshed = await refreshIfNeeded();
				if (refreshed) {
					this.tokenGap = true;
					this.reconnectDelay = 1000;
					this.connect(this.wid);
					return;
				}
			}
			if (!response.ok || !response.body) throw new Error(`SSE request failed: ${response.status}`);

			const reconnect = this.tokenGap || this.reconnectDelay > 1000;
			this.tokenGap = false;
			this.reconnectDelay = 1000;
			this.statusHandlers.forEach((handler) => handler('connected', reconnect));
			const reader = response.body.getReader();
			const decoder = new TextDecoder();
			let buffer = '';
			while (true) {
				arm();
				const { value, done } = await reader.read();
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
			if (controller.signal.aborted && !stalled) return;
			this.statusHandlers.forEach((handler) => handler('reconnecting', false));
		} finally {
			clearTimeout(watchdog);
			if (this.controller === controller) {
				this.controller = null;
				this.scheduleReconnect();
			}
		}
	}

	disconnect() {
		this.controller?.abort();
		this.controller = null;
		if (this.reconnectTimeout) clearTimeout(this.reconnectTimeout);
		this.reconnectTimeout = null;
		this.reconnectDelay = 1000;
		this.tokenGap = false;
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
