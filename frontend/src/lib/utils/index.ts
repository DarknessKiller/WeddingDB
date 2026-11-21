import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function getInitials(name: string): string {
  return name
    .split(' ')
    .map(w => w[0])
    .join('')
    .slice(0, 2)
    .toUpperCase();
}

// Venue WiFi can stay "connected" with no internet; a request with no deadline
// then hangs until the OS gives up, minutes later.
export const REQUEST_TIMEOUT_MS = 15_000;

/** A signal that aborts after ms, or earlier when the caller's signal aborts. */
export function withTimeout(signal?: AbortSignal | null, ms = REQUEST_TIMEOUT_MS): AbortSignal {
  const timeout = AbortSignal.timeout(ms);
  if (!signal) return timeout;
  // shortcut: browsers without AbortSignal.any (Safari < 17.4) keep only the caller's signal.
  return typeof AbortSignal.any === 'function' ? AbortSignal.any([signal, timeout]) : signal;
}

// Admin-set background values go into a style attribute: anything but a known
// size or a 0-100% / keyword position is dropped, and the URL is quoted with
// its quote, backslash and newlines escaped, so a value cannot add CSS.
const BG_SIZES = ['cover', 'contain', 'auto'];
const BG_KEYWORDS = ['center', 'left', 'right', 'top', 'bottom'];
export function cssPosition(v: string): string {
  if (BG_KEYWORDS.includes(v)) return v;
  const n = /^(\d+(?:\.\d+)?)%$/.exec(v);
  return n ? `${Math.min(100, Number(n[1]))}%` : 'center';
}
export function cssSize(v: string): string {
  return BG_SIZES.includes(v) ? v : 'cover';
}
export function backgroundStyle(url: string, size: string, posX: string, posY: string): string {
  const safeUrl = url.replace(/["\\\n\r]/g, encodeURIComponent);
  return `background-image: url("${safeUrl}"); background-size: ${cssSize(size)}; background-position: ${cssPosition(posX)} ${cssPosition(posY)};`;
}
