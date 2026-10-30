import { writable } from 'svelte/store';
import type { Guest } from '$lib/types';

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

// Auth state. Only the short-lived access token is persisted: the refresh
// token lives in an HttpOnly cookie that page script cannot read, so an XSS
// costs one access-token lifetime instead of the whole session.
let _accessToken: string | null = null;
let _role: string | null = null;
let _name: string | null = null;

function loadAuth() {
  if (typeof window === 'undefined') return;
  try {
    _accessToken = localStorage.getItem('weddingdb_access_token');
    _role = localStorage.getItem('weddingdb_role');
    _name = localStorage.getItem('weddingdb_name');
  } catch {}
}
loadAuth();

// Another tab can refresh or sign out. Mirror it instead of holding a stale token.
if (typeof window !== 'undefined') {
  window.addEventListener('storage', (e) => {
    if (e.key === null || e.key.startsWith('weddingdb_')) loadAuth();
  });
}

export function getAuth() {
  return { accessToken: _accessToken, role: _role, name: _name };
}

function dropLegacyRefreshToken() {
  try { localStorage.removeItem('weddingdb_refresh_token'); } catch {}
}

export function setAuth(access: string, r: string, n: string) {
  _accessToken = access;
  _role = r;
  _name = n;
  if (typeof window !== 'undefined') {
    localStorage.setItem('weddingdb_access_token', access);
    localStorage.setItem('weddingdb_role', r);
    localStorage.setItem('weddingdb_name', n);
    dropLegacyRefreshToken();
  }
}

export function clearAuth() {
  _accessToken = null;
  _role = null;
  _name = null;
  if (typeof window !== 'undefined') {
    localStorage.removeItem('weddingdb_access_token');
    localStorage.removeItem('weddingdb_refresh_token');
    localStorage.removeItem('weddingdb_role');
    localStorage.removeItem('weddingdb_name');
  }
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

if (typeof window !== 'undefined') {
	window.addEventListener('storage', (e) => {
		if (e.key === seatNumbersKey(currentWid)) showSeatNumbers.set(e.newValue !== '0');
	});
}

// Kiosk idle auto-reset (opt-in). Same localStorage mirror so the kiosk tab
// picks up the settings change without a reload.
export const kioskIdleReset = writable(false);

const idleResetKey = (wid: string) => `weddingdb_kiosk_idle_reset_${wid}`;

export function setKioskIdleReset(wid: string, value: boolean) {
	currentWid = wid;
	kioskIdleReset.set(value);
	try {
		localStorage.setItem(idleResetKey(wid), value ? '1' : '0');
	} catch {}
}
