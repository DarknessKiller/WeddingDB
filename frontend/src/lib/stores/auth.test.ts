import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

function memoryStorage() {
	const map = new Map<string, string>();
	return {
		getItem: (k: string) => map.get(k) ?? null,
		setItem: (k: string, v: string) => void map.set(k, String(v)),
		removeItem: (k: string) => void map.delete(k),
		clear: () => map.clear()
	};
}

let storage: ReturnType<typeof memoryStorage>;
let listeners: Record<string, ((e: StorageEvent) => void)[]>;

beforeEach(() => {
	vi.resetModules();
	storage = memoryStorage();
	storage.setItem('weddingdb_access_token', 'old-access');
	// Legacy key from an older build; the store must stop reading it.
	storage.setItem('weddingdb_refresh_token', 'refresh-1');
	Object.defineProperty(globalThis, 'localStorage', { value: storage, configurable: true });
	listeners = {};
	Object.defineProperty(globalThis, 'window', {
		value: {
			addEventListener: (type: string, handler: (e: StorageEvent) => void) => {
				(listeners[type] ??= []).push(handler);
			}
		},
		configurable: true
	});
});

afterEach(() => {
	delete (globalThis as any).window;
	delete (globalThis as any).localStorage;
});

describe('auth tab sync', () => {
	it('adopts tokens another tab rotated', async () => {
		const { getAuth } = await import('$lib/stores');
		expect(getAuth().accessToken).toBe('old-access');

		storage.setItem('weddingdb_access_token', 'new-access');
		listeners.storage[0]({ key: 'weddingdb_access_token' } as StorageEvent);

		expect(getAuth()).toMatchObject({ accessToken: 'new-access' });
	});

	it('mirrors another tab logging out', async () => {
		const { getAuth } = await import('$lib/stores');
		expect(getAuth().accessToken).toBe('old-access');

		storage.clear();
		listeners.storage[0]({ key: null } as StorageEvent);

		expect(getAuth()).toMatchObject({ accessToken: null });
	});
});
