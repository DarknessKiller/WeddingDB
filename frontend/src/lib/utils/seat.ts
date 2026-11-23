import { LIMITS } from './limits.ts';

export function formatSeatRange(seatNumber: number | null, pax: number, withLabel = true): string {
  if (seatNumber === null) return 'No seat assigned';
  const range = pax <= 1 ? `${seatNumber}` : `${seatNumber}–${seatNumber + pax - 1}`;
  return withLabel ? `Seat ${range}` : range;
}

export function formatPartySize(pax: number): string {
  return `${pax} ${pax === 1 ? 'guest' : 'guests'}`;
}

/** Highest occupied seat end (seatNumber + pax - 1), mirroring the server floor.
 * Guests without a seat are ignored. */
export function highestOccupiedSeat(
  guests: readonly { seatNumber: number | null; pax?: number }[]
): number {
  return guests.reduce(
    (hi, g) => (g.seatNumber == null ? hi : Math.max(hi, g.seatNumber + (g.pax ?? 1) - 1)),
    0
  );
}

/** Clamp a capacity field value into [floor, LIMITS.tableCapacity]. */
export function clampTableCapacity(value: number, floor: number): number {
  const v = Math.floor(Number(value));
  return Math.min(LIMITS.tableCapacity, Math.max(floor, Number.isFinite(v) ? v : floor));
}

/** Group guests by table id, skipping unassigned ones (used by the map editors). */
export function groupGuestsByTable<T extends { tableId: string | null }>(
  guests: readonly T[]
): Record<string, T[]> {
  const grouped: Record<string, T[]> = {};
  for (const g of guests) {
    if (g.tableId === null) continue;
    const key = g.tableId;
    (grouped[key] ??= []).push(g);
  }
  return grouped;
}
