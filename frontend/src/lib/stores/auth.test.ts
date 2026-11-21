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
	// Legacy per-token keys must be ignored; the session lives under one JSON key.
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
	it('restores a persisted session after a reload', async () => {
		storage.setItem(
			'weddingdb_auth',
			JSON.stringify({ accessToken: 'saved', refreshToken: 'saved-refresh', role: 'staff', name: 'N' })
		);

		const { getAuth } = await import('#lib/stores/index.ts');

		expect(getAuth()).toEqual({
			accessToken: 'saved',
			refreshToken: 'saved-refresh',
			role: 'staff',
			name: 'N'
		});
	});

	it('ignores malformed persisted sessions', async () => {
		storage.setItem('weddingdb_auth', '{not json');

		const { getAuth } = await import('#lib/stores/index.ts');

		expect(getAuth()).toMatchObject({ accessToken: null, refreshToken: null });
	});

	it('persists on setAuth and removes the session on clearAuth', async () => {
		const { getAuth, setAuth, clearAuth } = await import('#lib/stores/index.ts');

		setAuth('new-access', 'refresh-2', 'admin', 'A', 'a@x.com');
		expect(JSON.parse(storage.getItem('weddingdb_auth')!)).toEqual({
			accessToken: 'new-access',
			refreshToken: 'refresh-2',
			role: 'admin',
			name: 'A',
			user: 'a@x.com'
		});

		clearAuth();
		expect(storage.getItem('weddingdb_auth')).toBeNull();
		expect(getAuth()).toMatchObject({ accessToken: null, refreshToken: null, role: null, name: null });
	});

	it('ends this tab session when another tab logs out', async () => {
		const { getAuth, setAuth } = await import('#lib/stores/index.ts');
		const w = globalThis.window as any;
		w.location = { pathname: '/w1/guests', href: '/w1/guests' };
		setAuth('a', 'refresh-2', 'staff', 'S');

		storage.removeItem('weddingdb_auth');
		for (const h of listeners.storage) h({ key: 'weddingdb_auth', newValue: null } as StorageEvent);

		expect(getAuth()).toMatchObject({ accessToken: null, refreshToken: null });
		expect(w.location.href).toBe('/login');
	});

	it('keeps the session when another tab only rotates the token', async () => {
		const { getAuth, setAuth } = await import('#lib/stores/index.ts');
		const w = globalThis.window as any;
		w.location = { pathname: '/w1/guests', href: '/w1/guests' };
		setAuth('a', 'refresh-2', 'staff', 'S', 's@x.com');

		storage.setItem('weddingdb_auth', JSON.stringify({ accessToken: 'b', refreshToken: 'refresh-3', role: 'staff', name: 'S', user: 's@x.com' }));
		for (const h of listeners.storage) h({ key: 'weddingdb_auth', newValue: 'x' } as StorageEvent);

		expect(getAuth().refreshToken).toBe('refresh-2');
		expect(w.location.href).toBe('/w1/guests');
	});

	it('ends this tab session when another user logs in, leaving their stored auth', async () => {
		const { getAuth, setAuth } = await import('#lib/stores/index.ts');
		const w = globalThis.window as any;
		w.location = { pathname: '/w1/guests', href: '/w1/guests' };
		setAuth('a', 'refresh-2', 'staff', 'S', 's@x.com');

		const other = JSON.stringify({ accessToken: 'b', refreshToken: 'refresh-b', role: 'staff', name: 'B', user: 'b@x.com' });
		storage.setItem('weddingdb_auth', other);
		for (const h of listeners.storage) h({ key: 'weddingdb_auth', newValue: other } as StorageEvent);

		expect(getAuth()).toMatchObject({ accessToken: null, refreshToken: null });
		expect(w.location.href).toBe('/login');
		expect(storage.getItem('weddingdb_auth')).toBe(other);
	});
});
