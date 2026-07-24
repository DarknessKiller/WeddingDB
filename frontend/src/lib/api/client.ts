import { getAuth, setAuth, clearAuth } from '$lib/stores';
import { setWeddingId } from '$lib/stores/weddingId';
import { encodeId } from '$lib/utils/encode';

const BASE = '';

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
					if (data.weddingId) {
						setWeddingId(encodeId(data.weddingId));
					}
					res = await fetch(`${BASE}${path}`, {
						...opts,
						headers: {
							'Content-Type': 'application/json',
							...opts.headers,
							Authorization: `Bearer ${data.accessToken}`
						}
					});
				} else {
					// Refresh failed — clear auth and redirect to login
					clearAuth();
					if (typeof window !== 'undefined') {
						window.location.href = '/login';
					}
				}
			} catch {
				// Network error on refresh — clear auth
				clearAuth();
				if (typeof window !== 'undefined') {
					window.location.href = '/login';
				}
			}
		}
	}
	return res;
}
