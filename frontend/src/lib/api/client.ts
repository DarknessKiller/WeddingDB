import { getAuth, setAuth, clearAuth } from '$lib/stores';
import { setWeddingId } from '$lib/stores/weddingId';

const BASE = '';

export function getAccessToken(): string {
	const { accessToken } = getAuth();
	return accessToken ?? '';
}

export async function apiFetch(path: string, opts: RequestInit = {}): Promise<Response> {
	const { accessToken } = getAuth();
	let res = await fetch(`${BASE}${path}`, {
		...opts,
		headers: {
			'Content-Type': 'application/json',
			...opts.headers,
			...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {})
		}
	});

	// Auto-refresh on 401
	if (res.status === 401) {
		const { refreshToken } = getAuth();
		if (refreshToken) {
			try {
				const refreshRes = await fetch(`${BASE}/api/auth/refresh`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ refreshToken })
				});
				if (refreshRes.ok) {
					const data = await refreshRes.json();
					setAuth(data.accessToken, data.refreshToken, data.role ?? getAuth().role ?? '', data.name ?? getAuth().name ?? '');
					// On refresh, if we have a stored wedding ID keep it
					res = await fetch(`${BASE}${path}`, {
						...opts,
						headers: {
							'Content-Type': 'application/json',
							...opts.headers,
							Authorization: `Bearer ${data.accessToken}`
						}
					});
				} else {
					clearAuth();
					setWeddingId('');
					if (typeof window !== 'undefined') {
						window.location.href = '/login';
					}
				}
			} catch {
				clearAuth();
				setWeddingId('');
				if (typeof window !== 'undefined') {
					window.location.href = '/login';
				}
			}
		}
	}
	return res;
}

export async function uploadFile(file: File): Promise<{ url: string; filename: string }> {
	const formData = new FormData();
	formData.append('file', file);
	const res = await fetch('/api/upload', {
		method: 'POST',
		headers: {
			Authorization: `Bearer ${getAccessToken()}`
		},
		body: formData
	});
	if (!res.ok) {
		const err = await res.json().catch(() => ({ title: 'Upload failed' }));
		throw new Error(err.title || 'Upload failed');
	}
	return res.json();
}
