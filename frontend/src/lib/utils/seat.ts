export function formatSeatRange(seatNumber: number | null, pax: number, withLabel = true): string {
  if (seatNumber === null) return 'No seat assigned';
  const range = pax <= 1 ? `${seatNumber}` : `${seatNumber}–${seatNumber + pax - 1}`;
  return withLabel ? `Seat ${range}` : range;
}

export function formatPartySize(pax: number): string {
  return `${pax} ${pax === 1 ? 'guest' : 'guests'}`;
}

// First run of `count` consecutive free seats (1-based) at a table, or [] when
// none fits. A guest occupies seatNumber..seatNumber+pax-1, so a party must sit
// in a contiguous run.
export function firstFreeSeatRun(capacity: number, occupied: Set<number>, count: number): number[] {
  for (let start = 1; start + count - 1 <= capacity; start++) {
    const run = Array.from({ length: count }, (_, i) => start + i);
    if (run.every(s => !occupied.has(s))) return run;
  }
  return [];
}

export function isContiguous(seats: number[]): boolean {
  return seats.every((s, i) => i === 0 || s === seats[i - 1] + 1);
}
