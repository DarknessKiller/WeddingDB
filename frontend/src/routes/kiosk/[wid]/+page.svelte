<script lang="ts">
  import HallMap from '$lib/components/seating/HallMap.svelte';
  import { publicSearchGuests as searchGuests, publicListGuests as listGuests, publicListTables as listTables } from '$lib/api/public';
  import { cn, getInitials } from '$lib/utils';
  import { Maximize, Minimize, Monitor, Search, ArrowLeft, MapPin, Users, Star, X } from 'lucide-svelte';
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/state';
  import { setWeddingId } from '$lib/stores/weddingId';
  import { decodeId } from '$lib/utils/encode';
  import type { Guest, BanquetTable } from '$lib/types';

  let query = $state('');
  let results = $state<Guest[]>([]);
  let allGuests = $state<Guest[]>([]);
  let tables = $state<BanquetTable[]>([]);
  let selectedGuest = $state<Guest | null>(null);
  let isFullscreen = $state(false);
  let currentTime = $state(new Date());
  let timer: ReturnType<typeof setInterval>;
  let hoveredSeat = $state<{ seatNum: number; guest: Guest | null; x: number; y: number } | null>(null);
  let searching = $state(false);

  // Kiosk customization
  let kioskTitle = $state('Find Your Seat');
  let kioskDescription = $state('Enter your name to find your table and seat');
  let kioskLogoUrl = $state('');
  let kioskBackgroundUrl = $state('');
  let kioskBackgroundBlur = $state(0);
  let kioskBackgroundSize = $state('cover');
  let kioskBackgroundPosX = $state('center');
  let kioskBackgroundPosY = $state('center');

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

  let selectedTable = $derived(selectedGuest?.tableId ? tables.find(t => t.id === selectedGuest!.tableId) ?? null : null);
  let selectedTableName = $derived(selectedTable?.name ?? selectedGuest?.tableId ?? '—');
  let hasValidTable = $derived(selectedGuest?.tableId != null && selectedTable !== null);
  let seatOccupants = $derived(
    selectedGuest?.tableId
      ? Array.from({ length: selectedTable?.capacity ?? 10 }, (_, i) => {
          const seatNum = i + 1;
          const guest = allGuests.find(g => g.tableId === selectedGuest!.tableId && g.seatNumber === seatNum) ?? null;
          return { seatNum, guest };
        })
      : []
  );

  $effect(() => {
    const q = query.trim();
    if (!q) { results = []; searching = false; return; }
    searching = true;
    let cancelled = false;
    const timer = setTimeout(async () => {
      try {
        const r = await searchGuests(q);
        if (!cancelled) { results = r; searching = false; }
      } catch { if (!cancelled) { results = []; searching = false; } }
    }, 300);
    return () => { cancelled = true; clearTimeout(timer); };
  });

  onMount(() => {
    // Set wedding ID from URL param
    const wid = page.params.wid ? decodeId(page.params.wid) : '';
    if (wid) setWeddingId(wid);
    timer = setInterval(() => currentTime = new Date(), 1000);
    listGuests().then(g => allGuests = g).catch(() => {});
    listTables().then(t => tables = t).catch(() => {});
    // Load kiosk customization (public endpoint)
    fetch(`/api/public/weddings/${wid}/kiosk`).then(r => r.ok ? r.json() : null).then(data => {
      if (data) {
        if (data.kioskTitle) kioskTitle = data.kioskTitle;
        if (data.kioskDescription) kioskDescription = data.kioskDescription;
        if (data.kioskLogoUrl) kioskLogoUrl = data.kioskLogoUrl;
        if (data.kioskBackgroundUrl) kioskBackgroundUrl = data.kioskBackgroundUrl;
        if (data.kioskBackgroundBlur) kioskBackgroundBlur = data.kioskBackgroundBlur;
        if (data.kioskBackgroundSize) kioskBackgroundSize = data.kioskBackgroundSize;
        if (data.kioskBackgroundPosX) kioskBackgroundPosX = data.kioskBackgroundPosX;
        if (data.kioskBackgroundPosY) kioskBackgroundPosY = data.kioskBackgroundPosY;
      }
    }).catch(() => {});
  });

  onDestroy(() => clearInterval(timer));

  function toggleFullscreen() {
    if (!document.fullscreenElement) {
      document.documentElement.requestFullscreen();
      isFullscreen = true;
    } else {
      document.exitFullscreen();
      isFullscreen = false;
    }
  }

  function selectGuest(guest: Guest) {
    selectedGuest = guest;
    query = '';
  }

  function backToSearch() {
    selectedGuest = null;
    query = '';
  }

  function formatTime(d: Date) {
    return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
  }

  function formatDate(d: Date) {
    return d.toLocaleDateString('en-US', { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' });
  }
</script>

<svelte:head><title>Kiosk – WeddingDB</title></svelte:head>

<div class="min-h-dvh bg-gray-50 text-gray-900 flex flex-col">
  <!-- Top Bar -->
  <div class="flex items-center justify-between px-4 sm:px-8 py-3 sm:py-4 bg-white border-b border-gray-200 z-20">
    <div class="flex-1 flex items-center gap-3">
      {#if selectedGuest}
        <button onclick={backToSearch} class="p-2 rounded-xl bg-gray-100 hover:bg-gray-200 text-gray-600 transition-colors" aria-label="Back to search">
          <ArrowLeft class="w-5 h-5" />
        </button>
        <span class="font-semibold text-gray-900">Table {selectedTableName}</span>
      {/if}
    </div>
    <div class="flex-shrink-0 text-center">
      {#if !selectedGuest}
        <div class="flex items-center justify-center gap-2 mb-1">
          <Monitor class="w-5 h-5 text-gold" />
          <span class="font-semibold text-gray-600 text-sm sm:text-base">Find Your Seat</span>
        </div>
      {/if}
      <div class="text-lg sm:text-2xl font-bold text-red font-mono">{formatTime(currentTime)}</div>
      <div class="text-[10px] sm:text-xs text-gray-400 hidden sm:block">{formatDate(currentTime)}</div>
    </div>
    <div class="flex-1 flex justify-end">
      <button onclick={toggleFullscreen} class="p-2 sm:p-2.5 rounded-xl bg-gray-100 hover:bg-gray-200 text-gray-600 transition-colors" aria-label="Toggle fullscreen">
        {#if isFullscreen}
          <Minimize class="w-4 h-4 sm:w-5 sm:h-5" />
        {:else}
          <Maximize class="w-4 h-4 sm:w-5 sm:h-5" />
        {/if}
      </button>
    </div>
  </div>

  <!-- Content -->
  {#if selectedGuest}
    <!-- ===== Map + Info Panel View ===== -->
    <div class="flex-1 flex relative overflow-hidden">
      <!-- Hall Map (full screen) -->
      <HallMap
        tables={tables}
        selectedTableId={selectedGuest.tableId}
        tableGuests={tableGuests}
        dark={false}
        hoveredSeat={hoveredSeat}
      />

      <!-- Info Panel (floating overlay) -->
      {#if hasValidTable}
        <div class="absolute bottom-4 left-4 right-4 sm:left-auto sm:right-4 sm:bottom-4 sm:w-[360px] z-30">
          <div class="bg-white border border-gray-200 shadow-xl rounded-2xl shadow-2xl overflow-hidden">
            <!-- Guest Header -->
            <div class="flex items-center gap-3 p-4 border-b border-gray-100">
              <div class={cn(
                "w-12 h-12 rounded-full flex items-center justify-center text-base font-bold flex-shrink-0",
                selectedGuest.isVip ? "bg-gold-50 text-gold border-2 border-gold-200" :
                "bg-red-50 text-red border-2 border-red-200"
              )}>
                {getInitials(selectedGuest.name)}
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <span class="font-semibold text-gray-900 truncate">{selectedGuest.name}</span>
                  {#if selectedGuest.isVip}
                    <Star class="w-3.5 h-3.5 text-gold fill-gold flex-shrink-0" />
                  {/if}
                </div>
                <div class="text-xs text-gray-500">{selectedGuest.pax} pax</div>
              </div>
              <button onclick={backToSearch} class="p-1.5 rounded-lg hover:bg-gray-100 text-gray-400 hover:text-gray-700 transition-colors" aria-label="Close">
                <X class="w-4 h-4" />
              </button>
            </div>

            <!-- Table & Seat Info -->
            <div class="p-4">
              <div class="flex items-center justify-center gap-8 mb-4">
                <div class="text-center">
                  <div class="text-[10px] text-gray-500 uppercase tracking-wider mb-1">Table</div>
                  <div class="text-4xl font-extrabold text-red">{selectedTableName}</div>
                  {#if selectedTable?.isVip}
                    <span class="text-gold text-[10px] font-semibold">★ VIP</span>
                  {/if}
                </div>
                <div class="w-px h-10 bg-gray-200"></div>
                <div class="text-center">
                  <div class="text-[10px] text-gray-500 uppercase tracking-wider mb-1">Seats</div>
                  <div class="text-xl font-bold text-gray-900">
                    {selectedGuest.seatNumber}–{(selectedGuest.seatNumber ?? 0) + selectedGuest.pax - 1}
                  </div>
                </div>
              </div>

              <!-- Mini Seat Map -->
              <div class="grid grid-cols-5 gap-1.5">
                {#each seatOccupants as { seatNum, guest }}
                  {@const isOwn = seatNum >= (selectedGuest.seatNumber ?? 0) && seatNum < (selectedGuest.seatNumber ?? 0) + selectedGuest.pax}
                  <div class={cn(
                    "aspect-square rounded-md flex items-center justify-center text-[10px] font-bold border transition-colors",
                    isOwn ? "bg-red text-white border-red shadow-md shadow-red/20" :
                    guest ? "bg-gray-100 text-gray-500 border-gray-200" :
                    "bg-gray-50 text-gray-400 border-gray-200"
                  )}>
                    {seatNum}
                  </div>
                {/each}
              </div>

              <p class="text-center text-[11px] text-gray-400 mt-3">
                <MapPin class="w-3 h-3 inline mr-1" />
                Look for the highlighted table on the map
              </p>
            </div>
          </div>
        </div>
      {:else}
        <!-- No table assigned -->
        <div class="absolute bottom-4 left-4 right-4 sm:left-auto sm:right-4 sm:bottom-4 sm:w-[360px] z-30">
          <div class="bg-white border border-gray-200 shadow-xl rounded-2xl shadow-2xl p-6 text-center">
            <MapPin class="w-8 h-8 text-gray-400 mx-auto mb-3" />
            <h3 class="font-bold text-gray-900 mb-1">No Seat Assigned</h3>
            <p class="text-sm text-gray-500">Please see the reception desk for seating.</p>
          </div>
        </div>
      {/if}
    </div>

  {:else}
    <!-- ===== Search View ===== -->
    <div class="flex-1 flex flex-col items-center justify-center p-4 sm:p-8 relative overflow-hidden"
      style={kioskBackgroundUrl ? 'background: linear-gradient(135deg, #fef2f2, #fffbeb);' : ''}>
      {#if kioskBackgroundUrl}
        <div class="absolute inset-0" style={`background-image: url(${kioskBackgroundUrl}); background-size: ${kioskBackgroundSize}; background-position: ${kioskBackgroundPosX} ${kioskBackgroundPosY}; filter: blur(${kioskBackgroundBlur}px); transform: scale(${kioskBackgroundBlur > 0 ? 1.1 : 1});`}></div>
      {/if}
      <div class="w-full max-w-lg text-center relative z-10">
        <div class="mb-8">
          {#if kioskLogoUrl}
            <img src={kioskLogoUrl} alt="Logo" class="w-28 h-28 sm:w-32 sm:h-32 mx-auto mb-4 sm:mb-6 rounded-2xl object-cover shadow-lg" />
          {:else}
            <div class="w-28 h-28 sm:w-32 sm:h-32 rounded-2xl bg-red mx-auto flex items-center justify-center text-gold text-4xl sm:text-5xl font-serif font-bold mb-4 sm:mb-6 shadow-lg shadow-red/30">
              囍
            </div>
          {/if}
          <h1 class="text-3xl sm:text-4xl font-bold text-gray-900 mb-2">{kioskTitle}</h1>
          {#if kioskDescription}
            <p class="text-gray-500">{kioskDescription}</p>
          {/if}
        </div>

        <!-- Search Input -->
        <div class="relative mb-8">
          <Search class="absolute left-4 top-1/2 -translate-y-1/2 w-6 h-6 text-gray-500 pointer-events-none" />
          <input
            type="text"
            placeholder="Type your name..."
            bind:value={query}
            class="w-full pl-13 pr-5 py-5 border border-gray-200 rounded-2xl text-lg bg-white text-gray-900 placeholder-gray-400 focus:border-red focus:ring-2 focus:ring-red/15 outline-none transition-all"
            autofocus
          />
        </div>

        <!-- Results -->
        {#if results.length > 0}
          <div class="space-y-3 text-left">
            {#each results.slice(0, 8) as guest (guest.id)}
              <button
                onclick={() => selectGuest(guest)}
                class="w-full bg-white border border-gray-200 rounded-2xl p-4 flex items-center gap-4 hover:border-red/50 hover:bg-gray-50 shadow-sm transition-all group"
              >
                <div class={cn(
                  "w-12 h-12 rounded-full flex items-center justify-center text-lg font-bold flex-shrink-0",
                  guest.isVip ? "bg-gold-50 text-gold border-2 border-gold-200" :
                  "bg-red-50 text-red border-2 border-red-200"
                )}>
                  {getInitials(guest.name)}
                </div>
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="font-semibold text-gray-900 group-hover:text-red transition-colors">{guest.name}</span>
                    {#if guest.isVip}
                      <Star class="w-3.5 h-3.5 text-gold fill-gold" />
                    {/if}
                  </div>
                  <div class="flex items-center gap-3 text-sm text-gray-500 mt-0.5">
                    {#if guest.tableId}
                      <span class="flex items-center gap-1"><MapPin class="w-3.5 h-3.5" />Table {tables.find(t => t.id === guest.tableId)?.name ?? guest.tableId}</span>
                      <span>Seat {guest.seatNumber}–{(guest.seatNumber ?? 0) + guest.pax - 1}</span>
                    {:else}
                      <span class="text-gray-500">No seat assigned</span>
                    {/if}
                    <span>{guest.pax} pax</span>
                  </div>
                </div>
                <div class="text-gray-400 group-hover:text-red transition-colors">
                  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>
                </div>
              </button>
            {/each}
          </div>
        {:else if query.trim().length > 0 && !searching}
          <div class="text-center py-12 text-gray-400">
            <Search class="w-12 h-12 mx-auto mb-3 opacity-40" />
            <p class="font-medium">No guests found</p>
            <p class="text-sm mt-1">Try a different spelling</p>
          </div>
        {:else if searching}
          <div class="text-center py-12 text-gray-500">
            <div class="w-8 h-8 border-2 border-red/30 border-t-red rounded-full animate-spin mx-auto mb-3"></div>
            <p class="font-medium">Searching...</p>
          </div>
        {:else}
          <div class="text-center py-8 text-gray-400">
            <Users class="w-10 h-10 mx-auto mb-2 opacity-40" />
            <p class="text-sm">Start typing to search</p>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>

<!-- Hover Tooltip -->
{#if hoveredSeat}
  <div
    class="fixed z-[500] px-3 py-2 bg-gray-900 text-white text-xs rounded-lg shadow-xl pointer-events-none whitespace-nowrap"
    style="left: {hoveredSeat.x}px; top: {hoveredSeat.y - 12}px; transform: translate(-50%, -100%);"
  >
    {#if hoveredSeat.guest}
      <div class="font-semibold">{hoveredSeat.guest.name}</div>
      <div class="text-gray-300">Seat {hoveredSeat.seatNum} • {hoveredSeat.guest.pax} pax</div>
    {:else}
      <div>Seat {hoveredSeat.seatNum} — Empty</div>
    {/if}
  </div>
{/if}
