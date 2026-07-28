import { writable, derived } from 'svelte/store';
import type { Guest, ViewMode } from '$lib/types';

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

// Kiosk mode
export const kioskTargetGuest = writable<Guest | null>(null);
export const kioskTargetTable = writable<number | null>(null);
export const kioskTargetSeat = writable<number | null>(null);

// Auth state (backed by localStorage for persistence)
let _accessToken: string | null = null;
let _refreshToken: string | null = null;
let _role: string | null = null;
let _name: string | null = null;

function loadAuth() {
  if (typeof window === 'undefined') return;
  try {
    _accessToken = localStorage.getItem('weddingdb_access_token');
    _refreshToken = localStorage.getItem('weddingdb_refresh_token');
    _role = localStorage.getItem('weddingdb_role');
    _name = localStorage.getItem('weddingdb_name');
  } catch {}
}
loadAuth();

export function getAuth() {
  return { accessToken: _accessToken, refreshToken: _refreshToken, role: _role, name: _name };
}

export function setAuth(access: string, refresh: string, r: string, n: string) {
  _accessToken = access;
  _refreshToken = refresh;
  _role = r;
  _name = n;
  if (typeof window !== 'undefined') {
    localStorage.setItem('weddingdb_access_token', access);
    localStorage.setItem('weddingdb_refresh_token', refresh);
    localStorage.setItem('weddingdb_role', r);
    localStorage.setItem('weddingdb_name', n);
  }
}

export function clearAuth() {
  _accessToken = null;
  _refreshToken = null;
  _role = null;
  _name = null;
  if (typeof window !== 'undefined') {
    localStorage.removeItem('weddingdb_access_token');
    localStorage.removeItem('weddingdb_refresh_token');
    localStorage.removeItem('weddingdb_role');
    localStorage.removeItem('weddingdb_name');
  }
}
