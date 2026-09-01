// WAAPI outro transitions: always animate from the element's current
// (presentation) transform, so a mid-drag or interrupted motion never snaps.

const SPRING = 'cubic-bezier(0.2, 0.8, 0.2, 1)';

function currentTranslate(node: HTMLElement): { x: number; y: number } {
  const m = getComputedStyle(node).transform.match(/matrix.*\((.+)\)/);
  if (!m) return { x: 0, y: 0 };
  const parts = m[1].split(', ');
  return { x: parseFloat(parts[parts.length - 2]) || 0, y: parseFloat(parts[parts.length - 1]) || 0 };
}

// Drawer/sheet exit: slide off the same edge it entered, starting from
// wherever the element currently is (respects an in-progress drag offset).
export function slideOut(node: HTMLElement, opts: { dir?: 'x' | 'y'; duration?: number } = {}) {
  const { dir = 'y', duration = 300 } = opts;
  const { x, y } = currentTranslate(node);
  const from = dir === 'x' ? `translateX(${x}px)` : `translateY(${y}px)`;
  const to = dir === 'x' ? 'translateX(100%)' : 'translateY(100%)';
  node.animate([{ transform: from, opacity: 1 }, { transform: to, opacity: 0.7 }], {
    duration, easing: SPRING, fill: 'forwards'
  });
  return { duration };
}

// Toast exit: same edge it entered (right side), with fade.
export function toastOut(node: HTMLElement, opts: { duration?: number } = {}) {
  const { duration = 200 } = opts;
  node.animate(
    [
      { opacity: 1, transform: 'translateX(0) scale(1)' },
      { opacity: 0, transform: 'translateX(24px) scale(0.95)' }
    ],
    { duration, easing: SPRING, fill: 'forwards' }
  );
  return { duration };
}
