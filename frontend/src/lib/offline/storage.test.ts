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
let events: string[];

beforeEach(() => {
	vi.resetModules();
	storage = memoryStorage();
	events = [];
	Object.defineProperty(globalThis, 'localStorage', { value: storage, configurable: true });
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
		storage.setItem('theme', 'dark');

		clearAuth();

		expect(storage.getItem('offline_cache_w1')).toBeNull();
		expect(storage.getItem('offline_cache_w2')).toBeNull();
		expect(storage.getItem('offline_queue_w1')).toBeNull();
		expect(JSON.parse(storage.getItem('kiosk_cache_w1')!)).toEqual([kioskGuest]);
		expect(storage.getItem('theme')).toBe('dark');
		expect(events).toContain('offline-queue-changed');
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
