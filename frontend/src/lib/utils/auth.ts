import { goto } from '$app/navigation';
import { getAuth, clearAuth } from '$lib/stores';
import { setWeddingId } from '$lib/stores/weddingId';

/**
 * Validate the access token against the backend.
 * Returns true if valid, false if expired/deleted (and redirects to login).
 */
export async function validateToken(): Promise<boolean> {
  const { accessToken, refreshToken } = getAuth();
  if (!accessToken) {
    goto('/login', { replaceState: true });
    return false;
  }

  // Try a lightweight authenticated request to check token validity
  try {
    const res = await fetch('/api/weddings', {
      headers: { Authorization: `Bearer ${accessToken}` }
    });
    if (res.ok) return true;

    // Token invalid — try refresh
    if (refreshToken) {
      const refreshRes = await fetch('/api/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refreshToken })
      });
      if (refreshRes.ok) return true; // refreshed successfully
    }

    // Can't recover — clear and redirect
    clearAuth();
    setWeddingId('');
    goto('/login', { replaceState: true });
    return false;
  } catch {
    clearAuth();
    setWeddingId('');
    goto('/login', { replaceState: true });
    return false;
  }
}
