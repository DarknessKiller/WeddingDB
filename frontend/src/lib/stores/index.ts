import { writable } from 'svelte/store';
import type { Guest } from '#lib/types/index.ts';
import { clearStaffOfflineData } from '#lib/offline/storage.ts';

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

// Auth tokens stay in memory only; backend has no cookie-backed session.
if (typeof window !== 'undefined') {
  try {
    localStorage.removeItem('weddingdb_access_token');
    localStorage.removeItem('weddingdb_refresh_token');
  } catch {}
}
let _accessToken: string | null = null;
let _refreshToken: string | null = null;
let _role: string | null = null;
let _name: string | null = null;

export function getAuth() {
  return { accessToken: _accessToken, refreshToken: _refreshToken, role: _role, name: _name };
}

export function setAuth(access: string, refresh: string, r: string, n: string) {
  _accessToken = access;
  _refreshToken = refresh;
  _role = r;
  _name = n;
}

export function clearAuth() {
  _accessToken = null;
  _refreshToken = null;
  _role = null;
  _name = null;
  if (typeof window !== 'undefined') {
    localStorage.removeItem('weddingdb_access_token');
    localStorage.removeItem('weddingdb_refresh_token');
  }
  // Staff guest data and unsynced edits must not outlive the session: the next
  // person at this device (or the public kiosk) could read or replay them.
  clearStaffOfflineData();
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
