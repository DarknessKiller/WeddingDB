export function formatSeatRange(seatNumber: number | null, pax: number, withLabel = true): string {
  if (seatNumber === null) return 'No seat assigned';
  const range = pax <= 1 ? `${seatNumber}` : `${seatNumber}–${seatNumber + pax - 1}`;
  return withLabel ? `Seat ${range}` : range;
}

export function formatPartySize(pax: number): string {
  return `${pax} ${pax === 1 ? 'guest' : 'guests'}`;
}
