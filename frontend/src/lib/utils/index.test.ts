import { describe, it, expect } from 'vitest';
import { backgroundStyle, cssPosition } from './index';

describe('backgroundStyle', () => {
  it('keeps valid admin values', () => {
    expect(backgroundStyle('https://x/a.jpg', 'contain', '37%', 'top'))
      .toBe('background-image: url("https://x/a.jpg"); background-size: contain; background-position: 37% top;');
  });

  it('cannot be broken out of by injected values', () => {
    const style = backgroundStyle('x") ; color: red; ("', 'cover; position: fixed', '1%; z-index: 9', '150%');
    expect(style).toContain('url("x%22) ; color: red; (%22")');
    expect(style.slice(style.indexOf('");'))).not.toMatch(/color|position: fixed|z-index/);
    expect(style).toContain('background-size: cover;');
    expect(style).toContain('background-position: center 100%;');
  });

  it('drops a non-percent position', () => {
    expect(cssPosition('calc(1px)')).toBe('center');
  });
});
