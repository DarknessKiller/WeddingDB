<script lang="ts">
  // Number ticker: rolls digits to the new value when stats update over SSE.
  // Spring-driven via rAF; respects prefers-reduced-motion (snaps instantly).
  let { value, duration = 500 }: { value: number | string; duration?: number } = $props();

  let display = $state(0);
  let raf = 0;

  const target = $derived(typeof value === 'number' ? value : parseFloat(String(value)) || 0);

  $effect(() => {
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const from = display;
    const to = target;
    if (reduce || from === to) { display = to; return; }
    const start = performance.now();
    cancelAnimationFrame(raf);
    // Critically damped spring (same feel as the kiosk sheet)
    const omega = 4 * Math.PI / (duration / 1000);
    const tick = (now: number) => {
      const t = (now - start) / 1000;
      const ease = 1 - Math.exp(-omega * t) * (1 + omega * t);
      display = from + (to - from) * ease;
      if (Math.abs(display - to) < 0.5) { display = to; return; }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  });

  const shown = $derived(Number.isInteger(target) ? Math.round(display) : Math.round(display * 10) / 10);
</script>

<span class="ticker">{shown}</span>

<style>
  .ticker { font-variant-numeric: tabular-nums; }
</style>
