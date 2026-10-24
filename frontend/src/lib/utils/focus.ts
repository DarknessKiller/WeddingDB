const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

// Keeps Tab inside a modal panel and restores focus to the trigger on close.
export function trapFocus(node: HTMLElement) {
  const previous = document.activeElement as HTMLElement | null;
  const items = () =>
    Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(el => el.offsetParent !== null);

  (items()[0] ?? node).focus();

  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Tab') return;
    const list = items();
    if (!list.length) return;
    const first = list[0];
    const last = list[list.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }

  node.addEventListener('keydown', onKeydown);
  return {
    destroy() {
      node.removeEventListener('keydown', onKeydown);
      if (previous?.isConnected) previous.focus();
    },
  };
}
