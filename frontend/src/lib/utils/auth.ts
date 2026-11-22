import { goto } from '$app/navigation';
import { getAuth, clearAuth } from '#lib/stores/index.ts';
import { refreshSession } from '#lib/api/client.ts';
import { setWeddingId } from '#lib/stores/weddingId.ts';
import { clearOfflineData, recordDiscardedQueue } from '#lib/offline/queue.ts';

function toLogin() {
  recordDiscardedQueue();
  clearOfflineData();
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
  const { accessToken } = getAuth();
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
  // 401 is an expired token; 403 can be a forced password change. The shared
  // refresh answers both and redirects to change-password when forced.
  if (res.status !== 401 && res.status !== 403) return true;
  const result = await refreshSession();
  if (result === 'ended') toLogin();
  return result === 'refreshed' || result === 'unavailable';
}
