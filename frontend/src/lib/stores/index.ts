import { writable } from 'svelte/store';
import type { Guest } from '#lib/types/index.ts';
import { clearStaffOfflineData, expireStaffOfflineData } from '#lib/offline/storage.ts';

// Selected guest for drawer
export const selectedGuest = writable<Guest | null>(null);
export const isDrawerOpen = writable(false);
export const drawerStartEditing = writable(false);
export const drawerCreateMode = writable(false);

// Toast notifications
export interface Toast {
  id: string;
  message: string;
  type: 'success' | 'error' | 'info';
}
export const toasts = writable<Toast[]>([]);

export function addToast(message: string, type: Toast['type'] = 'info') {
  const id = Math.random().toString(36).slice(2);
  toasts.update(t => [...t, { id, message, type }]);
  setTimeout(() => {
    toasts.update(t => t.filter(toast => toast.id !== id));
  }, 3500);
}

// Sidebar state
export const sidebarCollapsed = writable(false);

// Auth persists in localStorage so an offline reload or browser restart keeps
// the staff pages and their cached guest list usable. Logout clears it.
const AUTH_KEY = 'weddingdb_auth';

interface StoredAuth {
  accessToken: string;
  refreshToken: string;
  role: string;
  name: string;
  // Login email. Lets a tab tell another tab's rotated token (same user) from
  // another user's login, whose tokens it must never adopt.
  user: string;
}

export function readStoredAuth(): StoredAuth | null {
  if (typeof window === 'undefined') return null;
  try {
    const raw = localStorage.getItem(AUTH_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<StoredAuth>;
    if (!parsed.accessToken || !parsed.refreshToken) return null;
    return {
      accessToken: parsed.accessToken,
      refreshToken: parsed.refreshToken,
      role: parsed.role ?? '',
      name: parsed.name ?? '',
      user: parsed.user ?? ''
    };
  } catch {
    return null;
  }
}

const storedAuth = readStoredAuth();
let _accessToken: string | null = storedAuth?.accessToken ?? null;
let _refreshToken: string | null = storedAuth?.refreshToken ?? null;
let _role: string | null = storedAuth?.role ?? null;
let _name: string | null = storedAuth?.name ?? null;
let _user: string | null = storedAuth?.user ?? null;

export function getAuth() {
  return { accessToken: _accessToken, refreshToken: _refreshToken, role: _role, name: _name };
}

// Refresh tokens this tab already rotated away from. A failed storage write
// leaves one of them in localStorage; refreshing with it is a replay that
// revokes the whole family, so it must never be mistaken for another tab's.
const retiredRefreshTokens = new Set<string>();
export function isRetiredRefreshToken(token: string): boolean {
  return retiredRefreshTokens.has(token);
}

// user defaults to the current one: refresh and wedding selection keep the same user.
export function setAuth(access: string, refresh: string, r: string, n: string, user = _user) {
  const hadSession = !!_refreshToken;
  if (_refreshToken && _refreshToken !== refresh) retiredRefreshTokens.add(_refreshToken);
  _accessToken = access;
  _refreshToken = refresh;
  _role = r;
  _name = n;
  _user = user;
  writeStoredAuth(hadSession);
}

function writeStoredAuth(guardForeignSession = false) {
  if (typeof window === 'undefined' || !_accessToken || !_refreshToken) return;
  try {
    // Another tab may have logged in as someone else while this tab's refresh
    // was in flight; that session wins and this tab ends on its storage event.
    const stored = guardForeignSession ? readStoredAuth() : null;
    if (stored?.user && stored.user !== _user) return;
    localStorage.setItem(AUTH_KEY, JSON.stringify({
      accessToken: _accessToken,
      refreshToken: _refreshToken,
      role: _role ?? '',
      name: _name ?? '',
      user: _user ?? ''
    } satisfies StoredAuth));
  } catch {}
}

function resetAuthState(keepStored = false) {
  _accessToken = null;
  _refreshToken = null;
  _role = null;
  _name = null;
  _user = null;
  if (typeof window !== 'undefined' && !keepStored) localStorage.removeItem(AUTH_KEY);
}

/** Explicit logout: the next person at this device must not inherit data. */
export function clearAuth() {
  resetAuthState();
  clearStaffOfflineData();
}

/**
 * The server rejected the refresh token. End the session but keep unsynced
 * edits and their owner so the same user's queued changes sync after login.
 */
export function expireAuth() {
  resetAuthState();
  expireStaffOfflineData();
}

/** True when the stored auth is this tab's user. Auth stored without a user never matches. */
export function isSameUser(stored: StoredAuth): boolean {
  return !!stored.user && stored.user === _user;
}

/**
 * Another user logged in from another tab. End this tab's session in memory
 * only: the stored auth and wedding id are theirs now.
 */
export function dropForeignSession() {
  resetAuthState(true);
  expireStaffOfflineData();
}

// Show seat numbers setting. Shared so every page reacts without a reload, and
// mirrored to localStorage so a kiosk/preview tab in the same browser updates too.
export const showSeatNumbers = writable(false);

const seatNumbersKey = (wid: string) => `weddingdb_show_seat_numbers_${wid}`;
let currentWid = '';


export function setShowSeatNumbers(wid: string, value: boolean) {
	currentWid = wid;
	showSeatNumbers.set(value);
	try {
		localStorage.setItem(seatNumbersKey(wid), value ? '1' : '0');
	} catch {}
}

/**
 * Another tab logged out, its session expired (the stored auth is gone), or
 * another user logged in. Drop this tab's tokens and roster cache too, or it
 * keeps showing guest data and later adopts the next user's tokens. Queued
 * edits stay, as on expiry. Returns true when this tab had a session to end.
 */
export function endSessionFromOtherTab(): boolean {
	if (!_refreshToken) return false;
	const stored = readStoredAuth();
	if (!stored) {
		expireAuth();
		return true;
	}
	if (stored.refreshToken === _refreshToken || isSameUser(stored)) return false;
	dropForeignSession();
	return true;
}

if (typeof window !== 'undefined') {
	window.addEventListener('storage', (e) => {
		if (e.key === seatNumbersKey(currentWid)) showSeatNumbers.set(e.newValue !== '0');
		// key is null when another tab called localStorage.clear().
		if (e.key !== AUTH_KEY && e.key !== null) return;
		// The public kiosk never uses the staff session, so it keeps running.
		if (window.location.pathname.startsWith('/kiosk')) return;
		if (endSessionFromOtherTab()) window.location.href = '/login';
	});
}
