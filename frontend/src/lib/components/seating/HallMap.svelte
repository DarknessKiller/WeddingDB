<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte';
  import { beforeNavigate, goto } from '$app/navigation';
  import BanquetTableComponent from './BanquetTable.svelte';
  import HallElementNode from './HallElementNode.svelte';
  import EditToolbar from './EditToolbar.svelte';
  import ConfirmDialog from '$lib/components/ui/ConfirmDialog.svelte';
  import type { BanquetTable as BanquetTableType, HallElement, Guest } from '$lib/types';
  import { reorderElements } from '$lib/utils/layout';
  import ZoomIn from 'lucide-svelte/icons/zoom-in';
import ZoomOut from 'lucide-svelte/icons/zoom-out';
import RotateCcw from 'lucide-svelte/icons/rotate-ccw';

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

  type LayoutSnapshot = {
    tables: BanquetTableType[];
    elements: HallElement[];
    hallWidth: number;
    hallHeight: number;
  };
  const HISTORY_LIMIT = 50;
  let history = $state<LayoutSnapshot[]>([]);
  let historyIndex = $state(-1);
  let baseline = $state<LayoutSnapshot | null>(null);
  let dragStart = $state<{ id: string; x: number; y: number } | null>(null);
  let dragCancelled = $state(false);
  let groupStart = $state<Record<string, { x: number; y: number }> | null>(null);
  let panStart = $state<{ x: number; y: number } | null>(null);

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
  let selectedIds = $state<string[]>([]);
  // Property panel and transformer follow the most recently clicked item.
  const selectedId = $derived(selectedIds[selectedIds.length - 1] ?? null);
  let snapEnabled = $state(true);
  let transformerEl = $state<any>(null);
  let nodeRefs = $state<Record<string, any>>({});

  function getTransformerNode() {
    return transformerEl?.node ?? transformerEl?.getNode?.() ?? transformerEl;
  }

  function refreshTransformer() {
    const transformer = getTransformerNode();
    if (!transformer?.nodes) return;
    transformer.forceUpdate?.();
    transformer.getLayer()?.batchDraw();
  }

  async function refreshTransformerAfterUpdate() {
    await tick();
    refreshTransformer();
  }

  const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value));

  function currentSnapshot(): LayoutSnapshot {
    return {
      tables: clone(editTables),
      elements: clone(editElements),
      hallWidth: editHallWidth,
      hallHeight: editHallHeight,
    };
  }

  // Takes explicit values: reading edit state here would make the mode effect
  // depend on it and reset the layout on every edit.
  function resetHistory(snapshot: LayoutSnapshot) {
    history = [snapshot];
    historyIndex = 0;
    baseline = snapshot;
  }

  function commit() {
    if (historyIndex >= 0 && JSON.stringify(currentSnapshot()) === JSON.stringify(history[historyIndex])) return;
    history = [...history.slice(0, historyIndex + 1), currentSnapshot()].slice(-HISTORY_LIMIT);
    historyIndex = history.length - 1;
  }

  function restore(snapshot: LayoutSnapshot) {
    editTables = clone(snapshot.tables);
    editElements = clone(snapshot.elements);
    editHallWidth = snapshot.hallWidth;
    editHallHeight = snapshot.hallHeight;
    const keep = selectedIds.filter(id => editTables.some(t => t.id === id) || editElements.some(el => el.id === id));
    if (keep.length !== selectedIds.length) selectedIds = keep;
    void refreshTransformerAfterUpdate();
  }

  function undo() {
    if (historyIndex <= 0) return;
    historyIndex--;
    restore(history[historyIndex]);
  }

  function redo() {
    if (historyIndex >= history.length - 1) return;
    historyIndex++;
    restore(history[historyIndex]);
  }

  const canUndo = $derived(historyIndex > 0);
  const canRedo = $derived(historyIndex < history.length - 1);
  const dirty = $derived.by(() => {
    if (mode !== 'edit' || !baseline) return false;
    return JSON.stringify(currentSnapshot()) !== JSON.stringify(baseline);
  });
  // Leaving edit mode or the page with unsaved edits asks first.
  let discardOpen = $state(false);
  let pendingAction: (() => void) | null = null;
  let skipGuard = false;

  function guard(proceed: () => void) {
    if (!dirty) { proceed(); return; }
    pendingAction = proceed;
    discardOpen = true;
  }

  beforeNavigate(nav => {
    if (!dirty || skipGuard) return;
    const href = nav.to?.url.href;
    if (!href) return;
    nav.cancel();
    pendingAction = () => { skipGuard = true; void goto(href); };
    discardOpen = true;
  });

  $effect(() => {
    if (mode === 'edit') {
      editTables = JSON.parse(JSON.stringify(tables));
      editElements = JSON.parse(JSON.stringify(elements));
      editHallWidth = hallWidth;
      editHallHeight = hallHeight;
      selectedIds = [];
      resetHistory(clone({ tables, elements, hallWidth, hallHeight }));
    } else {
      selectedIds = [];
      history = [];
      historyIndex = -1;
      baseline = null;
    }
  });

  // Attach transformer to selected node
  $effect(() => {
    const _ = selectedId;
    if (transformerEl) {
      const konvaTransformer = getTransformerNode();
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

  // Reload/close: beforeNavigate only sees in-app navigation.
  onMount(() => {
    const warn = (e: BeforeUnloadEvent) => { if (dirty) e.preventDefault(); };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  });

  onMount(() => {
    let ro: ResizeObserver | null = null;
    const canvasListeners: Array<[string, EventListener, any]> = [];

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

        // ponytail: store listeners for cleanup on destroy
        const onWheel = (e: Event) => {
          e.preventDefault();
          const we = e as WheelEvent;
          const rect = canvas.getBoundingClientRect();
          const delta = we.deltaY > 0 ? -0.08 : 0.08;
          zoomAt(zoom + delta, we.clientX - rect.left, we.clientY - rect.top);
        };
        canvas.addEventListener('wheel', onWheel, { passive: false });
        canvasListeners.push(['wheel', onWheel as EventListener, { passive: false }]);

        // Mobile: touch-based pan + tap detection + pinch zoom
        // These handlers fire BEFORE Konva's internal handlers and take over
        // touch processing entirely on mobile, so we must handle everything.
        const isMobile = 'ontouchstart' in window;
        if (isMobile) {
          let touchStartX = 0;
          let touchStartY = 0;
          let touchStartTime = 0;
          let isTouchPanning = false;
          let pinchStartDist = 0;
          let pinchStartZoom = 1;

          canvas.addEventListener('touchstart', (e: TouchEvent) => {
            if (e.touches.length === 1) {
              touchStartX = e.touches[0].clientX;
              touchStartY = e.touches[0].clientY;
              touchStartTime = performance.now();
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

          canvas.addEventListener('touchend', (e: TouchEvent) => {
            const dx = (e.changedTouches[0]?.clientX ?? touchStartX) - touchStartX;
            const dy = (e.changedTouches[0]?.clientY ?? touchStartY) - touchStartY;
            const dist = Math.sqrt(dx * dx + dy * dy);
            const elapsed = performance.now() - touchStartTime;
            // Tap: didn't move much and was quick
            if (!isTouchPanning && dist < 12 && elapsed < 300) {
              const touch = e.changedTouches[0];
              if (touch) {
                const rect = canvas.getBoundingClientRect();
                const tapX = touch.clientX - rect.left;
                const tapY = touch.clientY - rect.top;
                // Hit-test against computed table screen positions
                let hitTableId = '';
                for (const t of displayTables) {
                  const screenX = panX + offsetX + (t.x / 100) * displayHallWidth * viewScale * zoom;
                  const screenY = panY + offsetY + (t.y / 100) * displayHallHeight * viewScale * zoom;
                  const ddx = tapX - screenX;
                  const ddy = tapY - screenY;
                  // Hit radius: table visual radius (~34px) + seat orbit (~20px) + padding
                  const hitRadius = 60 * viewScale * zoom;
                  if (ddx * ddx + ddy * ddy < hitRadius * hitRadius) {
                    hitTableId = String(t.id);
                    break;
                  }
                }
                onTableClick?.(hitTableId);
              }
            }
            pinchStartDist = 0;
          }, { passive: true });
        }
      };
      requestAnimationFrame(attachHandlers);
    })();

      return () => {
      ro?.disconnect();
      // Clean up canvas event listeners
      const canvas = containerEl?.querySelector('canvas');
      if (canvas) {
        for (const [event, handler, opts] of canvasListeners) {
          canvas.removeEventListener(event, handler, opts);
        }
      }
    };
  });

  function handleStageClick(e: any) {
    if (mode !== 'edit') return;
    // A pan ends in a click too; only a real click on empty hall clears the selection.
    const moved = panStart && (Math.abs(panX - panStart.x) > 0.5 || Math.abs(panY - panStart.y) > 0.5);
    panStart = null;
    if (!moved && e.target === e.target.getStage()) selectedIds = [];
  }

  function resetView() {
    zoom = 1;
    panX = 0;
    panY = 0;
  }

  // Zooms about a point in stage coordinates, so whatever sits under it stays put.
  function zoomAt(next: number, mx: number, my: number) {
    const z = Math.max(0.3, Math.min(3, next));
    if (z === zoom) return;
    const ratio = z / zoom;
    panX = mx - offsetX - (mx - panX - offsetX) * ratio;
    panY = my - offsetY - (my - panY - offsetY) * ratio;
    zoom = z;
  }

  // Grid snap: snap to nearest 2.5% increment (toggleable)
  const GRID_STEP = 2.5;
  function snapGrid(v: number): number {
    if (!snapEnabled) return v;
    return Math.round(v / GRID_STEP) * GRID_STEP;
  }

  function handleDragStart(id: string, e: any) {
    dragStart = { id, x: e.target.x(), y: e.target.y() };
    dragCancelled = false;
    // Dragging any member of a multi-selection moves the whole group.
    groupStart = selectedIds.length > 1 && selectedIds.includes(id)
      ? Object.fromEntries(selectedIds.map(sid => [sid, { x: nodeRefs[sid]?.x() ?? 0, y: nodeRefs[sid]?.y() ?? 0 }]))
      : null;
  }

  function handleDragMove(e: any) {
    if (!dragStart || !groupStart) return;
    const dx = e.target.x() - dragStart.x;
    const dy = e.target.y() - dragStart.y;
    for (const [sid, start] of Object.entries(groupStart)) {
      if (sid === dragStart.id) continue;
      const node = nodeRefs[sid];
      if (!node) continue;
      node.x(start.x + dx);
      node.y(start.y + dy);
    }
    e.target.getLayer()?.batchDraw();
  }

  // Click select: shift toggles membership, a click inside an existing
  // multi-selection keeps it so the group can be dragged.
  function selectItem(id: string, e?: any) {
    if (e?.evt?.shiftKey) {
      selectedIds = selectedIds.includes(id) ? selectedIds.filter(x => x !== id) : [...selectedIds, id];
      return;
    }
    if (selectedIds.includes(id) && selectedIds.length > 1) {
      selectedIds = [...selectedIds.filter(x => x !== id), id];
      return;
    }
    selectedIds = [id];
  }

  // Table footprint in hall pixels: BanquetTable's circle plus its seat orbit.
  const TABLE_FOOTPRINT_PX = 116;
  type AlignOp = 'left' | 'centerX' | 'right' | 'top' | 'middle' | 'bottom' | 'distX' | 'distY';

  function alignSelection(op: AlignOp) {
    if (selectedIds.length < 2) return;
    const items = selectedIds.flatMap(id => {
      const t = editTables.find(x => x.id === id);
      if (t) return [{ id, wPx: TABLE_FOOTPRINT_PX, hPx: TABLE_FOOTPRINT_PX, xPx: t.x / 100 * displayHallWidth, yPx: t.y / 100 * displayHallHeight }];
      const el = editElements.find(x => x.id === id);
      return el ? [{ id, wPx: el.width / 100 * displayHallWidth, hPx: el.height / 100 * displayHallHeight, xPx: el.x / 100 * displayHallWidth, yPx: el.y / 100 * displayHallHeight }] : [];
    });
    if (items.length < 2) return;
    const setPos = (id: string, xPx: number, yPx: number) => {
      const x = Math.max(0, Math.min(100, xPx / displayHallWidth * 100));
      const y = Math.max(0, Math.min(100, yPx / displayHallHeight * 100));
      const tidx = editTables.findIndex(t => t.id === id);
      if (tidx >= 0) { editTables[tidx] = { ...editTables[tidx], x, y }; return; }
      const eidx = editElements.findIndex(el => el.id === id);
      if (eidx >= 0) editElements[eidx] = { ...editElements[eidx], x, y };
    };
    const left = Math.min(...items.map(i => i.xPx - i.wPx / 2));
    const right = Math.max(...items.map(i => i.xPx + i.wPx / 2));
    const top = Math.min(...items.map(i => i.yPx - i.hPx / 2));
    const bottom = Math.max(...items.map(i => i.yPx + i.hPx / 2));
    const spread = (axis: 'x' | 'y') => {
      const key = axis === 'x' ? 'xPx' : 'yPx';
      const sorted = [...items].sort((a, b) => a[key] - b[key]);
      const first = sorted[0][key];
      const last = sorted[sorted.length - 1][key];
      sorted.forEach((it, i) => {
        const next = first + (last - first) * i / (sorted.length - 1);
        setPos(it.id, axis === 'x' ? next : it.xPx, axis === 'y' ? next : it.yPx);
      });
    };
    switch (op) {
      case 'left': items.forEach(i => setPos(i.id, left + i.wPx / 2, i.yPx)); break;
      case 'right': items.forEach(i => setPos(i.id, right - i.wPx / 2, i.yPx)); break;
      case 'centerX': items.forEach(i => setPos(i.id, (left + right) / 2, i.yPx)); break;
      case 'top': items.forEach(i => setPos(i.id, i.xPx, top + i.hPx / 2)); break;
      case 'bottom': items.forEach(i => setPos(i.id, i.xPx, bottom - i.hPx / 2)); break;
      case 'middle': items.forEach(i => setPos(i.id, i.xPx, (top + bottom) / 2)); break;
      case 'distX': spread('x'); break;
      case 'distY': spread('y'); break;
    }
    commit();
    void refreshTransformerAfterUpdate();
  }

  function cancelDrag() {
    if (!dragStart) return;
    // stopDrag fires dragend synchronously, which clears dragStart.
    const start = dragStart;
    const group = groupStart;
    dragCancelled = true;
    const node = nodeRefs[start.id];
    node?.stopDrag?.();
    if (group) {
      for (const [sid, pos] of Object.entries(group)) {
        const peer = nodeRefs[sid];
        if (!peer) continue;
        peer.x(pos.x);
        peer.y(pos.y);
      }
    }
    if (node) {
      node.x(start.x);
      node.y(start.y);
      node.getLayer()?.batchDraw();
    }
    dragStart = null;
    groupStart = null;
  }

  function handleDragEnd(id: string, e: any) {
    if (dragCancelled) {
      dragCancelled = false;
      dragStart = null;
      groupStart = null;
      return;
    }
    dragStart = null;
    const group = groupStart;
    groupStart = null;
    const rawX = e.target.x() / displayHallWidth * 100;
    const rawY = e.target.y() / displayHallHeight * 100;
    const x = Math.max(0, Math.min(100, snapGrid(rawX)));
    const y = Math.max(0, Math.min(100, snapGrid(rawY)));
    // Move node to snapped position
    e.target.x(x / 100 * displayHallWidth);
    e.target.y(y / 100 * displayHallHeight);
    const moves: Array<[string, number, number]> = [[id, x, y]];
    if (group) {
      // Every follower takes the same snap correction, so spacing survives the drag.
      const dxPct = x - rawX;
      const dyPct = y - rawY;
      for (const sid of Object.keys(group)) {
        if (sid === id) continue;
        const node = nodeRefs[sid];
        if (!node) continue;
        const nx = Math.max(0, Math.min(100, node.x() / displayHallWidth * 100 + dxPct));
        const ny = Math.max(0, Math.min(100, node.y() / displayHallHeight * 100 + dyPct));
        node.x(nx / 100 * displayHallWidth);
        node.y(ny / 100 * displayHallHeight);
        moves.push([sid, nx, ny]);
      }
      e.target.getLayer()?.batchDraw();
    }
    let changed = false;
    for (const [sid, mx, my] of moves) {
      const idx = editTables.findIndex(t => t.id === sid);
      if (idx >= 0) { editTables[idx] = { ...editTables[idx], x: mx, y: my }; changed = true; continue; }
      const eidx = editElements.findIndex(el => el.id === sid);
      if (eidx >= 0) { editElements[eidx] = { ...editElements[eidx], x: mx, y: my }; changed = true; }
    }
    if (changed) commit();
  }

  function handleTransformEnd(id: string, e: any) {
    const node = e.target;
    const rotation = node.rotation();
    const idx = editTables.findIndex(t => t.id === id);
    if (idx >= 0) {
      editTables[idx] = { ...editTables[idx], degree: rotation };
      commit();
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
      commit();
      void refreshTransformerAfterUpdate();
    }
  }

  function handleAddElement(el: HallElement) {
    editElements = [...editElements, el];
    commit();
  }

  function handleUpdateSelected(props: Record<string, any>) {
    if (!selectedId) return;
    const tidx = editTables.findIndex(t => t.id === selectedId);
    if (tidx >= 0) {
      editTables[tidx] = { ...editTables[tidx], ...props };
      commit();
      void refreshTransformerAfterUpdate();
      return;
    }
    const eidx = editElements.findIndex(el => el.id === selectedId);
    if (eidx >= 0) {
      editElements[eidx] = { ...editElements[eidx], ...props };
      commit();
      void refreshTransformerAfterUpdate();
    }
  }

  function handleDeleteSelected(id: string) {
    const ids = selectedIds.includes(id) ? selectedIds : [id];
    editElements = editElements.filter(el => !ids.includes(el.id));
    selectedIds = selectedIds.filter(sid => !ids.includes(sid));
    commit();
  }

  function handleReorder(dir: 'front' | 'back') {
    if (!selectedId) return;
    const edge = editElements[dir === 'front' ? editElements.length - 1 : 0];
    const ordered = reorderElements(editElements, selectedId, dir);
    if (ordered === editElements) return;
    editElements = ordered;

    // Keyed Svelte blocks do not reorder the Konva nodes that svelte-konva added on mount.
    const node = nodeRefs[selectedId];
    const edgeNode = nodeRefs[edge.id];
    if (node && edgeNode && node !== edgeNode) {
      node.zIndex(edgeNode.zIndex());
      node.getLayer()?.batchDraw();
    }
    commit();
  }

  function nudgeSelected(dx: number, dy: number) {
    if (!selectedIds.length) return;
    // Snapping each member of a group separately would eat the spacing between them.
    const snap = selectedIds.length === 1 ? snapGrid : (v: number) => v;
    const clamp = (v: number) => Math.max(0, Math.min(100, snap(v)));
    let changed = false;
    for (const id of selectedIds) {
      const tidx = editTables.findIndex(t => t.id === id);
      if (tidx >= 0) {
        const t = editTables[tidx];
        editTables[tidx] = { ...t, x: clamp(t.x + dx), y: clamp(t.y + dy) };
        changed = true;
        continue;
      }
      const eidx = editElements.findIndex(el => el.id === id);
      if (eidx >= 0) {
        const el = editElements[eidx];
        editElements[eidx] = { ...el, x: clamp(el.x + dx), y: clamp(el.y + dy) };
        changed = true;
      }
    }
    if (changed) {
      commit();
      void refreshTransformerAfterUpdate();
    }
  }

  function isTypingTarget(target: EventTarget | null): boolean {
    const el = target as HTMLElement | null;
    if (!el) return false;
    return el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' ||
      el.tagName === 'SELECT' || el.isContentEditable;
  }

  function handleKeydown(e: KeyboardEvent) {
    if (mode !== 'edit' || isTypingTarget(e.target)) return;
    const mod = e.metaKey || e.ctrlKey;
    if (mod && e.key.toLowerCase() === 'z') {
      e.preventDefault();
      if (e.shiftKey) redo(); else undo();
      return;
    }
    if (mod && e.key.toLowerCase() === 'y') {
      e.preventDefault();
      redo();
      return;
    }
    if (e.key === 'Escape') {
      if (dragStart) cancelDrag();
      else selectedIds = [];
      return;
    }
    if (!selectedId) return;
    if (e.key === 'Delete' || e.key === 'Backspace') {
      // Tables are removed from the tables page, so only elements are deletable here.
      if (selectedIds.some(sid => editElements.some(el => el.id === sid))) {
        e.preventDefault();
        handleDeleteSelected(selectedId);
      }
      return;
    }
    const deltas: Record<string, [number, number]> = {
      ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1],
    };
    const delta = deltas[e.key];
    if (!delta) return;
    e.preventDefault();
    const step = e.shiftKey ? 10 : (snapEnabled ? GRID_STEP : 1);
    nudgeSelected(delta[0] * step, delta[1] * step);
  }

  async function handleSave() {
    await onSaveLayout?.(editTables, editElements, editHallWidth, editHallHeight);
  }

  function handleCancel() {
    guard(() => onCancelEdit?.());
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
    snap={snapEnabled}
    {canUndo}
    {canRedo}
    {dirty}
    onUndo={undo}
    onRedo={redo}
    onSave={handleSave}
    onCancel={handleCancel}
    onDelete={handleDeleteSelected}
    selectedCount={selectedIds.length}
    elementCount={selectedIds.filter(sid => editElements.some(el => el.id === sid)).length}
    onAlign={alignSelection}
    onAddElement={handleAddElement}
    onUpdateSelected={handleUpdateSelected}
    onWidthChange={(w) => { editHallWidth = w; commit(); }}
    onHeightChange={(h) => { editHallHeight = h; commit(); }}
    onSnapToggle={() => snapEnabled = !snapEnabled}
    onReorder={handleReorder}
  />
{/if}

<svelte:window onkeydown={handleKeydown} />

<div
  bind:this={containerEl}
  class="relative flex-1 z-10 overflow-hidden select-none {dark ? 'bg-gray-950' : ''}"
  style="touch-action: none;"
  role="application"
  aria-label="Banquet hall seating map"
>
  <!-- Zoom controls -->
  <div class="absolute top-3 right-3 sm:top-4 sm:right-4 z-40 flex flex-col gap-1.5" style="position: absolute !important;">
    <button onclick={() => zoomAt(zoom + 0.15, stageW / 2, stageH / 2)} class="w-8 h-8 sm:w-9 sm:h-9 {dark ? 'bg-gray-800 border-gray-700 hover:bg-gray-700 text-gray-300' : 'bg-white border-gray-200 hover:bg-gray-50 text-gray-600'} border rounded-lg shadow-sm flex items-center justify-center transition-colors" aria-label="Zoom in">
      <ZoomIn class="w-4 h-4" />
    </button>
    <button onclick={() => zoomAt(zoom - 0.15, stageW / 2, stageH / 2)} class="w-8 h-8 sm:w-9 sm:h-9 {dark ? 'bg-gray-800 border-gray-700 hover:bg-gray-700 text-gray-300' : 'bg-white border-gray-200 hover:bg-gray-50 text-gray-600'} border rounded-lg shadow-sm flex items-center justify-center transition-colors" aria-label="Zoom out">
      <ZoomOut class="w-4 h-4" />
    </button>
    <button onclick={resetView} class="w-8 h-8 sm:w-9 sm:h-9 {dark ? 'bg-gray-800 border-gray-700 hover:bg-gray-700 text-gray-300' : 'bg-white border-gray-200 hover:bg-gray-50 text-gray-600'} border rounded-lg shadow-sm flex items-center justify-center transition-colors" aria-label="Fit hall to view" title="Fit hall to view">
      <RotateCcw class="w-4 h-4" />
    </button>
    <div class="text-center text-[10px] {dark ? 'text-gray-500' : 'text-gray-400'} font-medium mt-0.5">{Math.round(zoom * 100)}%</div>
  </div>

  <!-- Legend (view mode only) -->
  {#if mode !== 'edit'}
    <div class="absolute {legendPosition === 'top-left' ? 'top-3 left-3' : 'bottom-3 left-3'} z-40 {dark ? 'bg-gray-800/90 border-gray-700' : 'bg-white/90 border-gray-200'} backdrop-blur-sm border rounded-lg px-2 py-1.5 text-[9px] sm:text-[10px] flex flex-wrap gap-x-3 gap-y-0.5">
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
      draggable
      onMouseDown={() => { panStart = { x: panX, y: panY }; }}
      onDragEnd={(e: any) => { panX = e.target.x(); panY = e.target.y(); }}
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
            isSelected={mode === 'edit' && selectedIds.includes(el.id)}
            onrefready={(node: any) => { nodeRefs[el.id] = node; }}
            ondragstart={(e: any) => handleDragStart(el.id, e)}
            ondragmove={handleDragMove}
            ondragend={(e: any) => handleDragEnd(el.id, e)}
            ontransformend={(e: any) => handleTransformEnd(el.id, e)}
            onselect={(e: any) => { if (mode === 'edit') selectItem(el.id, e); }}
          />
        {/each}
        {#each displayTables as t (t.id)}
          <BanquetTableComponent
            table={t}
            guests={tableGuestsRaw[t.id] ?? []}
            isSelected={mode === 'edit' ? selectedIds.includes(t.id) : selectedTableId === t.id}
            isHighlighted={highlightedTableId === t.id}
            {dark}
            hallWidth={displayHallWidth}
            hallHeight={displayHallHeight}
            {mode}
            onrefready={(node: any) => { nodeRefs[t.id] = node; }}
            onTableClick={mode === 'edit' ? (e: any) => selectItem(t.id, e) : () => onTableClick?.(t.id)}
            onSeatClick={mode === 'edit' ? undefined : (seatNum, guest) => onSeatClick?.(t.id, seatNum, guest)}
            ondragstart={(e: any) => handleDragStart(t.id, e)}
            ondragmove={handleDragMove}
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

<ConfirmDialog
  open={discardOpen}
  title="Discard unsaved changes?"
  message="This layout has edits that have not been saved."
  confirmLabel="Discard"
  cancelLabel="Keep editing"
  variant="danger"
  onConfirm={() => { const action = pendingAction; pendingAction = null; discardOpen = false; action?.(); }}
  onCancel={() => { pendingAction = null; discardOpen = false; }}
/>
