import { getAuth, setAuth, clearAuth } from '$lib/stores';
import { setWeddingId } from '$lib/stores/weddingId';

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
