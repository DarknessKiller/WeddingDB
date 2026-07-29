<script lang="ts">
  import { searchGuests, listGuests, listTables } from '$lib/api/search';
  import { getLayout } from '$lib/api/layout';
  import { getWedding } from '$lib/api/weddings';
  import { addToast, selectedGuest, isDrawerOpen } from '$lib/stores';
  import { weddingId } from '$lib/stores/weddingId';
  import { get } from 'svelte/store';
  import Badge from '$lib/components/ui/Badge.svelte';
  import { getInitials, cn } from '$lib/utils';
  import { Search, CheckCircle2, Phone } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import type { Guest, BanquetTable, HallElement } from '$lib/types';
  import HallMap from '$lib/components/seating/HallMap.svelte';

  let query = $state('');
  let results = $state<Guest[]>([]);
  let loading = $state(false);

  let tables = $state<BanquetTable[]>([]);
  let allGuests = $state<Guest[]>([]);
  let elements = $state<HallElement[]>([]);
  let hallWidth = $state(860);
  let hallHeight = $state(1000);
  let dataLoading = $state(true);
  let showSeatNumbers = $state(true);
  let highlightTableId = $state<string | null>(null);
  let showResults = $state(false);
  let sortCol = $state<string>('name');
  let sortDir = $state<'asc' | 'desc'>('asc');

  function toggleSort(col: string) {
    if (sortCol === col) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
    else { sortCol = col; sortDir = 'asc'; }
  }

  let sortedGuests = $derived.by(() => {
    const r = [...allGuests];
    r.sort((a, b) => {
      let av: string | number, bv: string | number;
      switch (sortCol) {
        case 'name': av = a.name; bv = b.name; break;
        case 'rsvp': av = a.rsvp; bv = b.rsvp; break;
        case 'pax': av = a.pax; bv = b.pax; break;
        case 'checkedIn': av = a.checkedIn ? 1 : 0; bv = b.checkedIn ? 1 : 0; break;
        default: av = a.name; bv = b.name;
      }
      const cmp = av < bv ? -1 : av > bv ? 1 : 0;
      return sortDir === 'asc' ? cmp : -cmp;
    });
    return r;
  });

  // Track drawer open/close to refresh data
  let prevDrawerOpen = $state(false);
  $effect(() => {
    const isOpen = $isDrawerOpen;
    if (prevDrawerOpen && !isOpen) {
      // Drawer just closed — refresh guest data
      refreshGuests();
    }
    prevDrawerOpen = isOpen;
  });

  async function refreshGuests() {
    const g = await listGuests().catch(() => [] as Guest[]);
    allGuests = g;
    // Also update the store guest if it's still selected
    if ($selectedGuest) {
      const updated = allGuests.find(g => g.id === $selectedGuest!.id);
      if (updated) $selectedGuest = updated;
    }
  }

  let initialized = $state(false);

  onMount(() => {
    init();
  });

  async function init() {
    if (initialized) return;
    initialized = true;
    try {
      const [t, g, layout] = await Promise.all([
        listTables().catch(() => [] as BanquetTable[]),
        listGuests().catch(() => [] as Guest[]),
        getLayout(get(weddingId)).catch(() => null),
      ]);
      tables = t;
      allGuests = g;
      if (layout) {
        elements = layout.elements;
        hallWidth = layout.hallWidth;
        hallHeight = layout.hallHeight;
      }
      const wid = get(weddingId);
      const w = await getWedding(wid).catch(() => null) as any;
      if (w?.showSeatNumbers !== undefined) showSeatNumbers = w.showSeatNumbers;
    } catch {
      addToast('Failed to load data', 'error');
    } finally {
      dataLoading = false;
    }
  }

  async function doSearch() {
    const q = query.trim();
    if (!q) { results = []; return; }
    loading = true;
    try {
      results = await searchGuests(q);
    } catch {
      addToast('Search failed', 'error');
      results = [];
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    if (query.trim().length > 0) {
      showResults = true;
      loading = true;
      timer = setTimeout(doSearch, 300);
    } else {
      results = [];
      showResults = false;
      loading = false;
    }
    return () => clearTimeout(timer);
  });

  function openGuest(guest: Guest) {
    $selectedGuest = guest;
    $isDrawerOpen = true;
    highlightTableId = guest.tableId;
    showResults = false;
  }

  function handleTableClick(id: string) {
    highlightTableId = highlightTableId === id ? null : id;
  }

  // Build tableGuests for HallMap
  let tableGuests = $derived.by(() => {
    const obj: Record<string, Guest[]> = {};
    for (const g of allGuests) {
      if (g.tableId === null) continue;
      const key = String(g.tableId);
      if (!obj[key]) obj[key] = [];
      obj[key].push(g);
    }
    return obj;
  });

  // Use store guest for map highlighting, fallback to local highlightTableId
  let highlightedTableId = $derived($selectedGuest?.tableId ?? highlightTableId);
</script>

<svelte:head><title>Check In – WeddingDB</title></svelte:head>

{#if dataLoading}
  <div class="flex items-center justify-center h-full">
    <div class="text-center text-gray-400">
      <div class="w-8 h-8 border-2 border-red/30 border-t-red rounded-full animate-spin mx-auto mb-3"></div>
      <p class="text-sm">Loading check-in data...</p>
    </div>
  </div>
{:else}
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="flex flex-col h-full"
  onclick={(e) => {
    const target = e.target as HTMLElement;
    if (!target.closest('[data-search-area]')) {
      showResults = false;
    }
  }}
>
  <!-- Search Toolbar -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div data-search-area class="relative bg-white/90 backdrop-blur-xl border-b border-black/[0.06] px-4 py-3 z-30 flex-shrink-0" onclick={(e) => e.stopPropagation()}>
    <div class="relative max-w-xl mx-auto">
      <Search class="absolute left-4 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400 pointer-events-none" />
      <input
        type="text"
        placeholder="Search by name or phone number..."
        bind:value={query}
        onfocus={() => { if (query.trim()) showResults = true; }}
        class="w-full pl-12 pr-11 py-3.5 bg-white/80 rounded-2xl text-base border border-black/[0.06] focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all placeholder-gray-400 min-h-[48px] shadow-sm"
        autofocus
      />
      {#if loading}
        <div class="absolute right-4 top-1/2 -translate-y-1/2 w-5 h-5 border-2 border-red/30 border-t-red rounded-full animate-spin"></div>
      {/if}
    </div>

    <!-- Search Results Dropdown -->
    {#if showResults && query.trim().length > 0}
      <div class="absolute top-full left-0 right-0 mt-2 max-w-xl mx-auto bg-white/95 backdrop-blur-xl rounded-xl shadow-xl border border-black/[0.06] overflow-hidden max-h-[60vh] overflow-y-auto z-50">
        {#if results.length > 0}
          {#each results.slice(0, 10) as guest (guest.id)}
            <button
              class="w-full flex items-center gap-3 px-4 py-3 hover:bg-gray-50 transition-colors text-left border-b border-gray-100 last:border-0"
              onclick={() => openGuest(guest)}
            >
              <div class={cn(
                "w-10 h-10 rounded-full flex items-center justify-center text-sm font-bold flex-shrink-0",
                guest.checkedIn ? "bg-emerald-50 text-emerald-700 border-2 border-emerald-300" :
                guest.isVip ? "bg-gold-50 text-gold border-2 border-gold-300" :
                "bg-red-50 text-red border-2 border-red-200"
              )}>
                {getInitials(guest.name)}
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-1.5">
                  {#if guest.isVip}<span class="text-gold text-xs">★</span>{/if}
                  <span class="font-semibold text-sm text-gray-900 truncate">{guest.name}</span>
                </div>
                <div class="flex items-center gap-2 text-xs text-gray-500">
                  <span class="flex items-center gap-0.5"><Phone class="w-3 h-3" />{guest.phone}</span>
                  <span>•</span>
                  <span>{tables.find(t => t.id === guest.tableId)?.name || `T${guest.tableId ?? '?'}`}</span>
                  <span>•</span>
                  <span>{guest.pax} pax</span>
                </div>
              </div>
              <Badge status={guest.rsvp} />
              {#if guest.checkedIn}
                <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-emerald-50 text-emerald-700 rounded-full text-xs font-semibold border border-emerald-200 flex-shrink-0">
                  <CheckCircle2 class="w-3 h-3" />
                </span>
              {/if}
            </button>
          {/each}
        {:else if !loading}
          <div class="px-4 py-6 text-center text-gray-400">
            <Search class="w-8 h-8 mx-auto mb-2 opacity-40" />
            <p class="text-sm font-medium">No guests found</p>
          </div>
        {/if}
      </div>
    {/if}
  </div>

  <!-- Content Area: Guest List + Map -->
  <div class="flex-1 flex flex-col md:flex-row min-h-0">
    <!-- Guest List (top on mobile, left sidebar on desktop) -->
    {#if !$isDrawerOpen && (!showResults || !query.trim())}
      <div class="md:w-80 lg:w-96 md:border-r border-gray-200 bg-white overflow-y-auto flex-shrink-0 max-h-[35vh] md:max-h-none">
        {#if allGuests.length === 0}
          <div class="p-8 text-center text-gray-400">
            <p class="text-sm">Loading guests...</p>
          </div>
        {:else}
          <div class="flex items-center gap-2 px-3 py-1.5 border-b border-gray-100 bg-gray-50 text-xs">
            <button onclick={() => toggleSort('name')} class="{sortCol === 'name' ? 'text-red font-bold' : 'text-gray-500'}">Name</button>
            <button onclick={() => toggleSort('rsvp')} class="{sortCol === 'rsvp' ? 'text-red font-bold' : 'text-gray-500'}">RSVP</button>
            <button onclick={() => toggleSort('checkedIn')} class="{sortCol === 'checkedIn' ? 'text-red font-bold' : 'text-gray-500'}">Status</button>
          </div>
          <div class="divide-y divide-gray-100">
            {#each sortedGuests as guest (guest.id)}
              <button
                class="w-full flex items-center gap-3 px-4 py-3 hover:bg-gray-50 transition-colors text-left"
                onclick={() => openGuest(guest)}
              >
                <div class={cn(
                  "w-10 h-10 rounded-full flex items-center justify-center text-sm font-bold flex-shrink-0",
                  guest.checkedIn ? "bg-emerald-50 text-emerald-700 border-2 border-emerald-300" :
                  guest.isVip ? "bg-gold-50 text-gold border-2 border-gold-300" :
                  "bg-red-50 text-red border-2 border-red-200"
                )}>
                  {getInitials(guest.name)}
                </div>
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-1.5">
                    {#if guest.isVip}<span class="text-gold text-xs">★</span>{/if}
                    <span class="font-semibold text-sm text-gray-900 truncate">{guest.name}</span>
                  </div>
                  <div class="flex items-center gap-2 text-xs text-gray-500">
                    <span>{tables.find(t => t.id === guest.tableId)?.name || '—'}</span>
                    <span>•</span>
                    <span>{guest.pax} pax</span>
                  </div>
                </div>
                <Badge status={guest.rsvp} />
                {#if guest.checkedIn}
                  <CheckCircle2 class="w-4 h-4 text-emerald-500 flex-shrink-0" />
                {/if}
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

    <!-- Map (fills remaining space) -->
    <div class="flex-1 relative overflow-hidden min-w-0 flex flex-col">
      {#if tables.length > 0}
        <HallMap
          tables={tables}
          {elements}
          {hallWidth}
          {hallHeight}
          tableGuests={tableGuests}
          legendPosition="top-left"
          onTableClick={handleTableClick}
          {highlightedTableId}
        />
      {/if}
    </div>
  </div>
</div>
{/if}
