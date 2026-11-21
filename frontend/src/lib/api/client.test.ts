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

function res(body: unknown, status = 200): Response {
	return { ok: status >= 200 && status < 300, status, json: async () => body } as Response;
}

const REFRESH_OK = { accessToken: 'new-access', refreshToken: 'refresh-2', role: 'admin', name: 'A' };

let fetchMock: ReturnType<typeof vi.fn>;
let location: { href: string };

beforeEach(async () => {
	vi.resetModules();
	Object.defineProperty(globalThis, 'localStorage', { value: memoryStorage(), configurable: true });
	location = { href: '' };
	Object.defineProperty(globalThis, 'window', { value: { location, addEventListener: () => {} }, configurable: true });
	fetchMock = vi.fn();
	Object.defineProperty(globalThis, 'fetch', { value: fetchMock, configurable: true });
	const { setAuth } = await import('#lib/stores/index.ts');
	setAuth('old-access', 'refresh-1', 'admin', 'A', 'a@x.com');
});

afterEach(() => {
	vi.restoreAllMocks();
	delete (globalThis as any).window;
	delete (globalThis as any).localStorage;
});

describe('apiFetch refresh recovery', () => {
	it('retries the request with the rotated token after a successful refresh', async () => {
		fetchMock
			.mockResolvedValueOnce(res({}, 401))
			.mockResolvedValueOnce(res(REFRESH_OK))
			.mockResolvedValueOnce(res({ data: [] }));

		const { apiFetch } = await import('./client');
		const result = await apiFetch('/api/guests');

		expect(result.status).toBe(200);
		expect(fetchMock).toHaveBeenCalledTimes(3);
		expect(fetchMock.mock.calls[1][0]).toContain('/api/auth/refresh');
		expect((fetchMock.mock.calls[2][1].headers as Record<string, string>).Authorization).toBe('Bearer new-access');
		expect(location.href).toBe('');
	});

	it('keeps the session when the refresh request cannot reach the server', async () => {
		fetchMock.mockResolvedValueOnce(res({}, 401)).mockRejectedValueOnce(new TypeError('Failed to fetch'));

		const { apiFetch } = await import('./client');
		const { getAuth } = await import('#lib/stores/index.ts');
		const result = await apiFetch('/api/guests');

		expect(result.status).toBe(401);
		expect(location.href).toBe('');
		expect(getAuth()).toMatchObject({ accessToken: 'old-access', refreshToken: 'refresh-1' });
		expect(JSON.parse(localStorage.getItem('weddingdb_auth')!).refreshToken).toBe('refresh-1');
	});

	it('keeps the session when refresh fails with a server error', async () => {
		fetchMock.mockResolvedValueOnce(res({}, 401)).mockResolvedValueOnce(res({ title: 'boom' }, 502));

		const { apiFetch } = await import('./client');
		const { getAuth } = await import('#lib/stores/index.ts');
		await apiFetch('/api/guests');

		expect(location.href).toBe('');
		expect(getAuth()).toMatchObject({ accessToken: 'old-access', refreshToken: 'refresh-1' });
	});

	it('clears the session when the server rejects the refresh token', async () => {
		fetchMock
			.mockResolvedValueOnce(res({}, 401))
			.mockResolvedValueOnce(res({ title: 'invalid refresh token' }, 401));

		const { apiFetch } = await import('./client');
		const { getAuth } = await import('#lib/stores/index.ts');
		const result = await apiFetch('/api/guests');

		expect(result.status).toBe(401);
		expect(location.href).toBe('/login');
		expect(getAuth().accessToken).toBeNull();
		expect(localStorage.getItem('weddingdb_auth')).toBeNull();
	});

	it('ends the session without refreshing when another tab logged out', async () => {
		localStorage.removeItem('weddingdb_auth');
		fetchMock.mockResolvedValueOnce(res({}, 401));

		const { apiFetch } = await import('./client');
		const { getAuth } = await import('#lib/stores/index.ts');
		await apiFetch('/api/guests');

		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(location.href).toBe('/login');
		expect(getAuth().refreshToken).toBeNull();
	});

	it('adopts a token another tab already rotated instead of reusing the stale one', async () => {
		localStorage.setItem('weddingdb_auth', JSON.stringify({ accessToken: 'tab2-access', refreshToken: 'refresh-2', role: 'admin', name: 'A', user: 'a@x.com' }));
		fetchMock
			.mockResolvedValueOnce(res({}, 401))
			.mockResolvedValueOnce(res({ guests: [] }));

		const { apiFetch } = await import('./client');
		const { getAuth } = await import('#lib/stores/index.ts');
		const result = await apiFetch('/api/guests');

		expect(result.status).toBe(200);
		expect(fetchMock).toHaveBeenCalledTimes(2);
		expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/api/auth/refresh'))).toBe(false);
		expect(fetchMock.mock.calls[1][1].headers.Authorization).toBe('Bearer tab2-access');
		expect(getAuth().refreshToken).toBe('refresh-2');
	});

	it('never adopts tokens of another user who logged in from another tab', async () => {
		const other = JSON.stringify({ accessToken: 'b-access', refreshToken: 'refresh-b', role: 'staff', name: 'B', user: 'b@x.com' });
		localStorage.setItem('weddingdb_auth', other);
		fetchMock.mockResolvedValueOnce(res({}, 401));

		const { apiFetch } = await import('./client');
		const { getAuth } = await import('#lib/stores/index.ts');
		const result = await apiFetch('/api/guests');

		expect(result.status).toBe(401);
		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(getAuth()).toMatchObject({ accessToken: null, refreshToken: null });
		expect(location.href).toBe('/login');
		expect(localStorage.getItem('weddingdb_auth')).toBe(other);
	});

	it('re-picks the URL wedding when another tab rescoped the shared token', async () => {
		const { setAuth, getAuth } = await import('#lib/stores/index.ts');
		setAuth('scoped-b', 'refresh-1', 'staff', 'S');
		fetchMock
			.mockResolvedValueOnce(res({ error: 'Access denied' }, 403))
			.mockResolvedValueOnce(res({ accessToken: 'scoped-a' }))
			.mockResolvedValueOnce(res({ guests: [] }));

		const { apiFetch } = await import('./client');
		const result = await apiFetch('/api/weddings/wA/guests');

		expect(result.status).toBe(200);
		expect(fetchMock.mock.calls[1][0]).toContain('/api/auth/select-wedding');
		expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ weddingId: 'wA', refreshToken: 'refresh-1' });
		expect(fetchMock.mock.calls[2][1].headers.Authorization).toBe('Bearer scoped-a');
		expect(getAuth().accessToken).toBe('scoped-a');
	});

	it('returns the 403 when the user really has no access', async () => {
		const { setAuth } = await import('#lib/stores/index.ts');
		setAuth('scoped-b', 'refresh-1', 'staff', 'S');
		fetchMock.mockResolvedValueOnce(res({}, 403)).mockResolvedValueOnce(res({ title: 'No access' }, 403));

		const { apiFetch } = await import('./client');
		const result = await apiFetch('/api/weddings/wA/guests');

		expect(result.status).toBe(403);
		expect(fetchMock).toHaveBeenCalledTimes(2);
	});
});
