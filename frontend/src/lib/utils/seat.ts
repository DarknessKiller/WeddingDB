import { t } from '$lib/i18n';

export function formatSeatRange(seatNumber: number | null, pax: number): string {
  if (seatNumber === null) return t('move.seatNone');
  if (pax <= 1) return t('move.seatN', { n: seatNumber });
  return t('move.partyOf', { n: pax, a: seatNumber, b: seatNumber + pax - 1 });
}

export function formatPartySize(pax: number): string {
  return t('pax', { n: pax });
}
