import { writable } from 'svelte/store';
import zh from './zh';
import en from './en';

export type Locale = 'zh' | 'en';

const KEY = 'weddingdb_locale';
const dicts: Record<Locale, Record<string, string>> = { zh, en };

function initial(): Locale {
  if (typeof window === 'undefined') return 'zh';
  try {
    const v = localStorage.getItem(KEY);
    if (v === 'en' || v === 'zh') return v;
  } catch {}
  return 'zh';
}

let current: Locale = initial();

export const locale = writable<Locale>(current);

locale.subscribe((v) => {
  current = v;
  if (typeof window === 'undefined') return;
  try { localStorage.setItem(KEY, v); } catch {}
});

// Cross-tab: language toggle in one tab updates kiosk/preview tabs too.
if (typeof window !== 'undefined') {
  window.addEventListener('storage', (e) => {
    if (e.key === KEY && (e.newValue === 'en' || e.newValue === 'zh')) locale.set(e.newValue);
  });
}

export function setLocale(v: Locale) {
  locale.set(v);
}

// Interpolates "{name}" style params; missing keys fall back to zh, then the key itself.
export function t(key: string, params?: Record<string, string | number>): string {
  let value = dicts[current][key] ?? dicts.zh[key] ?? key;
  if (params) {
    for (const [k, v] of Object.entries(params)) value = value.replaceAll(`{${k}}`, String(v));
  }
  return value;
}

// shortcut: i18n infra exists, remaining routes still hardcoded zh, convert on touch
