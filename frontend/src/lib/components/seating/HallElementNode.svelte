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
    return groupEl;
  }

  // Register Konva node with parent for transformer
  $effect(() => {
    if (groupEl) onrefready?.(groupEl);
  });

  onMount(async () => {
    const mod = await import('svelte-konva');
    Group = mod.Group;
    Rect = mod.Rect;
    KText = mod.Text;
  });

  const s = $derived.by(() => {
    const styleMap: Record<string, { fill: string; stroke: string; strokeW: number; textFill: string; label: string; dash?: number[] }> = {
      stage: { fill: '#7F1D1D', stroke: '#D4AF37', strokeW: 2, textFill: '#D4AF37', label: '✦ Stage ✦' },
      dj_counter: { fill: '#1F2937', stroke: '#4B5563', strokeW: 1, textFill: '#FFFFFF', label: element.label },
      entrance: { fill: '#E5E7EB', stroke: '#9CA3AF', strokeW: 1, textFill: '#6B7280', label: element.label || '▼ Entrance ▼' },
      tv: { fill: '#111827', stroke: '#374151', strokeW: 1, textFill: '#9CA3AF', label: 'TV' },
      walkway: { fill: dark ? '#374151' : '#6B7280', stroke: dark ? '#4B5563' : '#9CA3AF', strokeW: 1, textFill: 'transparent', label: '' },
      box: { fill: 'transparent', stroke: '#1F2937', strokeW: 2, textFill: dark ? '#D1D5DB' : '#374151', label: element.label },
    };
    return styleMap[element.type] ?? styleMap.box;
  });
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
      fill={element.type === 'stage' ? undefined : s.fill}
      fillRadialGradientStartPoint={element.type === 'stage' ? { x: 0, y: 0 } : undefined}
      fillRadialGradientStartRadius={element.type === 'stage' ? 0 : undefined}
      fillRadialGradientEndPoint={element.type === 'stage' ? { x: 0, y: 0 } : undefined}
      fillRadialGradientEndRadius={element.type === 'stage' ? Math.max(w, h) : undefined}
      fillRadialGradientColorStops={element.type === 'stage' ? [0, '#A11217', 1, '#7F1D1D'] : undefined}
      stroke={s.stroke}
      strokeWidth={s.strokeW}
      cornerRadius={element.type === 'stage' ? [0, 0, 8, 8] : element.type === 'entrance' ? [8, 8, 0, 0] : 4}
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
