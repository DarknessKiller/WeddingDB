<script lang="ts">
  import { searchGuests, getGuest, listGuests, listTables, checkInGuest } from '$lib/api/search';
  import { addToast } from '$lib/stores';
  import Badge from '$lib/components/ui/Badge.svelte';
  import { getInitials, cn } from '$lib/utils';
  import { goto } from '$app/navigation';
  import { Search, CheckCircle2, UserCheck, Phone, MapPin, Gift, Banknote, X, MapPinned, Loader2 } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import type { Guest, BanquetTable } from '$lib/types';
  import HallMap from '$lib/components/seating/HallMap.svelte';

  let query = $state('');
  let results = $state<Guest[]>([]);
  let loading = $state(false);


  let showCheckinModal = $state(false);
  let showSeatView = $state(false);
  let checkinGuest = $state<Guest | null>(null);
  let selectedGuest = $state<Guest | null>(null);
  let angbaoAmount = $state('');
  let giftItem = $state('');
  let checkingIn = $state(false);

  let tables = $state<BanquetTable[]>([]);
  let allGuests = $state<Guest[]>([]);

  // ponytail: load tables/guests on mount for seating map
  let initialized = $state(false);

  onMount(() => {
    init();
  });

  async function init() {
    if (initialized) return;
    initialized = true;
    try {
      [tables, allGuests] = await Promise.all([listTables(), listGuests()]);
    } catch {
      addToast('Failed to load data', 'error');
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

  function selectGuest(guest: Guest) {
    selectedGuest = guest;
    highlightTableId = guest.tableId;
  }

  function openCheckinModal(guestId: string) {
    const guest = results.find(g => g.id === guestId) ?? allGuests.find(g => g.id === guestId);
    if (guest) {
      checkinGuest = guest;
      angbaoAmount = guest.angbaoAmount?.toString() ?? '';
      giftItem = guest.giftItem ?? '';
      showSeatView = false;
      showCheckinModal = true;
    }
  }

  async function handleCheckIn() {
    if (!checkinGuest || checkingIn) return;
    checkingIn = true;
    try {
      const amt = angbaoAmount ? parseFloat(angbaoAmount) : undefined;
      const updated = await checkInGuest(checkinGuest.id, amt, giftItem || undefined);
      checkinGuest = updated;
      if (selectedGuest?.id === updated.id) selectedGuest = updated;
      results = results.map(g => g.id === updated.id ? updated : g);
      addToast(`${updated.name} checked in successfully`, 'success');
      showSeatView = true;
    } catch {
      addToast('Check-in failed', 'error');
    } finally {
      checkingIn = false;
    }
  }

  function closeModal() {
    showCheckinModal = false;
    showSeatView = false;
    checkinGuest = null;
  }

  function viewOnMap() {
    if (!checkinGuest?.tableId) return;
    const tableId = checkinGuest.tableId;
    closeModal();
    goto(`/seating?table=${tableId}`);
  }

  // ponytail: build tableGuests map for HallMap
  let tableGuests = $derived.by(() => {
    const map = new Map<number, Guest[]>();
    for (const g of allGuests) {
      if (g.tableId === null) continue;
      const arr = map.get(g.tableId) ?? [];
      arr.push(g);
      map.set(g.tableId, arr);
    }
    return map;
  });

  let highlightTableId = $state<number | null>(null);

  let showResults = $state(false);

  function selectGuestAndClose(guest: Guest) {
    selectGuest(guest);
    showResults = false;
  }

  function deselectGuest() {
    selectedGuest = null;
    highlightTableId = null;
  }

  function handleTableClick(id: number) {
    highlightTableId = highlightTableId === id ? null : id;
  }

  function getSeatOccupants(tableId: number, capacity: number) {
    return Array.from({ length: capacity }, (_, i) => {
      const seatNum = i + 1;
      const guest = allGuests.find(g =>
        g.tableId === tableId &&
        g.seatNumber !== null &&
        seatNum >= g.seatNumber &&
        seatNum < g.seatNumber + g.pax
      );
      return { seatNum, guest };
    });
  }
</script>

<svelte:head><title>Check In – WeddingDB</title></svelte:head>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="relative flex h-[calc(100dvh-56px)] sm:h-[calc(100dvh-64px)] overflow-hidden p-4 gap-4"
  onclick={(e) => {
    const target = e.target as HTMLElement;
    if (!target.closest('[data-search-area]') && !target.closest('[data-panel-area]')) {
      showResults = false;
    }
  }}
>
  <!-- Full-screen Seating Map -->
  {#if tables.length > 0}
    <HallMap
      tables={tables}
      tableGuests={tableGuests}
      onTableClick={handleTableClick}
      highlightedTableId={selectedGuest?.tableId ?? highlightTableId}
    />
  {/if}

  <!-- Floating Search Bar + Dropdown -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div data-search-area class="absolute top-6 left-1/2 -translate-x-1/2 w-full max-w-xl z-30 px-4" onclick={(e) => e.stopPropagation()}>
    <div class="relative">
      <Search class="absolute left-4 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400 pointer-events-none" />
      <input
        type="text"
        placeholder="Search by name or phone number..."
        bind:value={query}
        onfocus={() => { if (query.trim()) showResults = true; }}
        class="w-full pl-12 pr-11 py-3.5 bg-white/95 backdrop-blur-md rounded-2xl text-base shadow-lg border border-white/60 focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all placeholder-gray-400"
        autofocus
      />
      {#if loading}
        <Loader2 class="absolute right-4 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400 animate-spin" />
      {/if}
    </div>

    <!-- Search Results Dropdown -->
    {#if showResults && query.trim().length > 0}
      <div class="mt-2 bg-white/95 backdrop-blur-md rounded-2xl shadow-xl border border-white/60 overflow-hidden max-h-[50vh] overflow-y-auto">
        {#if results.length > 0}
          {#each results.slice(0, 10) as guest (guest.id)}
            <button
              class="w-full flex items-center gap-3 px-4 py-3 hover:bg-gray-50 transition-colors text-left border-b border-gray-100 last:border-0"
              onclick={() => selectGuestAndClose(guest)}
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

  <!-- Side Panel (right) -->
  {#if selectedGuest}
    <div data-panel-area class="absolute top-0 right-0 h-full w-[340px] bg-white shadow-2xl z-40 flex flex-col animate-in hidden md:flex">
      <!-- Panel Header -->
      <div class="flex items-center justify-between px-5 py-4 border-b border-gray-100">
        <div class="flex items-center gap-3">
          <div class={cn(
            "w-11 h-11 rounded-full flex items-center justify-center text-sm font-bold",
            selectedGuest.checkedIn ? "bg-emerald-50 text-emerald-700 border-2 border-emerald-300" :
            selectedGuest.isVip ? "bg-gold-50 text-gold border-2 border-gold-300" :
            "bg-red-50 text-red border-2 border-red-200"
          )}>
            {getInitials(selectedGuest.name)}
          </div>
          <div>
            <h3 class="font-bold text-gray-900 flex items-center gap-1.5">
              {#if selectedGuest.isVip}<span class="text-gold">★</span>{/if}
              {selectedGuest.name}
            </h3>
            <p class="text-xs text-gray-500">{selectedGuest.phone}</p>
          </div>
        </div>
        <button onclick={deselectGuest} class="p-2 rounded-lg hover:bg-gray-100 transition-colors" aria-label="Close panel">
          <X class="w-4 h-4 text-gray-400" />
        </button>
      </div>

      <!-- Guest Info -->
      <div class="px-5 py-4">
        <div class="grid grid-cols-3 gap-3 text-sm mb-4">
          <div class="bg-gray-50 rounded-xl p-3 text-center">
            <div class="text-gray-500 text-xs">Table</div>
            <div class="font-bold text-gray-900 text-lg">{tables.find(t => t.id === selectedGuest.tableId)?.name || (selectedGuest.tableId ?? '—')}</div>
          </div>
          <div class="bg-gray-50 rounded-xl p-3 text-center">
            <div class="text-gray-500 text-xs">Seat</div>
            <div class="font-bold text-gray-900 text-lg">{selectedGuest.seatNumber ?? '—'}</div>
          </div>
          <div class="bg-gray-50 rounded-xl p-3 text-center">
            <div class="text-gray-500 text-xs">Pax</div>
            <div class="font-bold text-gray-900 text-lg">{selectedGuest.pax}</div>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <Badge status={selectedGuest.rsvp} />
          {#if selectedGuest.checkedIn}
            <span class="inline-flex items-center gap-1 px-2.5 py-1 bg-emerald-50 text-emerald-700 rounded-full text-xs font-semibold border border-emerald-200">
              <CheckCircle2 class="w-3 h-3" /> Checked In
            </span>
          {/if}
        </div>

        {#if selectedGuest.checkedIn && selectedGuest.angbaoAmount}
          <div class="flex items-center gap-1.5 text-sm text-emerald-600 mt-3">
            <Banknote class="w-4 h-4" />RM {selectedGuest.angbaoAmount}
          </div>
        {/if}
        {#if selectedGuest.checkedIn && selectedGuest.giftItem}
          <div class="flex items-center gap-1.5 text-sm text-gold mt-1">
            <Gift class="w-4 h-4" />{selectedGuest.giftItem}
          </div>
        {/if}
      </div>

      <!-- Panel Footer -->
      <div class="mt-auto px-5 py-4 border-t border-gray-100 bg-gray-50/50">
        {#if selectedGuest.checkedIn}
          <span class="w-full flex items-center justify-center gap-1.5 px-4 py-2.5 bg-emerald-50 text-emerald-700 rounded-xl text-sm font-semibold border border-emerald-200">
            <CheckCircle2 class="w-4 h-4" /> Already Checked In
          </span>
        {:else}
          <button
            onclick={() => openCheckinModal(selectedGuest!.id)}
            class="w-full flex items-center justify-center gap-1.5 px-4 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors"
          >
            <UserCheck class="w-4 h-4" /> Check In
          </button>
        {/if}
      </div>
    </div>
  {/if}
</div>

{#if showCheckinModal && checkinGuest}
  {@const table = tables.find(t => t.id === checkinGuest!.tableId)}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="absolute inset-0 bg-black/40 backdrop-blur-sm" onclick={closeModal} role="presentation"></div>

    <div class="relative bg-white rounded-2xl shadow-2xl w-full max-w-md overflow-hidden">
      {#if showSeatView}
        <div class="flex items-center justify-between p-5 border-b border-gray-100">
          <div class="flex items-center gap-3">
            <div class="w-11 h-11 rounded-full bg-emerald-50 border-2 border-emerald-300 flex items-center justify-center">
              <CheckCircle2 class="w-5 h-5 text-emerald-600" />
            </div>
            <div>
              <h3 class="font-bold text-gray-900">{checkinGuest.name}</h3>
              <p class="text-sm text-emerald-600 font-medium">Checked In Successfully</p>
            </div>
          </div>
          <button onclick={closeModal} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
            <X class="w-4 h-4 text-gray-400" />
          </button>
        </div>

        <div class="p-5 space-y-4">
          {#if checkinGuest.tableId}
            <div class="text-center">
              <div class="text-sm text-gray-500 mb-1">Your Table</div>
              <div class="text-4xl font-extrabold text-red">{table?.name || checkinGuest.tableId}</div>
              {#if table?.isVip}
                <span class="text-gold font-semibold text-sm">★ VIP Table</span>
              {/if}
              <div class="text-sm text-gray-500 mt-1">
                Seats {checkinGuest.seatNumber}–{(checkinGuest.seatNumber ?? 0) + checkinGuest.pax - 1}
                · {checkinGuest.pax} pax
              </div>
            </div>

            {@const seats = getSeatOccupants(checkinGuest.tableId, table?.capacity ?? 10)}
            <div class="grid grid-cols-5 gap-2">
              {#each seats as { seatNum, guest }}
                {@const isOwn = seatNum >= (checkinGuest.seatNumber ?? 0) && seatNum < (checkinGuest.seatNumber ?? 0) + checkinGuest.pax}
                <div class={cn(
                  "aspect-square rounded-lg flex items-center justify-center text-[11px] font-bold border-2 transition-colors",
                  isOwn ? "bg-red text-white border-red" :
                  guest ? "bg-gray-100 text-gray-500 border-gray-200" :
                  "bg-gray-50 text-gray-300 border-gray-100"
                )}>
                  {seatNum}
                </div>
              {/each}
            </div>

            <button
              onclick={viewOnMap}
              class="w-full py-3 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2"
            >
              <MapPinned class="w-4 h-4" /> View on Seating Map
            </button>
          {:else}
            <div class="text-center py-4 text-gray-500">
              <p class="font-medium">No table assigned yet</p>
              <p class="text-sm mt-1">Please see the reception desk for seating.</p>
            </div>
          {/if}
        </div>

        <div class="p-5 pt-0">
          <button
            onclick={closeModal}
            class="w-full py-2.5 border border-gray-200 bg-white rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors"
          >
            Close
          </button>
        </div>

      {:else}
        <div class="flex items-center justify-between p-5 border-b border-gray-100">
          <div class="flex items-center gap-3">
            <div class={cn(
              "w-11 h-11 rounded-full flex items-center justify-center text-sm font-bold",
              checkinGuest.isVip ? "bg-gold-50 text-gold border-2 border-gold-300" : "bg-red-50 text-red border-2 border-red-200"
            )}>
              {getInitials(checkinGuest.name)}
            </div>
            <div>
              <h3 class="font-bold text-gray-900">{checkinGuest.name}</h3>
               <p class="text-sm text-gray-500">{table?.name || `Table ${checkinGuest.tableId}`} · {checkinGuest.pax} pax</p>
            </div>
          </div>
          <button onclick={closeModal} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
            <X class="w-4 h-4 text-gray-400" />
          </button>
        </div>

        <div class="p-5 space-y-4">
          <p class="text-sm text-gray-600">Record gift details for this guest's check-in.</p>

          <div>
            <label for="angbao" class="flex items-center gap-1.5 text-sm font-semibold text-gray-700 mb-1.5">
              <Banknote class="w-4 h-4 text-emerald-600" /> Angbao Amount (RM)
            </label>
            <input
              id="angbao"
              type="number"
              min="0"
              step="10"
              bind:value={angbaoAmount}
              placeholder="e.g. 200"
              class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all"
            />
          </div>

          <div>
            <label for="gift" class="flex items-center gap-1.5 text-sm font-semibold text-gray-700 mb-1.5">
              <Gift class="w-4 h-4 text-gold" /> Gift Item
            </label>
            <input
              id="gift"
              type="text"
              bind:value={giftItem}
              placeholder="e.g. Gold bracelet, Red packet, etc."
              class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all"
            />
          </div>
        </div>

        <div class="flex gap-3 p-5 pt-0">
          <button
            onclick={handleCheckIn}
            disabled={checkingIn}
            class="flex-1 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2 disabled:opacity-50"
          >
            {#if checkingIn}
              <Loader2 class="w-4 h-4 animate-spin" /> Processing...
            {:else}
              <CheckCircle2 class="w-4 h-4" /> Confirm Check-In
            {/if}
          </button>
          <button
            onclick={closeModal}
            class="px-5 py-2.5 border border-gray-200 bg-white rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors"
          >
            Cancel
          </button>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  @keyframes slideIn {
    from { transform: translateX(100%); opacity: 0; }
    to { transform: translateX(0); opacity: 1; }
  }
</style>
