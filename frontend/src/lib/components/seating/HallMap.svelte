<script lang="ts">
  import { onMount } from 'svelte';
  import BanquetTableComponent from './BanquetTable.svelte';
  import HallElementNode from './HallElementNode.svelte';
  import type { BanquetTable as BanquetTableType, HallElement, Guest } from '$lib/types';
  import { ZoomIn, ZoomOut, RotateCcw } from 'lucide-svelte';

  let {
    mode = 'view',
    tables = [],
    elements = [],
    hallWidth = 860,
    hallHeight = 1000,
    tableGuests: tableGuestsRaw = {},
    selectedTableId = null,
    highlightedTableId = null,
    dark = false,
    onTableClick,
    onSeatClick,
  }: {
    mode?: 'view' | 'edit';
    tables?: BanquetTableType[];
    elements?: HallElement[];
    hallWidth?: number;
    hallHeight?: number;
    tableGuests?: Record<string, Guest[]>;
    selectedTableId?: string | null;
    highlightedTableId?: string | null;
    dark?: boolean;
    onTableClick?: (id: string) => void;
    onSeatClick?: (tableId: string, seatNum: number, guest: Guest | null) => void;
  } = $props();

  let zoom = $state(1);
  let panX = $state(0);
  let panY = $state(0);
  let isDragging = $state(false);
  let dragStart = { x: 0, y: 0 };

  let containerEl = $state<HTMLElement | null>(null);
  let containerW = $state(800);
  let containerH = $state(600);

  // ponytail: 3 separate svelte-konva imports; consolidate via props if load time matters
  let KonvaStage: any = $state(null);
  let KLayer: any = $state(null);

  let viewScale = $derived(Math.min(containerW / hallWidth, containerH / hallHeight));

  onMount(() => {
    let wheelCleanup: (() => void) | null = null;
    let ro: ResizeObserver | null = null;

    if (containerEl) {
      ro = new ResizeObserver(entries => {
        for (const entry of entries) {
          containerW = entry.contentRect.width;
          containerH = entry.contentRect.height;
        }
      });
      ro.observe(containerEl);
    }

    (async () => {
      const mod = await import('svelte-konva');
      KonvaStage = mod.Stage;
      KLayer = mod.Layer;

      const canvas = containerEl?.querySelector('canvas');
      if (canvas) {
        const handler = (e: WheelEvent) => {
          e.preventDefault();
          const delta = e.deltaY > 0 ? -0.08 : 0.08;
          const newZoom = Math.max(0.3, Math.min(3, zoom + delta));
          const rect = canvas.getBoundingClientRect();
          const mx = e.clientX - rect.left;
          const my = e.clientY - rect.top;
          const scale = newZoom / zoom;
          panX = mx - scale * (mx - panX);
          panY = my - scale * (my - panY);
          zoom = newZoom;
        };
        canvas.addEventListener('wheel', handler, { passive: false });
        wheelCleanup = () => canvas.removeEventListener('wheel', handler);
      }
    })();

    return () => {
      ro?.disconnect();
      wheelCleanup?.();
    };
  });

  function handleMouseDown(e: MouseEvent) {
    if (e.button !== 0) return;
    isDragging = true;
    dragStart = { x: e.clientX - panX, y: e.clientY - panY };
  }

  function handleMouseMove(e: MouseEvent) {
    if (!isDragging) return;
    panX = e.clientX - dragStart.x;
    panY = e.clientY - dragStart.y;
  }

  function handleMouseUp() {
    isDragging = false;
  }

  function resetView() {
    zoom = 1;
    panX = 0;
    panY = 0;
  }

  let touchStart = { x: 0, y: 0 };
  function handleTouchStart(e: TouchEvent) {
    if (e.touches.length === 1) {
      isDragging = true;
      touchStart = { x: e.touches[0].clientX - panX, y: e.touches[0].clientY - panY };
    }
  }
  function handleTouchMove(e: TouchEvent) {
    if (!isDragging || e.touches.length !== 1) return;
    e.preventDefault();
    panX = e.touches[0].clientX - touchStart.x;
    panY = e.touches[0].clientY - touchStart.y;
  }
  function handleTouchEnd() {
    isDragging = false;
  }

  const stageW = $derived(containerW);
  const stageH = $derived(containerH);
</script>

<svelte:window onmousemove={handleMouseMove} onmouseup={handleMouseUp} />

<div
  bind:this={containerEl}
  class="relative flex-1 overflow-hidden select-none min-h-[300px] {dark ? 'bg-gray-950' : 'bg-gray-50'}"
  class:cursor-grab={!isDragging}
  class:cursor-grabbing={isDragging}
  onmousedown={handleMouseDown}
  ontouchstart={handleTouchStart}
  ontouchmove={handleTouchMove}
  ontouchend={handleTouchEnd}
  role="application"
  aria-label="Banquet hall seating map"
>
  <!-- Zoom controls -->
  <div class="absolute top-3 right-3 sm:top-4 sm:right-4 z-30 flex flex-col gap-1.5">
    <button onclick={() => zoom = Math.min(3, zoom + 0.15)} class="w-8 h-8 sm:w-9 sm:h-9 {dark ? 'bg-gray-800 border-gray-700 hover:bg-gray-700 text-gray-300' : 'bg-white border-gray-200 hover:bg-gray-50 text-gray-600'} border rounded-lg shadow-sm flex items-center justify-center transition-colors" aria-label="Zoom in">
      <ZoomIn class="w-4 h-4" />
    </button>
    <button onclick={() => zoom = Math.max(0.3, zoom - 0.15)} class="w-8 h-8 sm:w-9 sm:h-9 {dark ? 'bg-gray-800 border-gray-700 hover:bg-gray-700 text-gray-300' : 'bg-white border-gray-200 hover:bg-gray-50 text-gray-600'} border rounded-lg shadow-sm flex items-center justify-center transition-colors" aria-label="Zoom out">
      <ZoomOut class="w-4 h-4" />
    </button>
    <button onclick={resetView} class="w-8 h-8 sm:w-9 sm:h-9 {dark ? 'bg-gray-800 border-gray-700 hover:bg-gray-700 text-gray-300' : 'bg-white border-gray-200 hover:bg-gray-50 text-gray-600'} border rounded-lg shadow-sm flex items-center justify-center transition-colors" aria-label="Reset view">
      <RotateCcw class="w-4 h-4" />
    </button>
    <div class="text-center text-[10px] {dark ? 'text-gray-500' : 'text-gray-400'} font-medium mt-0.5">{Math.round(zoom * 100)}%</div>
  </div>

  <!-- Legend -->
  <div class="absolute bottom-3 left-3 sm:bottom-4 sm:left-4 z-30 {dark ? 'bg-gray-800/90 border-gray-700' : 'bg-white/90 border-gray-200'} backdrop-blur-sm border rounded-xl px-3 sm:px-4 py-2 sm:py-3 text-[10px] sm:text-xs space-y-1 sm:space-y-1.5">
    <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 sm:w-3 sm:h-3 rounded-full border-2 {dark ? 'border-gray-600 bg-gray-700' : 'border-gray-200 bg-gray-100'}"></span> Empty</div>
    <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 sm:w-3 sm:h-3 rounded-full border-2 border-red {dark ? 'bg-red-900/40' : 'bg-red-50'}"></span> Occupied</div>
    <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 sm:w-3 sm:h-3 rounded-full border-2 border-emerald-500 {dark ? 'bg-emerald-900/40' : 'bg-emerald-50'}"></span> Checked In</div>
    <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 sm:w-3 sm:h-3 rounded-full border-2 border-gold {dark ? 'bg-gold/20' : 'bg-gold-50'}"></span> VIP</div>
  </div>

  {#if KonvaStage && KLayer}
    <KonvaStage
      width={stageW}
      height={stageH}
      scaleX={viewScale * zoom}
      scaleY={viewScale * zoom}
      x={panX}
      y={panY}
    >
      <KLayer>
        {#each elements as el (el.id)}
          <HallElementNode element={el} {hallWidth} {hallHeight} {dark} />
        {/each}
        {#each tables as t (t.id)}
          <BanquetTableComponent
            table={t}
            guests={tableGuestsRaw[t.id] ?? []}
            isSelected={selectedTableId === t.id}
            isHighlighted={highlightedTableId === t.id}
            {dark}
            {hallWidth}
            {hallHeight}
            {mode}
            onTableClick={() => onTableClick?.(t.id)}
            onSeatClick={(seatNum, guest) => onSeatClick?.(t.id, seatNum, guest)}
          />
        {/each}
      </KLayer>
    </KonvaStage>
  {/if}
</div>
