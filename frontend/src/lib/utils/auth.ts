import { goto } from '$app/navigation';
import { getAuth, expireAuth } from '#lib/stores/index.ts';
import { setWeddingId } from '#lib/stores/weddingId.ts';
import { refreshAccessToken } from '#lib/api/client.ts';
import { withTimeout } from '#lib/utils/index.ts';

// Session expiry, not logout: keep the offline queue so queued check-ins sync after login.
function toLogin() {
  expireAuth();
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
      headers: { Authorization: `Bearer ${accessToken}` },
      signal: withTimeout()
    });
  } catch {
    return true;
  }
  if (res.ok) return true;

  // Only a rejected access token can be recovered with the refresh token.
  if (res.status !== 401) return true;
  if (refreshToken) {
    if (await refreshAccessToken()) return true;
    // The shared refresh ends the session (expireAuth + redirect) only on a definite
    // rejection; network and server failures keep the refresh token.
    return getAuth().refreshToken !== null;
  }

  toLogin();
  return false;
}
