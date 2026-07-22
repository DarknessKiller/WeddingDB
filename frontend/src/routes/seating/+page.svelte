<script lang="ts">
  import HallMap from '$lib/components/seating/HallMap.svelte';
  import { getTableDefinitions } from '$lib/constants';
  import { getGuestsByTable, getTableOccupancy } from '$lib/mock/data';
  import { selectedGuest, isDrawerOpen, addToast } from '$lib/stores';
  import Badge from '$lib/components/ui/Badge.svelte';
  import { getInitials, cn } from '$lib/utils';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { Users, Star, X, ChevronUp } from 'lucide-svelte';
  import type { Guest } from '$lib/types';

  let selectedTableId = $state<number | null>(null);
  let hoveredSeat = $state<{ seatNum: number; guest: Guest | null; x: number; y: number } | null>(null);
  let showMobilePanel = $state(false);

  // Auto-select table from query param (e.g. /seating?table=5)
  onMount(() => {
    const tableParam = page.url.searchParams.get('table');
    if (tableParam) {
      const tableId = parseInt(tableParam, 10);
      if (!isNaN(tableId) && getTableDefinitions().some(t => t.id === tableId)) {
        selectedTableId = tableId;
        showMobilePanel = true;
      }
    }
  });

  let selectedTable = $derived(selectedTableId ? getTableDefinitions().find(t => t.id === selectedTableId) ?? null : null);
  let selectedTableGuests = $derived(selectedTableId ? getGuestsByTable(selectedTableId) : []);
  let selectedOccupancy = $derived(selectedTableId ? getTableOccupancy(selectedTableId) : null);

  function handleTableClick(id: number) {
    selectedTableId = selectedTableId === id ? null : id;
    showMobilePanel = selectedTableId !== null;
  }

  function handleSeatClick(tableId: number, seatNum: number, guest: Guest | null) {
    if (guest) {
      $selectedGuest = guest;
      $isDrawerOpen = true;
    }
  }

  function closePanel() {
    selectedTableId = null;
    showMobilePanel = false;
  }
</script>

<svelte:head><title>Seating Map – WeddingDB</title></svelte:head>

<div class="flex h-[calc(100dvh-56px)] sm:h-[calc(100dvh-64px)]">
  <!-- Map -->
  <HallMap
    selectedTableId={selectedTableId}
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
            {selectedTable.id}
          </div>
          <div>
            <div class="font-semibold text-gray-900">Table {selectedTable.id}</div>
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
          {@const guest = selectedTableGuests.find(g => g.seatNumber === seatNum)}
          <button
            class="w-full flex items-center gap-3 p-2.5 rounded-xl transition-all duration-150 text-left {guest ? 'hover:bg-gray-50' : 'hover:bg-gray-50/50'} {hoveredSeat?.seatNum === seatNum ? 'bg-gold-50 ring-1 ring-gold-200' : ''}"
            onclick={() => { if (guest) { $selectedGuest = guest; $isDrawerOpen = true; } }}
          >
            <div class={cn(
              "w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold flex-shrink-0 border-2 transition-colors",
              guest?.checkedIn ? "bg-emerald-50 border-emerald-400 text-emerald-700" :
              guest ? "bg-red-50 border-red text-red" :
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
              <span class="text-xs text-gray-400 italic">Empty seat</span>
            {/if}
          </button>
        {/each}
      </div>

      <!-- Panel Footer -->
      <div class="px-5 py-4 border-t border-gray-100 bg-gray-50/50 flex gap-2">
        <button class="flex-1 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2">
          <Users class="w-4 h-4" /> Assign Guest
        </button>
        <button class="px-4 py-2.5 border border-gray-200 bg-white rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
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
            {selectedTable.id}
          </div>
          <div>
            <div class="font-semibold text-gray-900 text-sm">Table {selectedTable.id}</div>
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
            {@const guest = selectedTableGuests.find(g => g.seatNumber === seatNum)}
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
