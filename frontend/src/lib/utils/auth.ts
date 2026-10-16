import { goto } from '$app/navigation';
import { getAuth, setAuth, clearAuth } from '$lib/stores';
import { setWeddingId } from '$lib/stores/weddingId';

function toLogin() {
  clearAuth();
  setWeddingId('');
  goto('/login', { replaceState: true });
}

/**
 * Validate the access token against the backend.
 * Returns false only when the session is over and the user was sent to login.
 * Network errors and server errors keep the session: the refresh token is
 * still valid, and offline pages must keep working.
 */
export async function validateToken(): Promise<boolean> {
  const { accessToken, refreshToken } = getAuth();
  if (!accessToken) {
    goto('/login', { replaceState: true });
    return false;
  }

  // Try a lightweight authenticated request to check token validity
  let res: Response;
  try {
    res = await fetch('/api/weddings', {
      headers: { Authorization: `Bearer ${accessToken}` }
    });
  } catch {
    return true;
  }
  if (res.ok) return true;

  // Only a rejected access token can be recovered with the refresh token.
  if (res.status === 401 && refreshToken) {
    let refreshRes: Response;
    try {
      refreshRes = await fetch('/api/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refreshToken })
      });
    } catch {
      // Network failure — the refresh token was never seen by the server, so keep it.
      return true;
    }
    if (refreshRes.ok) {
      const data = await refreshRes.json();
      setAuth(data.accessToken, data.refreshToken, data.role ?? getAuth().role ?? '', data.name ?? getAuth().name ?? '');
      return true;
    }
    if (refreshRes.status !== 400 && refreshRes.status !== 401) return true;
  } else if (res.status !== 401) {
    return true;
  }

  toLogin();
  return false;
}
