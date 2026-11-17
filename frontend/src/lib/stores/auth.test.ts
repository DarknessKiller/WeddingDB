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

describe('auth storage', () => {
	it('does not load persisted tokens or write tokens to localStorage', async () => {
		const { getAuth, setAuth } = await import('#lib/stores/index.ts');
		expect(getAuth()).toMatchObject({ accessToken: null, refreshToken: null });

		setAuth('new-access', 'refresh-2', 'admin', 'A');

		expect(getAuth()).toMatchObject({ accessToken: 'new-access', refreshToken: 'refresh-2' });
		expect(storage.getItem('weddingdb_access_token')).toBeNull();
		expect(storage.getItem('weddingdb_refresh_token')).toBeNull();
	});
});
