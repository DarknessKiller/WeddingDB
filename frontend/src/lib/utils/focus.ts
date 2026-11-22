const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

// Only the dialog opened last handles Escape. Without this, one keypress closed
// the whole stack underneath it, dropping a drawer's unsaved edits.
const escapeStack: symbol[] = [];

/** Registers an Escape handler for the duration of the returned cleanup. */
export function registerEscape(handler: () => void): () => void {
  const token = Symbol('escape');
  escapeStack.push(token);
  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return;
    if (escapeStack[escapeStack.length - 1] !== token) return;
    // Capture phase + stopImmediatePropagation: the page's own Escape listeners
    // (window bubble) must not also close what is underneath this dialog.
    e.stopImmediatePropagation();
    handler();
  }
  window.addEventListener('keydown', onKeydown, true);
  return () => {
    window.removeEventListener('keydown', onKeydown, true);
    const i = escapeStack.lastIndexOf(token);
    if (i >= 0) escapeStack.splice(i, 1);
  };
}

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
