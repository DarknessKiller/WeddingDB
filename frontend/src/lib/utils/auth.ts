import { goto } from '$app/navigation';
import { getAuth, clearAuth } from '#lib/stores/index.ts';
import { refreshIfNeeded } from '#lib/api/client.ts';
import { setWeddingId } from '#lib/stores/weddingId.ts';
import { clearGuestStores, clearGuestCaches } from '#lib/stores/guestEvents.ts';

function toLogin() {
  clearAuth();
  setWeddingId('');
  // A dead session must not leave guest rows (emails, notes, angbao) cached.
  clearGuestStores();
  clearGuestCaches();
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
      headers: { Authorization: `Bearer ${accessToken}` },
      // Wifi with no upstream never rejects; time out so offline pages still load.
      signal: AbortSignal.timeout(10_000)
    });
  } catch {
    return true;
  }
  if (res.ok) return true;

  // Only a rejected access token can be recovered with the refresh token.
  if (res.status !== 401) return true;
  if (!refreshToken) {
    toLogin();
    return false;
  }
  // Shared refresh: a 401 clears auth and leaves for /login; 503, other errors
  // and network failures keep the session.
  return (await refreshIfNeeded()) || !!getAuth().accessToken;
}
