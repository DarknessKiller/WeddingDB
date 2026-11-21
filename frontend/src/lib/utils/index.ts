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
