import { goto } from '$app/navigation';
import { getAuth, setAuth, clearAuth } from '#lib/stores/index.ts';
import { setWeddingId } from '#lib/stores/weddingId.ts';
import { clearOfflineData } from '#lib/offline/queue.ts';

const BASE = '';

// refreshed: new tokens stored. forced: sent to change-password. ended: the
// session is over. unavailable: network or server fault, session kept.
export type RefreshResult = 'refreshed' | 'forced' | 'ended' | 'unavailable';

// Serialize concurrent refresh attempts — only one refresh in flight at a time.
let refreshPromise: Promise<RefreshResult> | null = null;

export function getAccessToken(): string {
	const { accessToken } = getAuth();
	return accessToken ?? '';
}

async function doRefresh(): Promise<RefreshResult> {
	const { refreshToken } = getAuth();
	if (!refreshToken) return 'ended';
	let refreshRes: Response;
	try {
		refreshRes = await fetch(`${BASE}/api/auth/refresh`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ refreshToken })
		});
	} catch {
		// Network failure — the token was never seen by the server, so keep the session.
		return 'unavailable';
	}
	if (refreshRes.ok) {
		// An unreadable body is a server fault, not a dead token: keep the session.
		const data = await refreshRes.json().catch(() => null);
		if (!data) return 'unavailable';
		setAuth(data.accessToken, data.refreshToken, data.role ?? getAuth().role ?? '', data.name ?? getAuth().name ?? '');
		if (data.forcePasswordChange) {
			// Client-side navigation: a full page load would drop the in-memory tokens.
			if (typeof window !== 'undefined') goto('/change-password?forced=1', { replaceState: true });
			return 'forced';
		}
		return 'refreshed';
	}
	// Only a definite rejection means the refresh token is dead. Keep the session on 5xx.
	if (refreshRes.status !== 400 && refreshRes.status !== 401) return 'unavailable';
	// The session is over: drop this account's queued edits and cached guest
	// data so the next login on this device cannot replay or read them.
	clearOfflineData();
	clearAuth();
	setWeddingId('');
	if (typeof window !== 'undefined') window.location.href = '/login';
	return 'ended';
}

/** The one refresh path: apiFetch, validateToken and SSE all share it. */
export function refreshSession(): Promise<RefreshResult> {
	if (!refreshPromise) {
		refreshPromise = doRefresh().finally(() => { refreshPromise = null; });
	}
	return refreshPromise;
}

async function refreshIfNeeded(): Promise<boolean> {
	return (await refreshSession()) === 'refreshed';
}

export async function apiFetch(path: string, opts: RequestInit = {}): Promise<Response> {
	const isDelete = opts.method === 'DELETE';
	// The retry after a refresh sends the same headers as the first attempt.
	const send = () => {
		const { accessToken } = getAuth();
		return fetch(`${BASE}${path}`, {
			...opts,
			headers: {
				...(!isDelete ? { 'Content-Type': 'application/json' } : {}),
				...opts.headers,
				...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {})
			}
		});
	};
	let res = await send();

	if (res.status === 401 && await refreshIfNeeded()) res = await send();
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
