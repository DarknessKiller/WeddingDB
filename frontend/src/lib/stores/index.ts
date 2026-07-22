import { writable, derived } from 'svelte/store';
import type { Guest, ViewMode } from '$lib/types';

// Selected guest for drawer
export const selectedGuest = writable<Guest | null>(null);
export const isDrawerOpen = writable(false);

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
