import { getAuth, setAuth, expireAuth, readStoredAuth, isSameUser, dropForeignSession } from '#lib/stores/index.ts';
import { setWeddingId } from '#lib/stores/weddingId.ts';
import { withTimeout } from '#lib/utils/index.ts';

const BASE = '';

// Serialize concurrent refresh attempts — only one refresh in flight at a time.
let refreshPromise: Promise<boolean> | null = null;

async function doRefresh(): Promise<boolean> {
	const { refreshToken } = getAuth();
	if (!refreshToken) return false;
	// Another tab already rotated the token. Reusing our stale one would make the
	// server revoke the whole token family and log out every tab.
	const stored = readStoredAuth();
	// Another tab logged out or expired the session: end ours, never refresh it.
	if (!stored) {
		expireAuth();
		setWeddingId('');
		if (typeof window !== 'undefined') window.location.href = '/login';
		return false;
	}
	if (stored.refreshToken !== refreshToken) {
		// Another user logged in from another tab: never become them.
		if (!isSameUser(stored)) {
			dropForeignSession();
			if (typeof window !== 'undefined') window.location.href = '/login';
			return false;
		}
		setAuth(stored.accessToken, stored.refreshToken, stored.role, stored.name);
		return true;
	}
	let refreshRes: Response;
	try {
		refreshRes = await fetch(`${BASE}/api/auth/refresh`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ refreshToken }),
			signal: withTimeout()
		});
	} catch {
		// Network failure — the token was never seen by the server, so keep the session.
		return false;
	}
	if (refreshRes.ok) {
		const data = await refreshRes.json();
		if (data.forcePasswordChange) {
			setAuth(data.accessToken, data.refreshToken, data.role ?? getAuth().role ?? '', data.name ?? getAuth().name ?? '');
			if (typeof window !== 'undefined') window.location.href = '/change-password?forced=1';
			return false;
		}
		setAuth(data.accessToken, data.refreshToken, data.role ?? getAuth().role ?? '', data.name ?? getAuth().name ?? '');
		return true;
	}
	// Only a definite rejection means the refresh token is dead. Keep the session on 5xx.
	if (refreshRes.status !== 400 && refreshRes.status !== 401) return false;
	// Session expiry, not logout: the queue must survive so offline edits made
	// before the token died still sync after the user logs back in.
	expireAuth();
	setWeddingId('');
	if (typeof window !== 'undefined') window.location.href = '/login';
	return false;
}

async function refreshIfNeeded(): Promise<boolean> {
	if (!refreshPromise) {
		refreshPromise = doRefresh().finally(() => { refreshPromise = null; });
	}
	return refreshPromise;
}

// Shared with the SSE client, which cannot use apiFetch for a streamed response.
export async function refreshAccessToken(): Promise<boolean> {
	return refreshIfNeeded();
}

// Tabs share one refresh token. When another tab picks a different wedding,
// our next refresh comes back scoped to that one and every call here gets 403.
// Re-pick the wedding in the URL once; admins are not wedding-scoped.
// Shared with the SSE client, whose stream gets the same 403.
export async function reselectWedding(wid: string): Promise<boolean> {
	const { accessToken, refreshToken, role, name } = getAuth();
	if (!refreshToken || role === 'admin') return false;
	try {
		const res = await fetch(`${BASE}/api/auth/select-wedding`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}) },
			body: JSON.stringify({ weddingId: wid, refreshToken }),
			signal: withTimeout()
		});
		if (!res.ok) return false;
		const data = await res.json();
		setAuth(data.accessToken, refreshToken, role ?? '', name ?? '');
		return true;
	} catch {
		return false;
	}
}

export async function apiFetch(path: string, opts: RequestInit = {}): Promise<Response> {
	const isDelete = opts.method === 'DELETE';
	const send = () => {
		const { accessToken } = getAuth();
		return fetch(`${BASE}${path}`, {
			...opts,
			// A caller passing its own signal owns the deadline (long imports, exports, sync).
			signal: opts.signal ?? withTimeout(),
			headers: {
				...(!isDelete ? { 'Content-Type': 'application/json' } : {}),
				...opts.headers,
				...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {})
			}
		});
	};
	let res = await send();
	if (res.status === 401 && (await refreshIfNeeded())) res = await send();
	if (res.status === 403) {
		const wid = path.match(/^\/api\/weddings\/([^/?]+)/)?.[1];
		if (wid && (await reselectWedding(wid))) res = await send();
	}
	return res;
}

export async function uploadFile(file: File): Promise<{ url: string; filename: string }> {
	const { accessToken } = getAuth();
	const formData = new FormData();
	formData.append('file', file);
	// Uploads get longer than the default deadline: a photo on venue WiFi is slow.
	const signal = () => withTimeout(null, 60_000);
	let res = await fetch('/api/upload', {
		method: 'POST',
		headers: {
			Authorization: `Bearer ${accessToken}`
		},
		body: formData,
		signal: signal()
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
				body: formData,
				signal: signal()
			});
		}
	}

	if (!res.ok) {
		const err = await res.json().catch(() => ({ title: 'Upload failed' }));
		throw new Error(err.title || 'Upload failed');
	}
	return res.json();
}
