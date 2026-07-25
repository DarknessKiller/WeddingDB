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
					res = await fetch('/api/upload', {
						method: 'POST',
						headers: {
							Authorization: `Bearer ${data.accessToken}`
						},
						body: formData
					});
				} else {
					clearAuth();
					setWeddingId('');
					if (typeof window !== 'undefined') window.location.href = '/login';
				}
			} catch {
				clearAuth();
				setWeddingId('');
				if (typeof window !== 'undefined') window.location.href = '/login';
			}
		}
	}

	if (!res.ok) {
		const err = await res.json().catch(() => ({ title: 'Upload failed' }));
		throw new Error(err.title || 'Upload failed');
	}
	return res.json();
}
