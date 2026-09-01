// svelte-check (via svelte2tsx) desugars `transition:`/`in:`/`out:` directives into
// an attribute of the same name, but svelte/elements' HTMLAttributes doesn't declare
// them, so svelte-check errors with "Object literal may only specify known properties".
// Index signature workaround until sveltejs/language-tools handles directives natively.
import 'svelte/elements';

declare module 'svelte/elements' {
  interface HTMLAttributes<T extends EventTarget = any> {
    out?: unknown;
    in?: unknown;
    [key: `transition:${string}`]: unknown;
    [key: `in:${string}`]: unknown;
    [key: `out:${string}`]: unknown;
  }
}
