import { getAuth } from '$lib/stores';

export interface GuestEvent {
	type: 'create' | 'update' | 'delete' | 'checkin' | 'checkout' | 'seat_assign';
	guestId: string;
	weddingId: string;
	guest?: {
		id: string;
		name: string;
		phone: string;
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
type StatusHandler = (status: 'connected' | 'reconnecting', reconnect: boolean) => void;

class SSEClient {
	private eventSource: EventSource | null = null;
	private handlers: Set<EventHandler> = new Set();
	private statusHandlers: Set<StatusHandler> = new Set();
	private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
	private reconnectDelay = 1000;
	private maxReconnectDelay = 30000;
	private wid = '';

	connect(weddingId: string) {
		if (this.eventSource) this.disconnect();
		this.wid = weddingId;
		const { accessToken } = getAuth();
		if (!accessToken) {
			console.warn('SSE: No access token available');
			return;
		}
		const url = `/api/weddings/${weddingId}/events?token=${encodeURIComponent(accessToken)}`;
		this.eventSource = new EventSource(url);
		this.eventSource.onopen = () => {
			const reconnect = this.reconnectDelay > 1000;
			this.reconnectDelay = 1000;
			this.statusHandlers.forEach((handler) => handler('connected', reconnect));
		};
		this.eventSource.onmessage = (event) => {
			try {
				const data: GuestEvent = JSON.parse(event.data);
				this.handlers.forEach((handler) => handler(data));
			} catch (e) {
				console.error('SSE: Failed to parse event', e);
			}
		};
		this.eventSource.onerror = () => {
			this.statusHandlers.forEach((handler) => handler('reconnecting', false));
			this.scheduleReconnect();
		};
	}

	disconnect() {
		this.eventSource?.close();
		this.eventSource = null;
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
