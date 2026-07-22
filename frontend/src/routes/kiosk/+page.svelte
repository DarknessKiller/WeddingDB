<script lang="ts">
  import HallMap from '$lib/components/seating/HallMap.svelte';
  import { searchGuests, getSeatGuest } from '$lib/mock/data';
  import { TABLE_DEFINITIONS } from '$lib/constants';
  import { cn } from '$lib/utils';
  import { Maximize, Minimize, Monitor, Search, ArrowLeft, MapPin, Users, Star, X } from 'lucide-svelte';
  import { onMount, onDestroy } from 'svelte';
  import type { Guest } from '$lib/types';

  let query = $state('');
  let results = $derived(query.trim().length > 0 ? searchGuests(query) : []);
  let selectedGuest = $state<Guest | null>(null);
  let isFullscreen = $state(false);
  let currentTime = $state(new Date());
  let timer: ReturnType<typeof setInterval>;
  let hoveredSeat = $state<{ seatNum: number; guest: Guest | null; x: number; y: number } | null>(null);

  let selectedTable = $derived(selectedGuest?.tableId ? TABLE_DEFINITIONS.find(t => t.id === selectedGuest!.tableId) ?? null : null);
  let seatOccupants = $derived(
    selectedGuest?.tableId
      ? Array.from({ length: selectedTable?.capacity ?? 10 }, (_, i) => {
          const seatNum = i + 1;
          const guest = getSeatGuest(selectedGuest!.tableId!, seatNum);
          return { seatNum, guest };
        })
      : []
  );

  onMount(() => {
    timer = setInterval(() => currentTime = new Date(), 1000);
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

<div class="min-h-dvh bg-gray-950 text-white flex flex-col">
  <!-- Top Bar -->
  <div class="flex items-center justify-between px-4 sm:px-8 py-3 sm:py-4 bg-gray-900/80 border-b border-gray-800 z-20">
    <div class="flex items-center gap-3">
      {#if selectedGuest}
        <button onclick={backToSearch} class="p-2 rounded-xl bg-gray-800 hover:bg-gray-700 transition-colors" aria-label="Back to search">
          <ArrowLeft class="w-5 h-5" />
        </button>
        <span class="font-semibold text-white">Table {selectedGuest.tableId}</span>
      {:else}
        <Monitor class="w-5 h-5 text-gold" />
        <span class="font-semibold text-gray-300 hidden sm:inline">Find Your Seat</span>
      {/if}
    </div>
    <div class="text-right">
      <div class="text-lg sm:text-2xl font-bold text-gold font-mono">{formatTime(currentTime)}</div>
      <div class="text-[10px] sm:text-xs text-gray-400 hidden sm:block">{formatDate(currentTime)}</div>
    </div>
    <button onclick={toggleFullscreen} class="p-2 sm:p-2.5 rounded-xl bg-gray-800 hover:bg-gray-700 transition-colors" aria-label="Toggle fullscreen">
      {#if isFullscreen}
        <Minimize class="w-4 h-4 sm:w-5 sm:h-5" />
      {:else}
        <Maximize class="w-4 h-4 sm:w-5 sm:h-5" />
      {/if}
    </button>
  </div>

  <!-- Content -->
  {#if selectedGuest}
    <!-- ===== Map + Info Panel View ===== -->
    <div class="flex-1 flex relative overflow-hidden">
      <!-- Hall Map (full screen) -->
      <HallMap
        kioskHighlightTableId={selectedGuest.tableId}
        hoveredSeat={hoveredSeat}
      />

      <!-- Info Panel (floating overlay) -->
      {#if selectedGuest.tableId}
        <div class="absolute bottom-4 left-4 right-4 sm:left-auto sm:right-4 sm:bottom-4 sm:w-[360px] z-30">
          <div class="bg-gray-900/95 backdrop-blur-sm border border-gray-700 rounded-2xl shadow-2xl overflow-hidden">
            <!-- Guest Header -->
            <div class="flex items-center gap-3 p-4 border-b border-gray-800">
              <div class={cn(
                "w-12 h-12 rounded-full flex items-center justify-center text-base font-bold flex-shrink-0",
                selectedGuest.isVip ? "bg-gold/20 text-gold border-2 border-gold/40" :
                "bg-red/20 text-red border-2 border-red/40"
              )}>
                {selectedGuest.name.split(' ').map(n => n[0]).join('')}
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <span class="font-semibold text-white truncate">{selectedGuest.name}</span>
                  {#if selectedGuest.isVip}
                    <Star class="w-3.5 h-3.5 text-gold fill-gold flex-shrink-0" />
                  {/if}
                </div>
                <div class="text-xs text-gray-400">{selectedGuest.pax} pax</div>
              </div>
              <button onclick={backToSearch} class="p-1.5 rounded-lg hover:bg-gray-800 text-gray-400 hover:text-white transition-colors" aria-label="Close">
                <X class="w-4 h-4" />
              </button>
            </div>

            <!-- Table & Seat Info -->
            <div class="p-4">
              <div class="flex items-center justify-center gap-8 mb-4">
                <div class="text-center">
                  <div class="text-[10px] text-gray-500 uppercase tracking-wider mb-1">Table</div>
                  <div class="text-4xl font-extrabold text-gold">{selectedGuest.tableId}</div>
                  {#if selectedTable?.isVip}
                    <span class="text-gold text-[10px] font-semibold">★ VIP</span>
                  {/if}
                </div>
                <div class="w-px h-10 bg-gray-700"></div>
                <div class="text-center">
                  <div class="text-[10px] text-gray-500 uppercase tracking-wider mb-1">Seats</div>
                  <div class="text-xl font-bold text-white">
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
                    isOwn ? "bg-gold text-gray-950 border-gold shadow-md shadow-gold/20" :
                    guest ? "bg-gray-800 text-gray-500 border-gray-700" :
                    "bg-gray-800/50 text-gray-600 border-gray-800"
                  )}>
                    {seatNum}
                  </div>
                {/each}
              </div>

              <p class="text-center text-[11px] text-gray-500 mt-3">
                <MapPin class="w-3 h-3 inline mr-1" />
                Look for the highlighted table on the map
              </p>
            </div>
          </div>
        </div>
      {:else}
        <!-- No table assigned -->
        <div class="absolute bottom-4 left-4 right-4 sm:left-auto sm:right-4 sm:bottom-4 sm:w-[360px] z-30">
          <div class="bg-gray-900/95 backdrop-blur-sm border border-gray-700 rounded-2xl shadow-2xl p-6 text-center">
            <MapPin class="w-8 h-8 text-gray-500 mx-auto mb-3" />
            <h3 class="font-bold text-white mb-1">No Seat Assigned</h3>
            <p class="text-sm text-gray-400">Please see the reception desk for seating.</p>
          </div>
        </div>
      {/if}
    </div>

  {:else}
    <!-- ===== Search View ===== -->
    <div class="flex-1 flex flex-col items-center justify-center p-4 sm:p-8">
      <div class="w-full max-w-lg text-center">
        <div class="mb-8">
          <div class="w-20 h-20 rounded-2xl bg-red mx-auto flex items-center justify-center text-gold text-3xl font-serif font-bold mb-4 shadow-lg shadow-red/30">
            囍
          </div>
          <h1 class="text-3xl sm:text-4xl font-bold text-white mb-2">Find Your Seat</h1>
          <p class="text-gray-400">Enter your name to find your table and seat</p>
        </div>

        <!-- Search Input -->
        <div class="relative mb-8">
          <Search class="absolute left-4 top-1/2 -translate-y-1/2 w-6 h-6 text-gray-500 pointer-events-none" />
          <input
            type="text"
            placeholder="Type your name..."
            bind:value={query}
            class="w-full pl-13 pr-5 py-5 border border-gray-700 rounded-2xl text-lg bg-gray-900 text-white placeholder-gray-500 focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all"
            autofocus
          />
        </div>

        <!-- Results -->
        {#if results.length > 0}
          <div class="space-y-3 text-left">
            {#each results.slice(0, 8) as guest (guest.id)}
              <button
                onclick={() => selectGuest(guest)}
                class="w-full bg-gray-900 border border-gray-800 rounded-2xl p-4 flex items-center gap-4 hover:border-gold/50 hover:bg-gray-800 transition-all group"
              >
                <div class={cn(
                  "w-12 h-12 rounded-full flex items-center justify-center text-lg font-bold flex-shrink-0",
                  guest.isVip ? "bg-gold/20 text-gold border-2 border-gold/40" :
                  "bg-red/20 text-red border-2 border-red/40"
                )}>
                  {guest.name.split(' ').map(n => n[0]).join('')}
                </div>
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="font-semibold text-white group-hover:text-gold transition-colors">{guest.name}</span>
                    {#if guest.isVip}
                      <Star class="w-3.5 h-3.5 text-gold fill-gold" />
                    {/if}
                  </div>
                  <div class="flex items-center gap-3 text-sm text-gray-400 mt-0.5">
                    {#if guest.tableId}
                      <span class="flex items-center gap-1"><MapPin class="w-3.5 h-3.5" />Table {guest.tableId}</span>
                      <span>Seat {guest.seatNumber}–{(guest.seatNumber ?? 0) + guest.pax - 1}</span>
                    {:else}
                      <span class="text-gray-500">No seat assigned</span>
                    {/if}
                    <span>{guest.pax} pax</span>
                  </div>
                </div>
                <div class="text-gray-600 group-hover:text-gold transition-colors">
                  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>
                </div>
              </button>
            {/each}
          </div>
        {:else if query.trim().length > 0}
          <div class="text-center py-12 text-gray-500">
            <Search class="w-12 h-12 mx-auto mb-3 opacity-40" />
            <p class="font-medium">No guests found</p>
            <p class="text-sm mt-1">Try a different spelling</p>
          </div>
        {:else}
          <div class="text-center py-8 text-gray-600">
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
