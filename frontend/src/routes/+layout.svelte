<script lang="ts">
  import '../app.css';
  import ToastContainer from '$lib/components/ui/ToastContainer.svelte';
  let { children } = $props();
</script>

{@render children()}
<ToastContainer />

<style>
  :global(html) {
    -webkit-tap-highlight-color: transparent;
  }

  :global(body) {
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, system-ui, sans-serif;
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
  }

  /* Reduced motion */
  @media (prefers-reduced-motion: reduce) {
    :global(*) {
      animation-duration: 0.01ms !important;
      animation-iteration-count: 1 !important;
      transition-duration: 0.15s !important;
    }
  }
</style>
