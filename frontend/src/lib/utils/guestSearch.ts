export interface SearchableGuest {
  name: string;
  pinyin?: string;
}

/** Compare names the way a hurried door volunteer types them: no case, spaces or punctuation. */
export function normalizeGuestQuery(value: string): string {
  return value
    .normalize('NFKC')
    .toLowerCase()
    .replace(/[\s'’·.,-]+/g, '');
}

/**
 * Rank guests locally for instant type-ahead. Name matches win over pinyin
 * matches; prefix matches win over substring matches.
 */
export function rankGuests<T extends SearchableGuest>(guests: readonly T[], query: string): T[] {
  const q = normalizeGuestQuery(query);
  if (!q) return [];

  const ranked: { guest: T; score: number }[] = [];
  for (const guest of guests) {
    const name = normalizeGuestQuery(guest.name);
    const pinyin = normalizeGuestQuery(guest.pinyin ?? '');
    let score = -1;
    if (name === q) score = 0;
    else if (name.startsWith(q)) score = 1;
    else if (pinyin.startsWith(q)) score = 2;
    else if (name.includes(q)) score = 3;
    else if (pinyin.includes(q)) score = 4;
    if (score >= 0) ranked.push({ guest, score });
  }

  ranked.sort((a, b) => a.score - b.score || a.guest.name.localeCompare(b.guest.name, 'zh'));
  return ranked.map((entry) => entry.guest);
}
