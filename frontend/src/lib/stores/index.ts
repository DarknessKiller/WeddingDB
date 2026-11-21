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
      name: parsed.name ?? ''
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

export function getAuth() {
  return { accessToken: _accessToken, refreshToken: _refreshToken, role: _role, name: _name };
}

export function setAuth(access: string, refresh: string, r: string, n: string) {
  _accessToken = access;
  _refreshToken = refresh;
  _role = r;
  _name = n;
  writeStoredAuth();
}

function writeStoredAuth() {
  if (typeof window === 'undefined' || !_accessToken || !_refreshToken) return;
  try {
    localStorage.setItem(AUTH_KEY, JSON.stringify({
      accessToken: _accessToken,
      refreshToken: _refreshToken,
      role: _role ?? '',
      name: _name ?? ''
    } satisfies StoredAuth));
  } catch {}
}

function resetAuthState() {
  _accessToken = null;
  _refreshToken = null;
  _role = null;
  _name = null;
  if (typeof window !== 'undefined') localStorage.removeItem(AUTH_KEY);
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
 * Another tab logged out or its session expired (the stored auth is gone).
 * Drop this tab's tokens and roster cache too, or it keeps showing guest data
 * and later adopts the next user's tokens. Queued edits stay, as on expiry.
 * Returns true when this tab had a session to end.
 */
export function endSessionFromOtherTab(): boolean {
	if (!_refreshToken || readStoredAuth()) return false;
	expireAuth();
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
