import { goto } from '$app/navigation';
import { getAuth, setAuth, clearAuth } from '#lib/stores/index.ts';
import { setWeddingId } from '#lib/stores/weddingId.ts';
import { clearGuestStores, clearGuestCaches } from '#lib/stores/guestEvents.ts';

const BASE = '';

// Serialize concurrent refresh attempts — only one refresh in flight at a time.
let refreshPromise: Promise<boolean> | null = null;

// A stalled connection (wifi up, no upstream) never rejects on its own; cap it so
// the edit falls back to the offline queue instead of spinning for minutes.
const REQUEST_TIMEOUT_MS = 10_000;

// Chrome/Firefox say "Failed to fetch"/"NetworkError"; Safari says "Load failed".
// A failed request is a fallback-to-cache case; a 4xx is not.
export function isNetworkError(e: unknown): boolean {
	if (typeof navigator !== 'undefined' && navigator.onLine === false) return true;
	const name = (e as { name?: string } | null)?.name;
	if (name === 'TimeoutError' || name === 'AbortError') return true;
	return e instanceof TypeError && /failed to fetch|networkerror|load failed/i.test((e as Error).message || '');
}

async function doRefresh(): Promise<boolean> {
	const { refreshToken } = getAuth();
	if (!refreshToken) return false;
	// A network failure rejects: the token was never seen by the server, so the
	// session stays, and apiFetch callers can queue the edit.
	const refreshRes = await fetch(`${BASE}/api/auth/refresh`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ refreshToken }),
		signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS)
	});
	if (refreshRes.ok) {
		const data = await refreshRes.json();
		setAuth(data.accessToken, data.refreshToken, data.role ?? getAuth().role ?? '', data.name ?? getAuth().name ?? '');
		if (data.forcePasswordChange) {
			// goto, not location.href: a reload would wipe the in-memory tokens.
			void goto('/change-password?forced=1');
			return false;
		}
		return true;
	}
	// Only 401 means the refresh token is dead. 503 and other errors keep the session for a retry.
	if (refreshRes.status !== 401) return false;
	clearAuth();
	setWeddingId('');
	// A revoked session must not leave the guest cache (emails, notes, angbao)
	// on the device for the next account.
	clearGuestStores();
	clearGuestCaches();
	if (typeof window !== 'undefined') window.location.href = '/login';
	return false;
}

/** Shared refresh. Rejects with the network error when the server is unreachable. */
function refreshShared(): Promise<boolean> {
	if (!refreshPromise) {
		refreshPromise = doRefresh().finally(() => { refreshPromise = null; });
	}
	return refreshPromise;
}

/** The one refresh path. True when a new access token is in place. */
export async function refreshIfNeeded(): Promise<boolean> {
	return refreshShared().catch((e) => {
		if (isNetworkError(e)) return false;
		throw e;
	});
}

export async function apiFetch(path: string, opts: RequestInit = {}): Promise<Response> {
	const { accessToken } = getAuth();
	const isDelete = opts.method === 'DELETE';
	let res = await fetch(`${BASE}${path}`, {
		...opts,
		signal: opts.signal ?? AbortSignal.timeout(REQUEST_TIMEOUT_MS),
		headers: {
			...(!isDelete ? { 'Content-Type': 'application/json' } : {}),
			...opts.headers,
			...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {})
		}
	});

	if (res.status === 401) {
		// Unreachable refresh throws the network error, so callers queue the edit.
		const refreshed = await refreshShared();
		if (refreshed) {
			const { accessToken: newToken } = getAuth();
			res = await fetch(`${BASE}${path}`, {
				...opts,
				signal: opts.signal ?? AbortSignal.timeout(REQUEST_TIMEOUT_MS),
				headers: {
					'Content-Type': 'application/json',
					...opts.headers,
					Authorization: `Bearer ${newToken}`
				}
			});
		}
	}
	return res;
}

export async function uploadFile(file: File): Promise<{ url: string; filename: string }> {
	const { accessToken } = getAuth();
	const formData = new FormData();
	formData.append('file', file);
	let res = await fetch('/api/upload', {
		method: 'POST',
		headers: {
			Authorization: `Bearer ${accessToken}`
		},
		body: formData
	});

	if (res.status === 401) {
		const refreshed = await refreshIfNeeded();
		if (refreshed) {
			const { accessToken: newToken } = getAuth();
			res = await fetch('/api/upload', {
				method: 'POST',
				headers: {
					Authorization: `Bearer ${newToken}`
				},
				body: formData
			});
		}
	}

	if (!res.ok) {
		const err = await res.json().catch(() => ({ title: 'Upload failed' }));
		throw new Error(err.title || 'Upload failed');
	}
	return res.json();
}
