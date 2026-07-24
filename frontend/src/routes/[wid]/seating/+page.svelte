<script lang="ts">
  import HallMap from '$lib/components/seating/HallMap.svelte';
  import { selectedGuest, isDrawerOpen, addToast } from '$lib/stores';
  import { weddingId } from '$lib/stores/weddingId';
  import { goto } from '$app/navigation';
  import { listGuests, assignSeat, type GuestResponse } from '$lib/api/guests';
  import { getOccupancy, listTables } from '$lib/api/tables';
  import Badge from '$lib/components/ui/Badge.svelte';
  import { cn } from '$lib/utils';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { Users, Star, X, Search } from 'lucide-svelte';
  import { get } from 'svelte/store';
  import type { BanquetTable, Guest, RSVPStatus, TableOccupancy } from '$lib/types';

  let allGuests = $state<Guest[]>([]);
  let allTables = $state<BanquetTable[]>([]);
  let occupancyData = $state.raw<Map<string, number>>(new Map());
  let loading = $state(true);

  let selectedTableId = $state<string | null>(null);
  let hoveredSeat = $state<{ seatNum: number; guest: Guest | null; x: number; y: number } | null>(null);
  let showMobilePanel = $state(false);
  let guestSearch = $state('');
  let unassignedGuests = $state<Guest[]>([]);
  let assigningSeat = $state<number | null>(null);

  function mapGuest(r: GuestResponse): Guest {
    return {
      id: r.id,
      name: r.name,
      phone: r.phone,
      email: r.email,
      rsvp: (r.rsvp as RSVPStatus) ?? 'no_response',
      pax: r.pax,
      tableId: r.tableId ?? null,
      seatNumber: r.seatNum,
      checkedIn: r.checkedInAt !== null,
      checkedInAt: r.checkedInAt ? new Date(r.checkedInAt) : undefined,
      notes: r.notes,
      dietaryRequirements: r.dietary ?? [],
      isVip: r.isVip,
      angbaoAmount: r.angbaoAmt ?? undefined,
      giftItem: r.giftItem ?? undefined,
      createdAt: new Date(r.createdAt),
    };
  }

  onMount(async () => {
    try {
      const wid = get(weddingId);
      const [guestRes, rawOcc, tablesRes] = await Promise.all([listGuests(wid), getOccupancy(wid), listTables(wid)]);
      allGuests = guestRes.guests.map(mapGuest);
      unassignedGuests = allGuests.filter(g => g.tableId === null);
      allTables = tablesRes;
      const occMap = new Map<string, number>();
      for (const o of rawOcc) occMap.set(o.TableID, o.Pax);
      occupancyData = occMap;
    } catch (e) {
      addToast('Failed to load seating data', 'error');
    } finally {
      loading = false;
      const tableParam = page.url.searchParams.get('table');
      if (tableParam) {
        if (allTables.some(t => t.id === tableParam)) {
          selectedTableId = tableParam;
          showMobilePanel = true;
        }
      }
    }
  });

  let selectedTable = $derived(selectedTableId ? allTables.find(t => t.id === selectedTableId) ?? null : null);
  let selectedTableGuests = $derived(selectedTableId ? allGuests.filter(g => g.tableId === selectedTableId) : []);
  let tableGuests = $derived.by(() => {
    const map = new Map<string, Guest[]>();
    for (const g of allGuests) {
      if (g.tableId === null) continue;
      const arr = map.get(g.tableId) ?? [];
      arr.push(g);
      map.set(g.tableId, arr);
    }
    return map;
  });
  let selectedOccupancy = $derived.by((): TableOccupancy | null => {
    if (!selectedTableId || !selectedTable) return null;
    const occupied = occupancyData.get(selectedTableId) ?? 0;
    const capacity = selectedTable.capacity;
    return { tableId: selectedTableId, occupied, capacity, percentage: capacity > 0 ? Math.round((occupied / capacity) * 100) : 0 };
  });

  let filteredUnassignedGuests = $derived(
    guestSearch.trim()
      ? unassignedGuests.filter(g =>
          g.name.toLowerCase().includes(guestSearch.toLowerCase()) ||
          g.phone.includes(guestSearch)
        )
      : unassignedGuests
  );

  function handleTableClick(id: string) {
    selectedTableId = selectedTableId === id ? null : id;
    showMobilePanel = selectedTableId !== null;
  }

  function handleSeatClick(tableId: string, seatNum: number, guest: Guest | null) {
    if (guest) {
      $selectedGuest = guest;
      $isDrawerOpen = true;
    }
  }

  function closePanel() {
    selectedTableId = null;
    showMobilePanel = false;
    assigningSeat = null;
    guestSearch = '';
  }

  function getInitials(name: string): string {
    return name.split(' ').map(w => w[0]).join('').slice(0, 2).toUpperCase();
  }

  async function assignGuestToSeat(guest: Guest, seatNum: number) {
    if (!selectedTable) return;
    const wid = get(weddingId);
    try {
      await assignSeat(wid, guest.id, String(selectedTable.id), seatNum);
      allGuests = allGuests.map(g => g.id === guest.id
        ? { ...g, tableId: selectedTable.id, seatNumber: seatNum }
        : g
      );
      unassignedGuests = unassignedGuests.filter(g => g.id !== guest.id);
      assigningSeat = null;
      guestSearch = '';
      addToast(`${guest.name} assigned to seat ${seatNum}`, 'success');
    } catch (e: any) {
      addToast(e.message ?? 'Assignment failed', 'error');
    }
  }
</script>

<svelte:head><title>Seating Map – WeddingDB</title></svelte:head>

<div class="flex h-[calc(100dvh-56px)] sm:h-[calc(100dvh-64px)]">
  <!-- Map -->
  <HallMap
    selectedTableId={selectedTableId}
    tableGuests={tableGuests}
    tables={allTables}
    onTableClick={handleTableClick}
    onSeatClick={handleSeatClick}
    bind:hoveredSeat
  />

  <!-- Desktop Side Panel -->
  {#if selectedTable}
    <div class="hidden md:flex w-[340px] bg-white border-l border-gray-200 flex-col overflow-hidden animate-in">
      <!-- Panel Header -->
      <div class="flex items-center justify-between px-5 py-4 border-b border-gray-100">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-xl bg-red-50 border border-red-100 flex items-center justify-center text-red font-bold text-lg">
            {selectedTable.name || selectedTable.id}
          </div>
          <div>
            <div class="font-semibold text-gray-900">{selectedTable.name || `Table ${selectedTable.id}`}</div>
            <div class="text-xs text-gray-500">
              {selectedOccupancy?.occupied ?? 0} of {selectedTable.capacity} seats
              {#if selectedTable.isVip}
                <span class="text-gold font-semibold">• VIP</span>
              {/if}
            </div>
          </div>
        </div>
        <button onclick={closePanel} class="p-2 rounded-lg hover:bg-gray-100 text-gray-400 hover:text-gray-600 transition-colors" aria-label="Close panel">
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- Occupancy Bar -->
      <div class="px-5 py-3 border-b border-gray-100">
        <div class="flex items-center justify-between text-xs text-gray-500 mb-1.5">
          <span>Occupancy</span>
          <span class="font-semibold text-gray-700">{selectedOccupancy?.percentage ?? 0}%</span>
        </div>
        <div class="h-2 bg-gray-100 rounded-full overflow-hidden">
          <div
            class="h-full rounded-full transition-all duration-500"
            style="width: {selectedOccupancy?.percentage ?? 0}%; background-color: {(selectedOccupancy?.percentage ?? 0) >= 90 ? 'var(--color-red)' : (selectedOccupancy?.percentage ?? 0) >= 60 ? 'var(--color-gold)' : '#059669'};"
          ></div>
        </div>
      </div>

      <!-- Seat List -->
      <div class="flex-1 overflow-y-auto px-5 py-3 space-y-1.5">
        <div class="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2">Seat Assignments</div>
        {#each Array(selectedTable.capacity) as _, seatIdx}
          {@const seatNum = seatIdx + 1}
          {@const guest = selectedTableGuests.find(g => g.seatNumber !== null && seatNum >= g.seatNumber && seatNum < g.seatNumber + g.pax)}
          <button
            class="w-full flex items-center gap-3 p-2.5 rounded-xl transition-all duration-150 text-left {guest ? 'hover:bg-gray-50' : 'hover:bg-gray-50/50'} {hoveredSeat?.seatNum === seatNum ? 'bg-gold-50 ring-1 ring-gold-200' : ''}"
            onclick={() => {
              if (guest) {
                $selectedGuest = guest;
                $isDrawerOpen = true;
              } else {
                assigningSeat = seatNum;
              }
            }}
          >
            <div class={cn(
              "w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold flex-shrink-0 border-2 transition-colors",
              guest?.checkedIn ? "bg-emerald-50 border-emerald-400 text-emerald-700" :
              guest ? "bg-red-50 border-red text-red" :
              assigningSeat === seatNum ? "border-dashed border-gold text-gold" :
              "bg-gray-50 border-gray-200 text-gray-400"
            )}>
              {seatNum}
            </div>
            {#if guest}
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-1.5">
                  {#if guest.isVip}
                    <Star class="w-3 h-3 text-gold fill-gold flex-shrink-0" />
                  {/if}
                  <span class="font-semibold text-sm text-gray-900 truncate">{guest.name}</span>
                </div>
                <div class="flex items-center gap-1.5 text-xs text-gray-500">
                  <span>{guest.phone}</span>
                  <span>•</span>
                  <span>{guest.pax} pax</span>
                </div>
              </div>
              <Badge status={guest.rsvp} />
            {:else}
              <span class="text-xs text-gray-400 italic">{assigningSeat === seatNum ? 'Select a guest below...' : 'Click to assign'}</span>
            {/if}
          </button>
        {/each}
      </div>

      <!-- Guest Search (when assigning) -->
      {#if assigningSeat !== null}
        <div class="px-5 py-3 border-t border-gray-100 space-y-2">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-gray-500">Assign Seat {assigningSeat}</span>
            <button onclick={() => assigningSeat = null} class="text-xs text-gray-400 hover:text-gray-600">Cancel</button>
          </div>
          <div class="relative">
            <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
            <input
              type="text"
              placeholder="Search unassigned guests..."
              bind:value={guestSearch}
              class="w-full pl-9 pr-3 py-2 border border-gray-200 rounded-lg text-sm focus:border-gold outline-none"
            />
          </div>
          <div class="max-h-[200px] overflow-y-auto space-y-1">
            {#each filteredUnassignedGuests as guest}
              <button
                onclick={() => { if (assigningSeat !== null) assignGuestToSeat(guest, assigningSeat); }}
                class="w-full flex items-center gap-3 p-2 rounded-lg hover:bg-gray-50 text-left transition-colors"
              >
                <div class="w-8 h-8 rounded-full bg-red-50 text-red flex items-center justify-center text-xs font-bold">
                  {getInitials(guest.name)}
                </div>
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium text-gray-900 truncate">{guest.name}</div>
                  <div class="text-xs text-gray-500">{guest.phone} • {guest.pax} pax</div>
                </div>
              </button>
            {:else}
              <p class="text-xs text-gray-400 text-center py-2">No unassigned guests</p>
            {/each}
          </div>
        </div>
      {/if}

      <!-- Panel Footer -->
      <div class="px-5 py-4 border-t border-gray-100 bg-gray-50/50 flex gap-2">
        <button onclick={() => goto(`/${$weddingId}/tables`)} class="flex-1 py-2.5 border border-gray-200 bg-white rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
          Edit Table
        </button>
      </div>
    </div>
  {/if}

  <!-- Mobile Bottom Panel -->
  {#if selectedTable && showMobilePanel}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="md:hidden fixed inset-x-0 bottom-0 z-40 bg-white border-t border-gray-200 rounded-t-2xl shadow-2xl animate-slide-up" style="max-height: 60vh;">
      <!-- Drag handle -->
      <div class="flex justify-center py-2">
        <div class="w-10 h-1 bg-gray-300 rounded-full"></div>
      </div>

      <!-- Panel Header -->
      <div class="flex items-center justify-between px-4 py-3 border-b border-gray-100">
        <div class="flex items-center gap-3">
          <div class="w-9 h-9 rounded-xl bg-red-50 border border-red-100 flex items-center justify-center text-red font-bold text-sm">
            {selectedTable.name || selectedTable.id}
          </div>
          <div>
            <div class="font-semibold text-gray-900 text-sm">{selectedTable.name || `Table ${selectedTable.id}`}</div>
            <div class="text-xs text-gray-500">
              {selectedOccupancy?.occupied ?? 0}/{selectedTable.capacity}
              {#if selectedTable.isVip}• VIP{/if}
            </div>
          </div>
        </div>
        <button onclick={closePanel} class="p-2 rounded-lg hover:bg-gray-100" aria-label="Close">
          <X class="w-5 h-5 text-gray-400" />
        </button>
      </div>

      <!-- Occupancy -->
      <div class="px-4 py-2 border-b border-gray-100">
        <div class="h-1.5 bg-gray-100 rounded-full overflow-hidden">
          <div class="h-full rounded-full transition-all" style="width: {selectedOccupancy?.percentage ?? 0}%; background-color: {(selectedOccupancy?.percentage ?? 0) >= 90 ? 'var(--color-red)' : '#D4AF37'};"></div>
        </div>
      </div>

      <!-- Seat Grid (compact) -->
      <div class="overflow-y-auto px-4 py-3" style="max-height: calc(60vh - 120px);">
        <div class="grid grid-cols-2 gap-2">
          {#each Array(selectedTable.capacity) as _, seatIdx}
            {@const seatNum = seatIdx + 1}
            {@const guest = selectedTableGuests.find(g => g.seatNumber !== null && seatNum >= g.seatNumber && seatNum < g.seatNumber + g.pax)}
            <button
              class="flex items-center gap-2 p-2 rounded-lg text-left {hoveredSeat?.seatNum === seatNum ? 'bg-gold-50' : guest ? 'bg-gray-50' : 'bg-white border border-gray-100'}"
              onclick={() => { if (guest) { $selectedGuest = guest; $isDrawerOpen = true; } }}
            >
              <div class={cn(
                "w-7 h-7 rounded-full flex items-center justify-center text-[10px] font-bold flex-shrink-0 border-2",
                guest?.checkedIn ? "bg-emerald-50 border-emerald-400 text-emerald-700" :
                guest ? "bg-red-50 border-red text-red" :
                "bg-gray-50 border-gray-200 text-gray-400"
              )}>
                {seatNum}
              </div>
              <div class="flex-1 min-w-0">
                {#if guest}
                  <div class="text-xs font-semibold text-gray-900 truncate">{guest.name}</div>
                  <div class="text-[10px] text-gray-500">{guest.pax} pax</div>
                {:else}
                  <div class="text-[10px] text-gray-400 italic">Empty</div>
                {/if}
              </div>
            </button>
          {/each}
        </div>
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

<style>
  @keyframes slideIn {
    from { transform: translateX(100%); opacity: 0; }
    to { transform: translateX(0); opacity: 1; }
  }
  @keyframes slideUp {
    from { transform: translateY(100%); }
    to { transform: translateY(0); }
  }
  .animate-in {
    animation: slideIn 0.2s ease-out;
  }
  .animate-slide-up {
    animation: slideUp 0.25s ease-out;
  }
</style>
