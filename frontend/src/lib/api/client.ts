import { goto } from '$app/navigation';
import { getAuth, setAuth, clearAuth } from '#lib/stores/index.ts';
import { setWeddingId } from '#lib/stores/weddingId.ts';

const BASE = '';

// Serialize concurrent refresh attempts — only one refresh in flight at a time.
let refreshPromise: Promise<boolean> | null = null;

// Chrome/Firefox say "Failed to fetch"/"NetworkError"; Safari says "Load failed".
// A failed request is a fallback-to-cache case; a 4xx is not.
export function isNetworkError(e: unknown): boolean {
	if (typeof navigator !== 'undefined' && navigator.onLine === false) return true;
	return e instanceof TypeError && /failed to fetch|networkerror|load failed/i.test((e as Error).message || '');
}

async function doRefresh(): Promise<boolean> {
	const { refreshToken } = getAuth();
	if (!refreshToken) return false;
	let refreshRes: Response;
	try {
		refreshRes = await fetch(`${BASE}/api/auth/refresh`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ refreshToken })
		});
	} catch {
		// Network failure — the token was never seen by the server, so keep the session.
		return false;
	}
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
	if (typeof window !== 'undefined') window.location.href = '/login';
	return false;
}

/** The one refresh path. True when a new access token is in place. */
export async function refreshIfNeeded(): Promise<boolean> {
	if (!refreshPromise) {
		refreshPromise = doRefresh().finally(() => { refreshPromise = null; });
	}
	return refreshPromise;
}

export async function apiFetch(path: string, opts: RequestInit = {}): Promise<Response> {
	const { accessToken } = getAuth();
	const isDelete = opts.method === 'DELETE';
	let res = await fetch(`${BASE}${path}`, {
		...opts,
		headers: {
			...(!isDelete ? { 'Content-Type': 'application/json' } : {}),
			...opts.headers,
			...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {})
		}
	});

	if (res.status === 401) {
		const refreshed = await refreshIfNeeded();
		if (refreshed) {
			const { accessToken: newToken } = getAuth();
			res = await fetch(`${BASE}${path}`, {
				...opts,
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
