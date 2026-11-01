import { describe, expect, it } from 'vitest';
import { normalizeGuestQuery, rankGuests } from './guestSearch';

const guests = [
  { id: '1', name: '张伟', pinyin: 'zhangwei' },
  { id: '2', name: '张薇', pinyin: 'zhangwei' },
  { id: '3', name: 'Wei Zhang', pinyin: 'weizhang' },
  { id: '4', name: '王芳', pinyin: 'wangfang' },
  { id: '5', name: '李娜' },
];

describe('guestSearch', () => {
  it('normalizes case, spaces and punctuation', () => {
    expect(normalizeGuestQuery("  O'Neil-Smith ")).toBe('oneilsmith');
  });

  it('finds a Chinese name and its pinyin', () => {
    expect(rankGuests(guests, '张伟').map((g) => g.id)).toEqual(['1']);
    expect(rankGuests(guests, 'zhangwei').map((g) => g.id)).toContain('1');
  });

  it('matches spaced pinyin as the syllables are stored joined', () => {
    expect(rankGuests(guests, 'zhang wei').map((g) => g.id)).toContain('1');
  });

  it('ranks exact name before a pinyin substring', () => {
    expect(rankGuests(guests, 'wangfang')[0].id).toBe('4');
  });

  it('falls back to the name when pinyin is absent', () => {
    expect(rankGuests(guests, '李娜').map((g) => g.id)).toEqual(['5']);
  });

  it('returns nothing for an empty query', () => {
    expect(rankGuests(guests, '   ')).toEqual([]);
  });
});
