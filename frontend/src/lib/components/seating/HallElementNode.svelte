<script lang="ts">
  import { onMount } from 'svelte';
  import type { HallElement } from '$lib/types';

  let { element, hallWidth, hallHeight, dark = false, mode = 'view', ondragend, ontransformend, onselect, onrefready }: {
    element: HallElement;
    hallWidth: number;
    hallHeight: number;
    dark?: boolean;
    mode?: 'view' | 'edit';
    ondragend?: (e: any) => void;
    ontransformend?: (e: any) => void;
    onselect?: () => void;
    onrefready?: (node: any) => void;
  } = $props();

  const px = $derived(element.x / 100 * hallWidth);
  const py = $derived(element.y / 100 * hallHeight);
  const w = $derived(element.width / 100 * hallWidth);
  const h = $derived(element.height / 100 * hallHeight);

  let Group: any = $state(null);
  let Rect: any = $state(null);
  let KText: any = $state(null);
  let groupEl = $state<any>(null);

  export function getNode() {
    return groupEl?.node ?? groupEl;
  }

  // Register the underlying Konva node with the parent.
  $effect(() => {
    if (groupEl) onrefready?.(groupEl.node ?? groupEl);
  });

  onMount(async () => {
    const mod = await import('svelte-konva');
    Group = mod.Group;
    Rect = mod.Rect;
    KText = mod.Text;
  });

  const s = $derived.by(() => {
    const defaults: Record<string, { fill: string; stroke: string; strokeW: number; textFill: string; defaultLabel: string }> = {
      stage: { fill: '#E5E7EB', stroke: '#000000', strokeW: 2, textFill: '#000000', defaultLabel: 'Stage' },
      dj_counter: { fill: '#E5E7EB', stroke: '#000000', strokeW: 1, textFill: '#000000', defaultLabel: 'DJ' },
      entrance: { fill: '#E5E7EB', stroke: '#000000', strokeW: 1, textFill: '#000000', defaultLabel: 'Entrance' },
      tv: { fill: '#E5E7EB', stroke: '#000000', strokeW: 1, textFill: '#000000', defaultLabel: 'TV' },
      walkway: { fill: '#E5E7EB', stroke: '#000000', strokeW: 1, textFill: '#000000', defaultLabel: '' },
      box: { fill: '#E5E7EB', stroke: '#000000', strokeW: 2, textFill: '#000000', defaultLabel: '' },
    };
    const d = defaults[element.type] ?? defaults.box;
    return {
      fill: element.color || d.fill,
      stroke: element.strokeColor || d.stroke,
      strokeW: d.strokeW,
      textFill: element.textColor || d.textFill,
      fillOpacity: element.opacity > 0 ? element.opacity : 1,
      label: element.name || d.defaultLabel,
    };
  });

  // Convert hex to rgba with alpha for fill-only opacity
  function hexToRgba(hex: string, alpha: number): string {
    if (!hex || hex === 'transparent') return 'transparent';
    const h = hex.replace('#', '');
    const r = parseInt(h.substring(0, 2), 16);
    const g = parseInt(h.substring(2, 4), 16);
    const b = parseInt(h.substring(4, 6), 16);
    return `rgba(${r},${g},${b},${alpha})`;
  }

  const fillColor = $derived(hexToRgba(s.fill, s.fillOpacity));
</script>

{#if Group && Rect && KText}
  <Group
    x={px}
    y={py}
    rotation={element.degree}
    draggable={mode === 'edit'}
    bind:this={groupEl}
    onclick={() => onselect?.()}
    ondragend={ondragend}
    ontransformend={ontransformend}
  >
    <Rect
      x={-w / 2}
      y={-h / 2}
      width={w}
      height={h}
      fill={fillColor}
      stroke={s.stroke}
      strokeWidth={s.strokeW}
      cornerRadius={4}
    />
    {#if s.label && element.type !== 'box'}
      <KText
        text={s.label}
        fontSize={Math.max(10, Math.min(w / 6, 14))}
        fill={s.textFill}
        fontStyle="bold"
        align="center"
        verticalAlign="middle"
        width={w}
        height={h}
        x={-w / 2}
        y={-h / 2}
        letterSpacing={element.type === 'stage' ? 2 : 0}
      />
    {/if}
    {#if s.label && element.type === 'box'}
      <KText
        text={s.label}
        fontSize={Math.max(9, Math.min(w / 8, 12))}
        fill={s.textFill}
        fontStyle="600"
        x={-w / 2 + 4}
        y={-h / 2 + 2}
      />
    {/if}
  </Group>
{/if}
