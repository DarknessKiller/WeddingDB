import { getAuth, setAuth, clearAuth } from '$lib/stores';
import { setWeddingId } from '$lib/stores/weddingId';

const BASE = '';

export function getAccessToken(): string {
	const { accessToken } = getAuth();
	return accessToken ?? '';
}

async function tryRefreshToken(): Promise<string | null> {
	const { refreshToken } = getAuth();
	if (!refreshToken) return null;
	try {
		const refreshRes = await fetch(`${BASE}/api/auth/refresh`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ refreshToken })
		});
		if (!refreshRes.ok) {
			redirectToLogin();
			return null;
		}
		const data = await refreshRes.json();
		if (data.forcePasswordChange) {
			setAuth(data.accessToken, data.refreshToken, data.role ?? getAuth().role ?? '', data.name ?? getAuth().name ?? '');
			if (typeof window !== 'undefined') window.location.href = '/change-password';
			return null;
		}
		setAuth(data.accessToken, data.refreshToken, data.role ?? getAuth().role ?? '', data.name ?? getAuth().name ?? '');
		return data.accessToken;
	} catch {
		redirectToLogin();
		return null;
	}
}

function redirectToLogin() {
	clearAuth();
	setWeddingId('');
	if (typeof window !== 'undefined') window.location.href = '/login';
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
		const newToken = await tryRefreshToken();
		if (newToken) {
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
		headers: { Authorization: `Bearer ${accessToken}` },
		body: formData
	});

	if (res.status === 401) {
		const newToken = await tryRefreshToken();
		if (newToken) {
			res = await fetch('/api/upload', {
				method: 'POST',
				headers: { Authorization: `Bearer ${newToken}` },
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
