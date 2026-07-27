<script lang="ts">
  import { onMount } from 'svelte';
  import BanquetTableComponent from './BanquetTable.svelte';
  import HallElementNode from './HallElementNode.svelte';
  import EditToolbar from './EditToolbar.svelte';
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
    legendPosition = 'bottom-left',
    onTableClick,
    onSeatClick,
    onSaveLayout,
    onCancelEdit,
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
    legendPosition?: 'bottom-left' | 'top-left';
    onTableClick?: (id: string) => void;
    onSeatClick?: (tableId: string, seatNum: number, guest: Guest | null) => void;
    onSaveLayout?: (tables: BanquetTableType[], elements: HallElement[], hallWidth: number, hallHeight: number) => Promise<void>;
    onCancelEdit?: () => void;
  } = $props();

  let zoom = $state(1);
  let panX = $state(0);
  let panY = $state(0);

  let containerEl = $state<HTMLElement | null>(null);
  let containerW = $state(800);
  let containerH = $state(600);

  // ponytail: 3 separate svelte-konva imports; consolidate via props if load time matters
  let KonvaStage: any = $state(null);
  let KLayer: any = $state(null);
  let KTransformer: any = $state(null);
  let KRect: any = $state(null);
  let KGroup: any = $state(null);
  let KLine: any = $state(null);
  let konvaLoaded = $state(false);
  let stageRef = $state<any>(null);

  // Edit state
  let editTables = $state<BanquetTableType[]>([]);
  let editElements = $state<HallElement[]>([]);
  let editHallWidth = $state(0);
  let editHallHeight = $state(0);
  let selectedId = $state<string | null>(null);
  let transformerEl = $state<any>(null);
  let nodeRefs = $state<Record<string, any>>({});

  $effect(() => {
    if (mode === 'edit') {
      editTables = JSON.parse(JSON.stringify(tables));
      editElements = JSON.parse(JSON.stringify(elements));
      editHallWidth = hallWidth;
      editHallHeight = hallHeight;
      selectedId = null;
    } else {
      selectedId = null;
    }
  });

  // Attach transformer to selected node
  $effect(() => {
    const _ = selectedId;
    if (transformerEl) {
      const konvaTransformer = transformerEl.getNode?.() ?? transformerEl;
      if (konvaTransformer?.nodes) {
        if (selectedId) {
          const node = nodeRefs[selectedId];
          konvaTransformer.nodes(node ? [node] : []);
        } else {
          konvaTransformer.nodes([]);
        }
        konvaTransformer.getLayer()?.batchDraw();
      }
    }
  });

  // Expose nodeRef setter for child components
  function setNodeRef(id: string, node: any) {
    if (node) nodeRefs[id] = node;
  }

  const displayTables = $derived(mode === 'edit' ? editTables : tables);
  const displayElements = $derived(mode === 'edit' ? editElements : elements);
  const displayHallWidth = $derived(mode === 'edit' ? editHallWidth : hallWidth);
  const displayHallHeight = $derived(mode === 'edit' ? editHallHeight : hallHeight);

  let viewScale = $derived(Math.min(containerW / displayHallWidth, containerH / displayHallHeight));

  const isTableSelected = $derived(
    mode === 'edit' && selectedId !== null && editTables.some(t => t.id === selectedId)
  );

  const selectedItem = $derived.by(() => {
    if (!selectedId) return null;
    const t = editTables.find(t => t.id === selectedId);
    if (t) return { kind: 'table' as const, ...t };
    const el = editElements.find(e => e.id === selectedId);
    if (el) return { kind: 'element' as const, ...el };
    return null;
  });

  onMount(() => {
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
      KTransformer = mod.Transformer;
      KRect = mod.Rect;
      KGroup = mod.Group;
      KLine = mod.Line;
      konvaLoaded = true;

      const attachHandlers = () => {
        const canvas = containerEl?.querySelector('canvas');
        if (!canvas) { requestAnimationFrame(attachHandlers); return; }

        // Wheel zoom (desktop)
        canvas.addEventListener('wheel', (e: WheelEvent) => {
          e.preventDefault();
          const delta = e.deltaY > 0 ? -0.08 : 0.08;
          zoom = Math.max(0.3, Math.min(3, zoom + delta));
        }, { passive: false });

        // Mobile: touch-based pan + tap detection + pinch zoom
        // These handlers fire BEFORE Konva's internal handlers and take over
        // touch processing entirely on mobile, so we must handle everything.
        const isMobile = 'ontouchstart' in window;
        if (isMobile) {
          let touchStartX = 0;
          let touchStartY = 0;
          let isTouchPanning = false;
          let pinchStartDist = 0;
          let pinchStartZoom = 1;

          canvas.addEventListener('touchstart', (e: TouchEvent) => {
            if (e.touches.length === 1) {
              touchStartX = e.touches[0].clientX;
              touchStartY = e.touches[0].clientY;
              isTouchPanning = false;
            } else if (e.touches.length === 2) {
              isTouchPanning = false;
              const dx = e.touches[0].clientX - e.touches[1].clientX;
              const dy = e.touches[0].clientY - e.touches[1].clientY;
              pinchStartDist = Math.sqrt(dx * dx + dy * dy);
              pinchStartZoom = zoom;
            }
          }, { passive: true });

          canvas.addEventListener('touchmove', (e: TouchEvent) => {
            if (e.touches.length === 2) {
              e.preventDefault();
              const dx = e.touches[0].clientX - e.touches[1].clientX;
              const dy = e.touches[0].clientY - e.touches[1].clientY;
              const dist = Math.sqrt(dx * dx + dy * dy);
              if (pinchStartDist > 0) {
                zoom = Math.max(0.3, Math.min(3, pinchStartZoom * (dist / pinchStartDist)));
              }
            } else if (e.touches.length === 1) {
              const dx = e.touches[0].clientX - touchStartX;
              const dy = e.touches[0].clientY - touchStartY;
              if (!isTouchPanning && (Math.abs(dx) > 8 || Math.abs(dy) > 8)) {
                isTouchPanning = true;
              }
              if (isTouchPanning) {
                e.preventDefault();
                panX += dx;
                panY += dy;
                touchStartX = e.touches[0].clientX;
                touchStartY = e.touches[0].clientY;
              }
            }
          }, { passive: false });

          canvas.addEventListener('touchend', () => {
            pinchStartDist = 0;
          }, { passive: true });
        }
      };
      requestAnimationFrame(attachHandlers);
    })();

    return () => {
      ro?.disconnect();
    };
  });

  function handleStageClick(e: any) {
    if (mode !== 'edit') return;
    if (e.target === e.target.getStage()) {
      selectedId = null;
    }
  }

  function resetView() {
    zoom = 1;
    panX = 0;
    panY = 0;
  }

  // Grid snap: snap to nearest 2.5% increment
  const GRID_STEP = 2.5;
  function snapGrid(v: number): number {
    return Math.round(v / GRID_STEP) * GRID_STEP;
  }

  function handleDragEnd(id: string, e: any) {
    const rawX = e.target.x() / displayHallWidth * 100;
    const rawY = e.target.y() / displayHallHeight * 100;
    const x = Math.max(0, Math.min(100, snapGrid(rawX)));
    const y = Math.max(0, Math.min(100, snapGrid(rawY)));
    // Move node to snapped position
    e.target.x(x / 100 * displayHallWidth);
    e.target.y(y / 100 * displayHallHeight);
    const idx = editTables.findIndex(t => t.id === id);
    if (idx >= 0) {
      editTables[idx] = { ...editTables[idx], x, y };
      return;
    }
    const eidx = editElements.findIndex(el => el.id === id);
    if (eidx >= 0) {
      editElements[eidx] = { ...editElements[eidx], x, y };
    }
  }

  function handleTransformEnd(id: string, e: any) {
    const node = e.target;
    const rotation = node.rotation();
    const idx = editTables.findIndex(t => t.id === id);
    if (idx >= 0) {
      editTables[idx] = { ...editTables[idx], degree: rotation };
      return;
    }
    const eidx = editElements.findIndex(el => el.id === id);
    if (eidx >= 0) {
      const el = editElements[eidx];
      const baseW = el.width / 100 * displayHallWidth;
      const baseH = el.height / 100 * displayHallHeight;
      const newW = (node.scaleX() * baseW) / displayHallWidth * 100;
      const newH = (node.scaleY() * baseH) / displayHallHeight * 100;
      node.scaleX(1);
      node.scaleY(1);
      editElements[eidx] = { ...el, degree: rotation, width: newW, height: newH };
    }
  }

  function handleAddElement(el: HallElement) {
    editElements = [...editElements, el];
  }

  function handleUpdateSelected(props: Record<string, any>) {
    if (!selectedId) return;
    const tidx = editTables.findIndex(t => t.id === selectedId);
    if (tidx >= 0) {
      editTables[tidx] = { ...editTables[tidx], ...props };
      return;
    }
    const eidx = editElements.findIndex(el => el.id === selectedId);
    if (eidx >= 0) {
      editElements[eidx] = { ...editElements[eidx], ...props };
    }
  }

  function handleDeleteSelected(id: string) {
    editElements = editElements.filter(el => el.id !== id);
    selectedId = null;
  }

  async function handleSave() {
    await onSaveLayout?.(editTables, editElements, editHallWidth, editHallHeight);
  }

  function handleCancel() {
    onCancelEdit?.();
  }

  const stageW = $derived(containerW);
  const stageH = $derived(containerH);

  // Center the hall in the stage
  const contentW = $derived(displayHallWidth * viewScale);
  const contentH = $derived(displayHallHeight * viewScale);
  const offsetX = $derived((stageW - contentW) / 2);
  const offsetY = $derived((stageH - contentH) / 2);
</script>

{#if mode === 'edit'}
  <EditToolbar
    hallWidth={editHallWidth}
    hallHeight={editHallHeight}
    {selectedId}
    {isTableSelected}
    selectedItem={selectedItem}
    onSave={handleSave}
    onCancel={handleCancel}
    onDelete={handleDeleteSelected}
    onAddElement={handleAddElement}
    onUpdateSelected={handleUpdateSelected}
    onWidthChange={(w) => editHallWidth = w}
    onHeightChange={(h) => editHallHeight = h}
  />
{/if}

<div
  bind:this={containerEl}
  class="relative flex-1 z-10 overflow-hidden select-none min-h-[300px] {dark ? 'bg-gray-950' : 'bg-gray-50'}"
  style="touch-action: none;"
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

  <!-- Legend (view mode only) -->
  {#if mode !== 'edit'}
    <div class="absolute {legendPosition === 'top-left' ? 'top-3 left-3' : 'bottom-3 left-3'} z-30 {dark ? 'bg-gray-800/90 border-gray-700' : 'bg-white/90 border-gray-200'} backdrop-blur-sm border rounded-lg px-2 py-1.5 text-[9px] sm:text-[10px] flex flex-wrap gap-x-3 gap-y-0.5">
      <span class="inline-flex items-center gap-1"><span class="w-2 h-2 rounded-full border {dark ? 'border-gray-600 bg-gray-700' : 'border-gray-200 bg-gray-100'}"></span>Empty</span>
      <span class="inline-flex items-center gap-1"><span class="w-2 h-2 rounded-full border border-red {dark ? 'bg-red-900/40' : 'bg-red-50'}"></span>Occupied</span>
      <span class="inline-flex items-center gap-1"><span class="w-2 h-2 rounded-full border border-emerald-500 {dark ? 'bg-emerald-900/40' : 'bg-emerald-50'}"></span>Checked In</span>
      <span class="inline-flex items-center gap-1"><span class="w-2 h-2 rounded-full border border-gold {dark ? 'bg-gold/20' : 'bg-gold-50'}"></span>VIP</span>
    </div>
  {/if}

  {#if KonvaStage && KLayer && KRect}
    <KonvaStage
      bind:this={stageRef}
      width={stageW}
      height={stageH}
      x={panX}
      y={panY}
      draggable={mode !== 'edit'}
      onDragEnd={(e: any) => { if (mode !== 'edit') { panX = e.target.x(); panY = e.target.y(); } }}
      onclick={handleStageClick}
    >
      <KLayer>
        <KGroup x={offsetX} y={offsetY} scaleX={viewScale * zoom} scaleY={viewScale * zoom}>
          <!-- Hall boundary border -->
          <KRect
          x={0}
          y={0}
          width={displayHallWidth}
          height={displayHallHeight}
          fill={dark ? '#111827' : '#FFFFFF'}
          stroke={dark ? '#4B5563' : '#D1D5DB'}
          strokeWidth={2}
          cornerRadius={12}
          shadowColor={dark ? 'rgba(0,0,0,0.5)' : 'rgba(0,0,0,0.08)'}
          shadowBlur={20}
          shadowOffsetY={4}
        />
        <!-- Grid lines (edit mode only, on top of hall rect) -->
        {#if mode === 'edit' && KLine}
          {#each Array(Math.floor(100 / GRID_STEP) + 1) as _, i (i)}
            <KLine
              points={[i * GRID_STEP / 100 * displayHallWidth, 0, i * GRID_STEP / 100 * displayHallWidth, displayHallHeight]}
              stroke={dark ? '#4B5563' : '#D1D5DB'}
              strokeWidth={0.5}
              dash={[4, 4]}
              listening={false}
            />
            <KLine
              points={[0, i * GRID_STEP / 100 * displayHallHeight, displayHallWidth, i * GRID_STEP / 100 * displayHallHeight]}
              stroke={dark ? '#4B5563' : '#D1D5DB'}
              strokeWidth={0.5}
              dash={[4, 4]}
              listening={false}
            />
          {/each}
        {/if}
        {#each displayElements as el (el.id)}
          <HallElementNode
            element={el}
            hallWidth={displayHallWidth}
            hallHeight={displayHallHeight}
            {dark}
            mode={mode}
            onrefready={(node: any) => { nodeRefs[el.id] = node; }}
            ondragend={(e: any) => handleDragEnd(el.id, e)}
            ontransformend={(e: any) => handleTransformEnd(el.id, e)}
            onselect={() => { if (mode === 'edit') selectedId = el.id; }}
          />
        {/each}
        {#each displayTables as t (t.id)}
          <BanquetTableComponent
            table={t}
            guests={tableGuestsRaw[t.id] ?? []}
            isSelected={selectedTableId === t.id}
            isHighlighted={highlightedTableId === t.id}
            {dark}
            hallWidth={displayHallWidth}
            hallHeight={displayHallHeight}
            {mode}
            onrefready={(node: any) => { nodeRefs[t.id] = node; }}
            onTableClick={mode === 'edit' ? () => { selectedId = t.id; } : () => onTableClick?.(t.id)}
            onSeatClick={mode === 'edit' ? undefined : (seatNum, guest) => onSeatClick?.(t.id, seatNum, guest)}
            ondragend={(e: any) => handleDragEnd(t.id, e)}
            ontransformend={(e: any) => handleTransformEnd(t.id, e)}
          />
        {/each}
        {#if KTransformer && mode === 'edit'}
          <KTransformer
            bind:this={transformerEl}
            rotateEnabled={true}
            enabledAnchors={isTableSelected ? [] : undefined}
            anchorSize={8}
            anchorCornerRadius={2}
            borderStroke="#D4AF37"
            anchorStroke="#D4AF37"
            anchorFill="#FFFFFF"
            anchorStrokeWidth={2}
            borderStrokeWidth={1.5}
            padding={4}
          />
        {/if}
        </KGroup>
      </KLayer>
    </KonvaStage>
  {/if}
</div>
