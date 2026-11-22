import { writable } from 'svelte/store';
import { setWeddingId } from '#lib/stores/weddingId.ts';
import type { Guest } from '#lib/types/index.ts';

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
let _name: string | null = null;
let _role: string | null = null;
let _userId: string | null = null;

export function getAuth() {
  return { accessToken: _accessToken, refreshToken: _refreshToken, role: _role, name: _name, userId: _userId };
}

export function setAuth(access: string, refresh: string, r: string, n: string, userId?: string) {
  _accessToken = access;
  _refreshToken = refresh;
  _role = r;
  _name = n;
  if (userId) _userId = userId;
}

const LOGOUT_KEY = 'weddingdb_logout';
let applyingRemoteLogout = false;

/** Tell every other tab this session ended. The emitting tab sees no storage event. */
function broadcastLogout() {
  if (typeof window === 'undefined' || applyingRemoteLogout) return;
  try { localStorage.setItem(LOGOUT_KEY, String(Date.now())); } catch {}
}

function applyRemoteLogout() {
  if (!_accessToken && !_refreshToken) return;
  applyingRemoteLogout = true;
  try {
    clearAuth();
    setWeddingId('');
  } finally {
    applyingRemoteLogout = false;
  }
  // Static import would cycle: queue.ts imports addToast from this module.
  void import('#lib/offline/queue.ts').then(({ recordDiscardedQueue, clearOfflineData }) => {
    recordDiscardedQueue();
    clearOfflineData();
    if (typeof window !== 'undefined') window.location.href = '/login';
  });
}

export function clearAuth() {
  _accessToken = null;
  _refreshToken = null;
  _role = null;
  _name = null;
  _userId = null;
  if (typeof window !== 'undefined') {
    localStorage.removeItem('weddingdb_access_token');
    localStorage.removeItem('weddingdb_refresh_token');
  }
  broadcastLogout();
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
		if (e.key === LOGOUT_KEY && e.newValue) applyRemoteLogout();
	});
}
