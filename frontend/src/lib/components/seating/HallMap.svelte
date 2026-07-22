<script lang="ts">
  import BanquetTable from './BanquetTable.svelte';
  import { TABLE_DEFINITIONS, HALL_LAYOUT } from '$lib/constants';
  import type { Guest } from '$lib/types';
  import { ZoomIn, ZoomOut, RotateCcw } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let {
    selectedTableId = null,
    highlightedTableId = null,
    kioskHighlightTableId = null,
    onTableClick,
    onSeatClick,
    hoveredSeat = $bindable(null),
  }: {
    selectedTableId?: number | null;
    highlightedTableId?: number | null;
    kioskHighlightTableId?: number | null;
    onTableClick?: (id: number) => void;
    onSeatClick?: (tableId: number, seatNum: number, guest: Guest | null) => void;
    hoveredSeat?: { seatNum: number; guest: Guest | null; x: number; y: number } | null;
  } = $props();

  let zoom = $state(1);
  let panX = $state(0);
  let panY = $state(0);
  let isDragging = $state(false);
  let dragStart = { x: 0, y: 0 };

  // Dynamic hall sizing
  let containerEl = $state<HTMLElement | null>(null);
  let containerW = $state(800);
  let containerH = $state(600);

  // Base design dimensions
  const BASE_W = HALL_LAYOUT.width;
  const BASE_H = HALL_LAYOUT.height;

  // Scale factor to fit container
  let scale = $derived(Math.min(containerW / BASE_W, containerH / BASE_H, 1));

  onMount(() => {
    if (!containerEl) return;
    const ro = new ResizeObserver(entries => {
      for (const entry of entries) {
        containerW = entry.contentRect.width;
        containerH = entry.contentRect.height;
      }
    });
    ro.observe(containerEl);
    return () => ro.disconnect();
  });

  function handleWheel(e: WheelEvent) {
    e.preventDefault();
    const delta = e.deltaY > 0 ? -0.08 : 0.08;
    zoom = Math.max(0.3, Math.min(3, zoom + delta));
  }

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

  // Touch support
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
</script>

<svelte:window onmousemove={handleMouseMove} onmouseup={handleMouseUp} />

<div
  bind:this={containerEl}
  class="relative flex-1 bg-gray-50 overflow-hidden select-none min-h-[300px]"
  class:cursor-grab={!isDragging}
  class:cursor-grabbing={isDragging}
  onwheel={handleWheel}
  onmousedown={handleMouseDown}
  ontouchstart={handleTouchStart}
  ontouchmove={handleTouchMove}
  ontouchend={handleTouchEnd}
  role="application"
  aria-label="Banquet hall seating map"
>
  <!-- Zoom controls -->
  <div class="absolute top-3 right-3 sm:top-4 sm:right-4 z-30 flex flex-col gap-1.5">
    <button onclick={() => zoom = Math.min(3, zoom + 0.15)} class="w-8 h-8 sm:w-9 sm:h-9 bg-white border border-gray-200 rounded-lg shadow-sm flex items-center justify-center hover:bg-gray-50 transition-colors" aria-label="Zoom in">
      <ZoomIn class="w-4 h-4 text-gray-600" />
    </button>
    <button onclick={() => zoom = Math.max(0.3, zoom - 0.15)} class="w-8 h-8 sm:w-9 sm:h-9 bg-white border border-gray-200 rounded-lg shadow-sm flex items-center justify-center hover:bg-gray-50 transition-colors" aria-label="Zoom out">
      <ZoomOut class="w-4 h-4 text-gray-600" />
    </button>
    <button onclick={resetView} class="w-8 h-8 sm:w-9 sm:h-9 bg-white border border-gray-200 rounded-lg shadow-sm flex items-center justify-center hover:bg-gray-50 transition-colors" aria-label="Reset view">
      <RotateCcw class="w-4 h-4 text-gray-600" />
    </button>
    <div class="text-center text-[10px] text-gray-400 font-medium mt-0.5">{Math.round(zoom * 100)}%</div>
  </div>

  <!-- Legend -->
  <div class="absolute bottom-3 left-3 sm:bottom-4 sm:left-4 z-30 bg-white/90 backdrop-blur-sm border border-gray-200 rounded-xl px-3 sm:px-4 py-2 sm:py-3 text-[10px] sm:text-xs space-y-1 sm:space-y-1.5">
    <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 sm:w-3 sm:h-3 rounded-full border-2 border-gray-200 bg-gray-100"></span> Empty</div>
    <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 sm:w-3 sm:h-3 rounded-full border-2 border-red bg-red-50"></span> Occupied</div>
    <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 sm:w-3 sm:h-3 rounded-full border-2 border-emerald-500 bg-emerald-50"></span> Checked In</div>
    <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 sm:w-3 sm:h-3 rounded-full border-2 border-gold bg-gold-50"></span> VIP</div>
  </div>

  <!-- Hall -->
  <div
    class="absolute inset-0 flex items-center justify-center"
    style="transform: translate({panX}px, {panY}px) scale({zoom}); transform-origin: center center; transition: {isDragging ? 'none' : 'transform 0.2s cubic-bezier(0.4,0,0.2,1)'};"
  >
    <div class="relative bg-white border-2 border-gray-200 rounded-3xl shadow-xl"
      style="width: {BASE_W * scale}px; height: {BASE_H * scale}px; min-width: {BASE_W * scale}px;">

      <!-- Stage -->
      <div class="absolute top-0 left-1/2 -translate-x-1/2 w-[55%] h-[6%] flex flex-col items-center justify-center z-10">
        <div class="absolute inset-0 bg-gradient-to-br from-red via-red-dark to-[#5C0A0C] rounded-b-2xl"></div>
        <div class="absolute inset-1 rounded-b-xl bg-gradient-to-b from-gold/15 to-transparent pointer-events-none"></div>
        <span class="relative z-10 text-gold text-[10px] sm:text-sm font-bold tracking-[0.12em] uppercase font-serif">✦ Stage ✦</span>
      </div>

      <!-- Main aisle (vertical) -->
      <div class="absolute top-[8%] bottom-[8%] left-1/2 -translate-x-1/2 w-px z-[1]"
        style="background: repeating-linear-gradient(180deg, transparent, transparent 8px, #E5E7EB 8px, #E5E7EB 10px);"></div>

      <!-- Side aisles -->
      <div class="absolute top-[8%] bottom-[8%] left-[30%] w-px z-[1] opacity-50"
        style="background: repeating-linear-gradient(180deg, transparent, transparent 8px, #F3F4F6 8px, #F3F4F6 10px);"></div>
      <div class="absolute top-[8%] bottom-[8%] left-[70%] w-px z-[1] opacity-50"
        style="background: repeating-linear-gradient(180deg, transparent, transparent 8px, #F3F4F6 8px, #F3F4F6 10px);"></div>

      <!-- Entrance -->
      <div class="absolute bottom-0 left-1/2 -translate-x-1/2 w-[14%] h-[4%] bg-gray-100 rounded-t-xl flex items-center justify-center z-10">
        <span class="text-[8px] sm:text-[10px] font-semibold text-gray-500 tracking-wide">▼ Entrance ▼</span>
      </div>

      <!-- Tables -->
      {#each TABLE_DEFINITIONS as tableDef (tableDef.id)}
        <BanquetTable
          table={tableDef}
          isSelected={selectedTableId === tableDef.id}
          isHighlighted={highlightedTableId === tableDef.id}
          isKioskHighlight={kioskHighlightTableId !== null}
          hallScale={scale}
          onTableClick={() => onTableClick?.(tableDef.id)}
          onSeatClick={(seatNum, guest) => onSeatClick?.(tableDef.id, seatNum, guest)}
          bind:hoveredSeat
        />
      {/each}
    </div>
  </div>
</div>
