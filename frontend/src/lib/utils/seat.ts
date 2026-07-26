export function formatSeatRange(seatNumber: number | null, pax: number): string {
  if (seatNumber === null) return 'No seat assigned';
  if (pax <= 1) return `Seat ${seatNumber}`;
  return `Seat ${seatNumber}–${seatNumber + pax - 1}`;
}
