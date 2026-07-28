import { describe, it, expect } from 'vitest';

type GuestRow = { angbaoAmt?: number | null };

function totalAngbao(guests: GuestRow[]): number {
  return guests.reduce((sum, g) => sum + (g.angbaoAmt ?? 0), 0);
}

describe('totalAngbao', () => {
  it('sums angbao amounts', () => {
    const guests = [{ angbaoAmt: 100 }, { angbaoAmt: 200 }, { angbaoAmt: 50 }];
    expect(totalAngbao(guests)).toBe(350);
  });

  it('handles null/undefined as zero', () => {
    const guests = [{ angbaoAmt: 100 }, { angbaoAmt: null }, { angbaoAmt: undefined }];
    expect(totalAngbao(guests)).toBe(100);
  });

  it('returns 0 for empty list', () => {
    expect(totalAngbao([])).toBe(0);
  });

  it('returns 0 when all null', () => {
    const guests = [{ angbaoAmt: null }, { angbaoAmt: null }];
    expect(totalAngbao(guests)).toBe(0);
  });
});
