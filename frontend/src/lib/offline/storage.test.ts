import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// localStorage with key()/length, which clearStaffOfflineData iterates.
function memoryStorage() {
	const map = new Map<string, string>();
	return {
		get length() { return map.size; },
		key: (i: number) => [...map.keys()][i] ?? null,
		getItem: (k: string) => map.get(k) ?? null,
		setItem: (k: string, v: string) => void map.set(k, String(v)),
		removeItem: (k: string) => void map.delete(k),
		clear: () => map.clear()
	};
}

let storage: ReturnType<typeof memoryStorage>;
let session: ReturnType<typeof memoryStorage>;
let events: string[];

beforeEach(() => {
	vi.resetModules();
	storage = memoryStorage();
	session = memoryStorage();
	events = [];
	Object.defineProperty(globalThis, 'localStorage', { value: storage, configurable: true });
	Object.defineProperty(globalThis, 'sessionStorage', { value: session, configurable: true });
	Object.defineProperty(globalThis, 'window', {
		value: {
			addEventListener: () => {},
			dispatchEvent: (e: Event) => { events.push(e.type); return true; }
		},
		configurable: true
	});
});

afterEach(() => {
	delete (globalThis as any).window;
	delete (globalThis as any).localStorage;
	delete (globalThis as any).sessionStorage;
});

const staffGuest = { id: 'g1', name: 'Alice', email: 'a@x.com', angbaoAmount: 200, checkedIn: true };
const kioskGuest = { id: 'g1', name: 'Alice' };

describe('offline data on logout', () => {
	it('clearAuth removes staff caches and queued edits for every wedding, keeps kiosk cache', async () => {
		const { cacheGuests, cacheKioskGuests, enqueue } = await import('#lib/offline/queue.ts');
		const { clearAuth } = await import('#lib/stores/index.ts');

		cacheGuests('w1', [staffGuest]);
		cacheGuests('w2', [staffGuest]);
		cacheKioskGuests('w1', [kioskGuest]);
		enqueue('w1', { method: 'DELETE', path: '/api/weddings/w1/guests/g1' } as any);
		storage.setItem('offline_cache_w3', '[]'); // left by an older build
		storage.setItem('theme', 'dark');

		clearAuth();

		expect(session.getItem('offline_cache_w1')).toBeNull();
		expect(session.getItem('offline_cache_w2')).toBeNull();
		expect(storage.getItem('offline_cache_w3')).toBeNull();
		expect(storage.getItem('offline_queue_w1')).toBeNull();
		expect(JSON.parse(storage.getItem('kiosk_cache_w1')!)).toEqual([kioskGuest]);
		expect(storage.getItem('theme')).toBe('dark');
		expect(events).toContain('offline-queue-changed');
	});
});

describe('staff data across reloads and logins', () => {
	const edit = { mutationId: 'm1', op: 'checkin', guestId: 'g1', clientUpdatedAt: '' } as const;

	it('keeps the staff roster out of localStorage so a closed tab leaves none behind', async () => {
		const { cacheGuests } = await import('#lib/offline/queue.ts');
		cacheGuests('w1', [staffGuest]);
		expect(storage.getItem('offline_cache_w1')).toBeNull();
		expect(JSON.parse(session.getItem('offline_cache_w1')!)).toEqual([staffGuest]);
	});

	it('same user logging back in keeps unsynced edits', async () => {
		const { enqueue, getQueue } = await import('#lib/offline/queue.ts');
		const { claimStaffOfflineData } = await import('#lib/offline/storage.ts');
		claimStaffOfflineData('Staff@x.test');
		enqueue('w1', edit);

		claimStaffOfflineData(' staff@x.test ');

		expect(getQueue('w1')).toEqual([edit]);
	});

	it('a different user logging in drops the previous user edits', async () => {
		const { enqueue, getQueue } = await import('#lib/offline/queue.ts');
		const { claimStaffOfflineData } = await import('#lib/offline/storage.ts');
		claimStaffOfflineData('staff@x.test');
		enqueue('w1', edit);

		claimStaffOfflineData('other@x.test');

		expect(getQueue('w1')).toEqual([]);
	});

	it('session expiry keeps unsynced edits but drops the cached roster', async () => {
		const { cacheGuests, enqueue, getQueue } = await import('#lib/offline/queue.ts');
		const { claimStaffOfflineData } = await import('#lib/offline/storage.ts');
		const { expireAuth } = await import('#lib/stores/index.ts');
		claimStaffOfflineData('staff@x.test');
		cacheGuests('w1', [staffGuest]);
		enqueue('w1', edit);

		expireAuth();

		expect(session.getItem('offline_cache_w1')).toBeNull();
		expect(storage.getItem('weddingdb_auth')).toBeNull();
		expect(getQueue('w1')).toEqual([edit]);

		// The same user logging back in keeps the queue for sync.
		claimStaffOfflineData('staff@x.test');
		expect(getQueue('w1')).toEqual([edit]);
	});
});

describe('kiosk and staff caches', () => {
	it('stay separate: kiosk writes never reach the staff cache and vice versa', async () => {
		const { cacheGuests, loadCachedGuests, cacheKioskGuests, loadCachedKioskGuests } =
			await import('#lib/offline/queue.ts');

		cacheGuests('w1', [staffGuest]);
		expect(loadCachedKioskGuests('w1')).toBeNull();

		cacheKioskGuests('w1', [kioskGuest]);
		expect(loadCachedGuests('w1')).toEqual([staffGuest]);
		expect(loadCachedKioskGuests('w1')).toEqual([kioskGuest]);
	});
});

describe('flushAllQueues', () => {
	it('syncs every wedding with queued edits and counts what is left', async () => {
		const fetchMock = vi.fn(async (url: string) => {
			if (url.includes('/w1/')) return new Response(JSON.stringify({ results: [{ guestId: 'g1', status: 'applied' }] }), { status: 200 });
			throw new TypeError('Failed to fetch');
		});
		Object.defineProperty(globalThis, 'fetch', { value: fetchMock, configurable: true });
		const { enqueue, flushAllQueues, queuedCount } = await import('#lib/offline/queue.ts');
		const m = { op: 'checkin' as const, guestId: 'g1', clientUpdatedAt: '2026-01-01T10:00:00Z' };
		enqueue('w1', { ...m, mutationId: 'a' });
		enqueue('w2', { ...m, mutationId: 'b' });
		enqueue('w2', { ...m, mutationId: 'c' });

		expect(await flushAllQueues()).toBe(2);
		expect(queuedCount('w1')).toBe(0);
		expect(fetchMock.mock.calls.map(([u]) => u)).toEqual(['/api/weddings/w1/guests/sync', '/api/weddings/w2/guests/sync']);
	});
});
