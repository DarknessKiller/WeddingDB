<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { cn } from '$lib/utils';
  import { addToast, isDrawerOpen } from '$lib/stores';
  import { Star, Users, Plus, MoreVertical, Pencil, Trash2, X, AlertCircle, Map } from 'lucide-svelte';
  import { listTables, createTable, updateTable, deleteTable, getOccupancy } from '$lib/api/tables';
  import { getLayout, saveLayout } from '$lib/api/layout';
  import { defaultSlot } from '$lib/utils/layout';
  import { weddingId } from '$lib/stores/weddingId';
  import { get } from 'svelte/store';
  import type { BanquetTable, TableOccupancy, HallElement, HallLayoutData } from '$lib/types';

  type TableCreateData = Omit<BanquetTable, 'id'>;
  import HallMap from '$lib/components/seating/HallMap.svelte';

  const RING_R = 24;
  const RING_CIRCUM = 2 * Math.PI * RING_R;

  let tables = $state<BanquetTable[]>([]);
  let elements = $state<HallElement[]>([]);
  let hallWidth = $state(860);
  let hallHeight = $state(1000);
  let tablesError = $state<string | null>(null);
  let occupancy: Record<string, TableOccupancy> = $state.raw({});
  let loading = $state(true);
  let editMode = $state(false);

  let gridCols = $derived.by(() => {
    const n = (tables ?? []).length;
    if (n <= 2) return 'grid-cols-2';
    if (n <= 4) return 'grid-cols-2 md:grid-cols-3';
    if (n <= 6) return 'grid-cols-2 md:grid-cols-3 lg:grid-cols-4';
    return 'grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5';
  });

  // Context menu
  let contextMenu = $state<{ x: number; y: number; table: BanquetTable } | null>(null);
  const menuWidth = 180;
  const menuHeight = 120;

  // Modal state
  let showModal = $state(false);
  let editingTable = $state<BanquetTable | null>(null);
  let formName = $state('');
  let formCapacity = $state(10);
  let formVip = $state(false);
  let saving = $state(false);

  let prevDrawerOpen = $state(false);
  $effect(() => {
    const isOpen = $isDrawerOpen;
    if (prevDrawerOpen && !isOpen) {
      loadData();
    }
    prevDrawerOpen = isOpen;
  });

  async function loadData() {
    const [layout, rawOcc] = await Promise.all([getLayout(wid), getOccupancy(wid)]);
    tables = layout.tables ?? [];
    elements = layout.elements ?? [];
    hallWidth = layout.hallWidth ?? 860;
    hallHeight = layout.hallHeight ?? 1000;
    tablesError = null;
    const occMap: Record<string, TableOccupancy> = {};
    for (const o of rawOcc) {
      const table = tables.find(t => t.id === o.TableID);
      const capacity = table?.capacity ?? 0;
      occMap[o.TableID] = {
        tableId: o.TableID,
        tableName: table?.name ?? '',
        occupied: o.Pax,
        capacity,
        percentage: capacity > 0 ? Math.round((o.Pax / capacity) * 100) : 0
      };
    }
    occupancy = occMap;
  }

  let previewTable = $derived.by(() => {
    const pos = defaultSlot(tables);
    return {
      id: '',
      name: formName || 'New',
      capacity: formCapacity,
      x: pos.x,
      y: pos.y,
      degree: 0,
      isVip: formVip,
    };
  });

  const wid = get(weddingId);

  function getOcc(tableId: string): { occupied: number; percentage: number } {
    const occ = occupancy[tableId];
    if (occ) return occ;
    return { occupied: 0, percentage: 0 };
  }

  function openCreate() {
    editingTable = null;
    formName = '';
    formCapacity = 10;
    formVip = false;
    showModal = true;
  }

  function openEdit(table: BanquetTable) {
    editingTable = table;
    formName = table.name;
    formCapacity = table.capacity;
    formVip = table.isVip;
    showModal = true;
    contextMenu = null;
  }

  function closeModal() {
    showModal = false;
    editingTable = null;
  }

  async function handleSave() {
    saving = true;
    try {
      const pos = defaultSlot(tables);
      const data: TableCreateData = {
        name: formName || `Table ${(tables ?? []).length + 1}`,
        capacity: formCapacity,
        x: editingTable ? editingTable.x : pos.x,
        y: editingTable ? editingTable.y : pos.y,
        degree: editingTable ? editingTable.degree : 0,
        isVip: formVip,
      };
      if (editingTable) {
        const updated = await updateTable(wid, editingTable.id, data);
        tables = tables.map(t => t.id === editingTable!.id ? updated : t);
        addToast('Table updated', 'success');
      } else {
        const created = await createTable(wid, data);
        tables = [...tables, created];
        addToast('Table created', 'success');
      }
      closeModal();
    } catch (e: any) {
      addToast(e.message ?? 'Save failed', 'error');
    } finally {
      saving = false;
    }
  }

  async function handleDelete(table: BanquetTable) {
    try {
      await deleteTable(wid, table.id);
      tables = tables.filter(t => t.id !== table.id);
      addToast(`Table ${table.id} deleted`, 'info');
    } catch (e: any) {
      addToast(e.message ?? 'Delete failed', 'error');
    }
    contextMenu = null;
  }

  function handleCtx(e: MouseEvent, table: BanquetTable) {
    e.preventDefault();
    contextMenu = { x: e.clientX, y: e.clientY, table };
  }

  function getMenuStyle(x: number, y: number): string {
    const vw = typeof window !== 'undefined' ? window.innerWidth : 1024;
    const vh = typeof window !== 'undefined' ? window.innerHeight : 768;
    const left = x + menuWidth > vw ? x - menuWidth : x;
    const top = y + menuHeight > vh ? y - menuHeight : y;
    return `left: ${Math.max(0, left)}px; top: ${Math.max(0, top)}px;`;
  }

  async function handleSaveLayout(editTables: BanquetTable[], editElements: HallElement[], hw: number, hh: number) {
    try {
      await saveLayout(wid, {
        hallWidth: hw,
        hallHeight: hh,
        tables: editTables.map(t => ({ id: t.id, x: t.x, y: t.y, degree: t.degree })),
        elements: editElements,
      });
      addToast('Layout saved', 'success');
      editMode = false;
      await loadData();
    } catch (e: any) {
      addToast(e.message ?? 'Save failed', 'error');
    }
  }

  function handleCancelEdit() {
    editMode = false;
  }

  onMount(async () => {
    try {
      await loadData();
    } catch (e: any) {
      tablesError = e.message ?? 'Failed to load tables';
      addToast(tablesError ?? 'Failed to load tables', 'error');
    } finally {
      loading = false;
    }
  });
</script>

<svelte:head><title>Tables – WeddingDB</title></svelte:head>
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="p-4 sm:p-7 max-w-[1400px]" onclick={() => contextMenu = null}>
  <div class="flex items-center justify-between mb-6">
    <div>
      <h1 class="text-xl font-bold text-gray-900" style="letter-spacing: -0.02em;">Banquet Tables</h1>
      <p class="text-sm text-gray-500 mt-0.5">Overview of all {(tables ?? []).length} tables</p>
    </div>
    <div class="flex items-center gap-1.5 sm:gap-2">
      <button onclick={() => editMode = !editMode} class="px-2.5 sm:px-4 py-1.5 sm:py-2.5 border border-black/[0.06] bg-white/90 rounded-xl text-xs sm:text-sm font-semibold hover:bg-gray-50 transition-colors flex items-center gap-1.5 sm:gap-2">
        <Map class="w-3.5 h-3.5 sm:w-4 sm:h-4" /> <span class="hidden sm:inline">{editMode ? 'Exit Editor' : 'Edit Layout'}</span><span class="sm:hidden">{editMode ? 'Exit' : 'Layout'}</span>
      </button>
      <button onclick={openCreate} class="px-2.5 sm:px-4 py-1.5 sm:py-2.5 bg-red text-white rounded-xl text-xs sm:text-sm font-semibold hover:bg-red-light transition-colors flex items-center gap-1.5 sm:gap-2">
        <Plus class="w-3.5 h-3.5 sm:w-4 sm:h-4" /> <span class="hidden sm:inline">Add Table</span><span class="sm:hidden">Add</span>
      </button>
      <button onclick={() => goto(`/${$weddingId}/seating`)} class="px-2.5 sm:px-4 py-1.5 sm:py-2.5 border border-black/[0.06] bg-white/90 rounded-xl text-xs sm:text-sm font-semibold hover:bg-gray-50 transition-colors flex items-center gap-1.5 sm:gap-2">
        <Users class="w-3.5 h-3.5 sm:w-4 sm:h-4" /> <span class="hidden sm:inline">Manage Seating</span><span class="sm:hidden">Seating</span>
      </button>
    </div>
  </div>

  {#if editMode}
    <div class="mb-4 rounded-2xl overflow-hidden border border-gray-200 flex flex-col" style="height: 600px;">
      <HallMap
        mode="edit"
        {tables}
        {elements}
        {hallWidth}
        {hallHeight}
        tableGuests={{}}
        onSaveLayout={handleSaveLayout}
        onCancelEdit={handleCancelEdit}
      />
    </div>
  {/if}

  {#if loading}
    <div class="grid {gridCols} gap-4">
      {#each Array(8) as _, i}
        <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl p-5 flex flex-col items-center gap-3 animate-pulse">
          <div class="w-14 h-14 bg-gray-100 rounded-full"></div>
          <div class="text-center space-y-1.5">
            <div class="h-4 bg-gray-100 rounded w-16 mx-auto"></div>
            <div class="h-3 bg-gray-100 rounded w-20 mx-auto"></div>
          </div>
        </div>
      {/each}
    </div>
  {:else if tablesError}
    <div class="flex flex-col items-center justify-center py-20 text-center">
      <div class="w-16 h-16 bg-red-50 rounded-full flex items-center justify-center mb-4">
        <AlertCircle class="w-8 h-8 text-red" />
      </div>
      <p class="text-red font-medium">Failed to load tables</p>
      <p class="text-sm text-gray-500 mt-1 mb-4">{tablesError}</p>
      <button onclick={() => location.reload()} class="px-4 py-2 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors">
        Retry
      </button>
    </div>
  {:else if (tables ?? []).length === 0}
    <div class="flex flex-col items-center justify-center py-20 text-center">
      <div class="w-16 h-16 bg-gray-100 rounded-full flex items-center justify-center mb-4">
        <Users class="w-8 h-8 text-gray-400" />
      </div>
      <p class="text-gray-500 font-medium">No tables yet</p>
      <p class="text-sm text-gray-400 mt-1 mb-4">Add your first banquet table to get started.</p>
      <button onclick={openCreate} class="px-4 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center gap-2">
        <Plus class="w-4 h-4" /> Add Table
      </button>
    </div>
  {:else}
    <div class="grid {gridCols} gap-4">
      {#each tables as table (table.id)}
        {@const occ = getOcc(table.id)}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          onclick={() => goto(`/${$weddingId}/seating?table=${table.id}`)}
          oncontextmenu={(e) => handleCtx(e, table)}
          class={cn(
            "bg-white/90 backdrop-blur-xl border rounded-2xl p-3 sm:p-5 flex flex-col items-center gap-2 sm:gap-3 transition-all cursor-pointer group relative",
            table.isVip ? "border-gold-200 hover:border-gold hover:shadow-lg hover:-translate-y-0.5" : "border-black/[0.06] hover:border-gold/30 hover:shadow-lg hover:-translate-y-0.5"
          )}
          role="button"
          tabindex="0"
        >
          <!-- Three-dot menu -->
          <button
            onclick={(e) => { e.stopPropagation(); handleCtx(e, table); }}
            class="absolute top-2 right-2 p-1.5 rounded-lg opacity-0 group-hover:opacity-100 hover:bg-gray-100 transition-all"
            aria-label="Table options"
          >
            <MoreVertical class="w-4 h-4 text-gray-400" />
          </button>

          <!-- Occupancy Ring -->
          <div class="relative">
            <svg class="w-11 h-11 sm:w-14 sm:h-14" viewBox="0 0 56 56">
              <circle cx="28" cy="28" r={RING_R} fill="none" stroke="#F3F4F6" stroke-width="5" />
              <circle
                cx="28" cy="28" r={RING_R}
                fill="none"
                stroke={occ.percentage >= 90 ? '#A11217' : occ.percentage >= 60 ? '#D4AF37' : '#059669'}
                stroke-width="5"
                stroke-dasharray={RING_CIRCUM}
                stroke-dashoffset={RING_CIRCUM * (1 - occ.percentage / 100)}
                stroke-linecap="round"
                class="transition-all duration-700"
                style="transform: rotate(-90deg); transform-origin: center;"
              />
              <text x="28" y="28" text-anchor="middle" dominant-baseline="central"
                class="font-bold text-gray-800 group-hover:text-red transition-colors"
                font-size="15"
              >{table.name || table.id}</text>
            </svg>
          </div>

          <div class="text-center">
            <div class="font-semibold text-xs sm:text-sm text-gray-800">{table.name || `Table ${table.id}`}</div>
            <div class="text-[10px] sm:text-xs text-gray-500">{occ.occupied}/{table.capacity} seats</div>
          </div>

          {#if table.isVip}
            <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-gold-50 text-gold border border-gold-200 rounded-full text-[11px] font-bold">
              <Star class="w-3 h-3 fill-gold" /> VIP
            </span>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<!-- Context Menu -->
{#if contextMenu}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="fixed z-[600] bg-white/95 backdrop-blur-xl border border-black/[0.06] rounded-xl shadow-xl py-1.5 min-w-[180px]"
    style={getMenuStyle(contextMenu.x, contextMenu.y)} onclick={(e) => e.stopPropagation()}>
    <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={() => openEdit(contextMenu!.table)}>
      <Pencil class="w-4 h-4" /> Edit
    </button>
    <hr class="my-1 border-gray-100" />
    <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-red hover:bg-red-50" onclick={() => handleDelete(contextMenu!.table)}>
      <Trash2 class="w-4 h-4" /> Delete
    </button>
  </div>
{/if}

<!-- Add/Edit Modal -->
{#if showModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="absolute inset-0 bg-black/30 backdrop-blur-md" onclick={closeModal} role="presentation"></div>
    <div class="relative bg-white/95 backdrop-blur-xl rounded-2xl shadow-2xl w-full max-w-md max-h-[90vh] overflow-y-auto">
      <div class="flex items-center justify-between p-5 border-b border-gray-100">
        <h3 class="font-bold text-gray-900">{editingTable ? 'Edit Table' : 'Add Table'}</h3>
        <button onclick={closeModal} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
          <X class="w-4 h-4 text-gray-400" />
        </button>
      </div>

      <div class="p-5 space-y-4">
        <div>
          <label for="table-name" class="text-sm font-semibold text-gray-700 mb-1.5 block">Table Name</label>
          <input
            id="table-name"
            type="text"
            bind:value={formName}
            placeholder="e.g. Table 1, VIP A, 圆桌"
            class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all"
          />
        </div>

        <!-- Mini Map Preview -->
        <div>
          <label class="text-sm font-semibold text-gray-700 mb-1.5 block">Position Preview</label>
          <div class="h-[200px] rounded-xl overflow-hidden border border-gray-200 bg-gray-50">
            <HallMap
              tables={[previewTable, ...(tables ?? []).filter(t => editingTable ? t.id !== editingTable.id : true)]}
              tableGuests={{}}
              selectedTableId={null}
            />
          </div>
        </div>

        <div>
          <label for="table-capacity" class="text-sm font-semibold text-gray-700 mb-1.5 block">Capacity</label>
          <input
            id="table-capacity"
            type="number"
            min="1"
            bind:value={formCapacity}
            class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all"
          />
        </div>

        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={formVip} class="rounded" />
          <span class="text-sm font-semibold text-gray-700">VIP Table</span>
        </label>
      </div>

      <div class="flex gap-3 p-5 pt-0 sticky bottom-0 bg-white/95 backdrop-blur-xl">
        <button
          onclick={handleSave}
          disabled={saving}
          class="flex-1 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors disabled:opacity-50"
        >
          {saving ? 'Saving...' : editingTable ? 'Save Changes' : 'Create Table'}
        </button>
        <button
          onclick={closeModal}
          class="px-5 py-2.5 border border-black/[0.06] bg-white/90 rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors"
        >
          Cancel
        </button>
      </div>
    </div>
  </div>
{/if}
