<script lang="ts">
  import type { Guest, RSVPStatus } from '$lib/types';
  import { selectedGuest, isDrawerOpen, drawerStartEditing, addToast } from '$lib/stores';
  import { weddingId } from '$lib/stores/weddingId';
  import { listGuests, deleteGuest as apiDeleteGuest, searchGuests, checkInGuest, checkOutGuest, assignSeat } from '$lib/api/guests';
  import type { GuestResponse } from '$lib/api/guests';
  import { listTables } from '$lib/api/tables';
  import type { BanquetTable } from '$lib/types';
  import Badge from '$lib/components/ui/Badge.svelte';
  import { getInitials } from '$lib/utils';
  import {
    Search, Download, Upload, Plus, ChevronLeft, ChevronRight,
    MoreHorizontal, Pencil, Trash2, ArrowUpDown, CheckCircle, XCircle
  } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let searchQuery = $state('');
  let rsvpFilter = $state<RSVPStatus | 'all'>('all');
  let currentPage = $state(0);
  let pageSize = $state(20);
  let sortCol = $state('name');
  let sortDir = $state<'asc' | 'desc'>('asc');
  let selectedIds = $state<Set<string>>(new Set());
  let contextMenu = $state<{ x: number; y: number; guest: GuestResponse } | null>(null);
  let menuWidth = 180;
  let menuHeight = 200;
  let guests = $state<GuestResponse[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let showMoveModal = $state(false);
  let moveGuest = $state<GuestResponse | null>(null);
  let moveTableId = $state('');
  let moveSeatNum = $state(1);
  let moveTables = $state<BanquetTable[]>([]);
  let moveSaving = $state(false);

  let wid = $state('MQ');

  weddingId.subscribe(v => { wid = v; });

  let tables = $state<BanquetTable[]>([]);

  onMount(async () => {
    await Promise.all([loadGuests(), loadTables()]);
  });

  async function loadTables() {
    try {
      tables = await listTables(wid);
    } catch {}
  }

  async function loadGuests() {
    loading = true;
    error = null;
    try {
      const data = await listGuests(wid);
      guests = data.guests;
    } catch (e: any) {
      error = e.message ?? 'Failed to load guests';
      addToast(error!, 'error');
    } finally {
      loading = false;
    }
  }

  async function handleSearch() {
    if (!searchQuery.trim()) {
      await loadGuests();
      return;
    }
    loading = true;
    try {
      guests = await searchGuests(wid, searchQuery);
    } catch (e: any) {
      addToast(e.message ?? 'Search failed', 'error');
    } finally {
      loading = false;
    }
  }

  function toGuest(r: GuestResponse): Guest {
    return {
      id: r.id,
      name: r.name,
      phone: r.phone,
      email: r.email || undefined,
      rsvp: r.rsvp as RSVPStatus,
      pax: r.pax,
      tableId: r.tableId ? parseInt(r.tableId, 10) : null,
      seatNumber: r.seatNum,
      checkedIn: !!r.checkedInAt,
      checkedInAt: r.checkedInAt ? new Date(r.checkedInAt) : undefined,
      notes: r.notes,
      dietaryRequirements: r.dietary ?? [],
      isVip: r.isVip,
      angbaoAmount: r.angbaoAmt ?? undefined,
      giftItem: r.giftItem ?? undefined,
      createdAt: new Date(r.createdAt),
    };
  }

  const columns = [
    { key: 'name', label: 'Name' },
    { key: 'phone', label: 'Phone' },
    { key: 'rsvp', label: 'RSVP' },
    { key: 'pax', label: 'Pax' },
    { key: 'tableId', label: 'Table' },
    { key: 'seatNum', label: 'Seat' },
    { key: 'checkedInAt', label: 'Check In' },
  ] as const;

  let filtered = $derived.by(() => {
    let r = [...guests];
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      r = r.filter(g => g.name.toLowerCase().includes(q) || g.phone.includes(q));
    }
    if (rsvpFilter !== 'all') r = r.filter(g => g.rsvp === rsvpFilter);
    r.sort((a, b) => {
      const av = a[sortCol as keyof GuestResponse] ?? '';
      const bv = b[sortCol as keyof GuestResponse] ?? '';
      const c = String(av).localeCompare(String(bv));
      return sortDir === 'asc' ? c : -c;
    });
    return r;
  });

  let totalPages = $derived(Math.ceil(filtered.length / pageSize));
  let page = $derived(filtered.slice(currentPage * pageSize, (currentPage + 1) * pageSize));

  function toggleSort(col: string) {
    if (sortCol === col) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
    else { sortCol = col; sortDir = 'asc'; }
  }

  function toggleSelectAll() {
    selectedIds = selectedIds.size === page.length ? new Set() : new Set(page.map(g => g.id));
  }

  function toggleSelect(id: string) {
    const n = new Set(selectedIds);
    n.has(id) ? n.delete(id) : n.add(id);
    selectedIds = n;
  }

  function openGuest(guest: GuestResponse) {
    $selectedGuest = toGuest(guest);
    $isDrawerOpen = true;
  }

  function handleCtx(e: MouseEvent, guest: GuestResponse) {
    e.preventDefault();
    contextMenu = { x: e.clientX, y: e.clientY, guest };
  }

  async function deleteGuest(guest: GuestResponse) {
    try {
      await apiDeleteGuest(wid, guest.id);
      guests = guests.filter(g => g.id !== guest.id);
      addToast(`${guest.name} deleted`, 'info');
    } catch (e: any) {
      addToast(e.message ?? 'Delete failed', 'error');
    }
    contextMenu = null;
  }

  function getMenuStyle(x: number, y: number): string {
    const vw = typeof window !== 'undefined' ? window.innerWidth : 1024;
    const vh = typeof window !== 'undefined' ? window.innerHeight : 768;
    const left = x + menuWidth > vw ? x - menuWidth : x;
    const top = y + menuHeight > vh ? y - menuHeight : y;
    return `left: ${Math.max(0, left)}px; top: ${Math.max(0, top)}px;`;
  }

  async function toggleCheckIn(guest: GuestResponse) {
    try {
      if (guest.checkedInAt) {
        await checkOutGuest(wid, guest.id);
        guests = guests.map(g => g.id === guest.id ? { ...g, checkedInAt: null } : g);
        addToast(`${guest.name} checked out`, 'info');
      } else {
        await checkInGuest(wid, guest.id);
        guests = guests.map(g => g.id === guest.id ? { ...g, checkedInAt: new Date().toISOString() } : g);
        addToast(`${guest.name} checked in`, 'success');
      }
    } catch (e: any) {
      addToast(e.message ?? 'Operation failed', 'error');
    }
    contextMenu = null;
  }

  async function openMoveTable(guest: GuestResponse) {
    moveGuest = guest;
    contextMenu = null;
    try {
      moveTables = await listTables(wid);
    } catch (e: any) {
      addToast(e.message ?? 'Failed to load tables', 'error');
      return;
    }
    // pre-select current table if guest has one
    moveTableId = guest.tableId ?? (moveTables.length ? String(moveTables[0].id) : '');
    moveSeatNum = getNextSeatNum();
    showMoveModal = true;
  }

  function getNextSeatNum(): number {
    if (!moveTableId) return 1;
    const occ = guests.filter(g => g.tableId === moveTableId && g.id !== moveGuest?.id);
    if (!occ.length) return 1;
    const maxSeat = Math.max(...occ.map(g => g.seatNum ?? 0));
    return maxSeat + 1;
  }

  function getOccupiedSeats(): Set<number> {
    if (!moveTableId) return new Set();
    return new Set(
      guests
        .filter(g => g.tableId === moveTableId && g.id !== moveGuest?.id && g.seatNum != null)
        .flatMap(g => {
          const start = g.seatNum!;
          return Array.from({ length: g.pax }, (_, i) => start + i);
        })
    );
  }

  function isSeatOccupied(seatNum: number): boolean {
    return getOccupiedSeats().has(seatNum);
  }

  function getTableCapacity(): number {
    if (!moveTableId) return 10;
    const t = moveTables.find(t => String(t.id) === moveTableId);
    return t?.capacity ?? 10;
  }

  async function confirmMoveTable() {
    if (!moveGuest || !moveTableId) return;
    if (isSeatOccupied(moveSeatNum)) {
      addToast(`Seat ${moveSeatNum} is already occupied`, 'error');
      return;
    }
    if (moveSeatNum < 1 || moveSeatNum > getTableCapacity()) {
      addToast(`Seat must be between 1 and ${getTableCapacity()}`, 'error');
      return;
    }
    moveSaving = true;
    try {
      await assignSeat(wid, moveGuest.id, moveTableId, moveSeatNum);
      guests = guests.map(g => g.id === moveGuest!.id
        ? { ...g, tableId: moveTableId, seatNum: moveSeatNum }
        : g
      );
      addToast(`${moveGuest.name} moved to table`, 'success');
      showMoveModal = false;
    } catch (e: any) {
      addToast(e.message ?? 'Move failed', 'error');
    } finally {
      moveSaving = false;
    }
  }
</script>

<svelte:head><title>Guests – WeddingDB</title></svelte:head>
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="p-4 sm:p-7 max-w-[1400px]" onclick={() => contextMenu = null}>
  <!-- Toolbar -->
  <div class="flex items-center justify-between gap-4 mb-5 flex-wrap">
    <div class="relative flex-1 min-w-[200px] max-w-md">
      <Search class="absolute left-3.5 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400 pointer-events-none" />
      <input
        type="text" placeholder="Search guests..." bind:value={searchQuery}
        onkeydown={(e) => e.key === 'Enter' && handleSearch()}
        class="w-full pl-11 pr-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all"
      />
    </div>
    <div class="flex items-center gap-2">
      <select bind:value={rsvpFilter} class="px-3 py-2 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold outline-none">
        <option value="all">All Status</option>
        <option value="confirmed">Confirmed</option>
        <option value="pending">Pending</option>
        <option value="declined">Declined</option>
        <option value="no_response">No Response</option>
      </select>
      <button class="p-2.5 border border-gray-200 rounded-xl bg-white hover:bg-gray-50 transition-colors" aria-label="Import CSV">
        <Upload class="w-4 h-4 text-gray-600" />
      </button>
      <button class="p-2.5 border border-gray-200 rounded-xl bg-white hover:bg-gray-50 transition-colors" aria-label="Export">
        <Download class="w-4 h-4 text-gray-600" />
      </button>
      <button onclick={() => goto('/reservation')} class="flex items-center gap-2 px-4 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors">
        <Plus class="w-4 h-4" /> Add Guest
      </button>
    </div>
  </div>

  {#if selectedIds.size > 0}
    <div class="mb-4 px-4 py-3 bg-red-50 border border-red-100 rounded-xl flex items-center gap-3 text-sm">
      <span class="font-semibold text-red">{selectedIds.size} selected</span>
      <button class="px-3 py-1.5 bg-white border border-gray-200 rounded-lg text-xs font-medium hover:bg-gray-50">Move Table</button>
      <button class="px-3 py-1.5 bg-white border border-red-200 rounded-lg text-xs font-medium text-red hover:bg-red-50">Delete</button>
    </div>
  {/if}

  {#if loading}
    <div class="flex items-center justify-center py-20 text-gray-400">Loading guests...</div>
  {:else if error}
    <div class="flex items-center justify-center py-20 text-red">{error}</div>
  {:else}
    <!-- Table -->
    <div class="bg-white border border-gray-200 rounded-2xl overflow-hidden">
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="bg-gray-50 border-b border-gray-200">
              <th class="pl-5 pr-3 py-3 text-left">
                <input type="checkbox" checked={selectedIds.size === page.length && page.length > 0} onchange={toggleSelectAll} class="rounded" />
              </th>
              {#each columns as col}
                <th
                  class="px-4 py-3 text-left text-[12px] font-semibold uppercase tracking-wider text-gray-500 cursor-pointer select-none hover:text-gray-700"
                  onclick={() => toggleSort(col.key)}
                >
                  <span class="inline-flex items-center gap-1">{col.label} <ArrowUpDown class="w-3 h-3" /></span>
                </th>
              {/each}
              <th class="px-4 py-3 text-left text-[12px] font-semibold uppercase tracking-wider text-gray-500">Notes</th>
              <th class="px-4 py-3 w-10"></th>
            </tr>
          </thead>
          <tbody>
            {#each page as guest (guest.id)}
              <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
              <tr
                class="border-b border-gray-100 hover:bg-gray-50 cursor-pointer transition-colors {selectedIds.has(guest.id) ? 'bg-red-50' : ''}"
                onclick={() => openGuest(guest)}
                oncontextmenu={(e) => handleCtx(e, guest)}
              >
                <td class="pl-5 pr-3 py-3.5" onclick={(e) => e.stopPropagation()}>
                  <input type="checkbox" checked={selectedIds.has(guest.id)} onchange={() => toggleSelect(guest.id)} class="rounded" />
                </td>
                <td class="px-4 py-3.5">
                  <div class="flex items-center gap-3">
                    <div class="w-8 h-8 rounded-full bg-red-50 flex items-center justify-center text-red text-xs font-bold flex-shrink-0">{getInitials(guest.name)}</div>
                    <span class="font-semibold text-gray-900">{guest.name}</span>
                  </div>
                </td>
                <td class="px-4 py-3.5 text-gray-600">{guest.phone}</td>
                <td class="px-4 py-3.5"><Badge status={guest.rsvp as RSVPStatus} /></td>
                <td class="px-4 py-3.5 text-gray-700 font-medium">{guest.pax}</td>
                <td class="px-4 py-3.5 text-gray-700 font-medium">{tables.find(t => t.id === guest.tableId)?.name || (guest.tableId ?? '—')}</td>
                <td class="px-4 py-3.5 text-gray-700 font-medium">{guest.seatNum ?? '—'}</td>
                <td class="px-4 py-3.5">
                  {#if guest.checkedInAt}
                    <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-emerald-50 text-emerald-700 rounded-full text-xs font-semibold border border-emerald-200">
                      <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span> Yes
                    </span>
                  {:else}
                    <span class="text-gray-400">No</span>
                  {/if}
                </td>
                <td class="px-4 py-3.5 text-gray-500 max-w-[140px] truncate">{guest.notes || '—'}</td>
                <td class="px-4 py-3.5" onclick={(e) => e.stopPropagation()}>
                  <button class="p-1.5 rounded-lg hover:bg-gray-100" aria-label="Actions"
                    onclick={(e) => { e.stopPropagation(); handleCtx(e, guest); }}>
                    <MoreHorizontal class="w-4 h-4 text-gray-400" />
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <!-- Pagination -->
      <div class="px-5 py-4 border-t border-gray-100 flex items-center justify-between text-sm">
        <span class="text-gray-500">
          Showing {currentPage * pageSize + 1}–{Math.min((currentPage + 1) * pageSize, filtered.length)} of {filtered.length}
        </span>
        <div class="flex items-center gap-2">
          <button onclick={() => currentPage = Math.max(0, currentPage - 1)} disabled={currentPage === 0}
            class="p-2 rounded-lg border border-gray-200 hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed" aria-label="Previous page">
            <ChevronLeft class="w-4 h-4" />
          </button>
          {#each Array.from({ length: Math.min(totalPages, 5) }, (_, i) => i) as pi}
            {@const pg = currentPage < 3 ? pi : currentPage - 2 + pi}
            {#if pg < totalPages}
              <button onclick={() => currentPage = pg}
                class="w-9 h-9 rounded-lg text-sm font-medium transition-colors {pg === currentPage ? 'bg-red text-white' : 'hover:bg-gray-50 text-gray-700'}">
                {pg + 1}
              </button>
            {/if}
          {/each}
          <button onclick={() => currentPage = Math.min(totalPages - 1, currentPage + 1)} disabled={currentPage >= totalPages - 1}
            class="p-2 rounded-lg border border-gray-200 hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed" aria-label="Next page">
            <ChevronRight class="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>

<!-- Context Menu -->
{#if contextMenu}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="fixed z-[600] bg-white border border-gray-200 rounded-xl shadow-xl py-1.5 min-w-[180px]"
    style={getMenuStyle(contextMenu.x, contextMenu.y)} onclick={(e) => e.stopPropagation()}>
    <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={() => toggleCheckIn(contextMenu!.guest)}>
      {#if contextMenu!.guest.checkedInAt}
        <XCircle class="w-4 h-4" /> Check Out
      {:else}
        <CheckCircle class="w-4 h-4" /> Check In
      {/if}
    </button>
    <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={() => { $drawerStartEditing = true; openGuest(contextMenu!.guest); contextMenu = null; }}>
      <Pencil class="w-4 h-4" /> Edit
    </button>
    <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={() => openMoveTable(contextMenu!.guest)}>
      <ArrowUpDown class="w-4 h-4" /> Move Table
    </button>
    <hr class="my-1 border-gray-100" />
    <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-red hover:bg-red-50" onclick={() => deleteGuest(contextMenu!.guest)}>
      <Trash2 class="w-4 h-4" /> Delete
    </button>
  </div>
{/if}

<!-- Move Table Modal -->
{#if showMoveModal && moveGuest}
  <div class="fixed inset-0 z-[700] flex items-center justify-center bg-black/40" onclick={() => showMoveModal = false}>
    <div class="bg-white rounded-2xl shadow-2xl w-full max-w-md p-6" onclick={(e) => e.stopPropagation()}>
      <h3 class="text-lg font-semibold text-gray-900 mb-4">Move Table</h3>
      <div class="space-y-3 mb-6">
        <div>
          <label class="block text-xs font-medium text-gray-500 mb-1">Guest</label>
          <div class="px-3 py-2 bg-gray-50 rounded-lg text-sm text-gray-900 font-medium">{moveGuest.name}</div>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-medium text-gray-500 mb-1">Current Table</label>
            <div class="px-3 py-2 bg-gray-50 rounded-lg text-sm text-gray-700">{moveGuest.tableId ?? '—'}</div>
          </div>
          <div>
            <label class="block text-xs font-medium text-gray-500 mb-1">Current Seat</label>
            <div class="px-3 py-2 bg-gray-50 rounded-lg text-sm text-gray-700">{moveGuest.seatNum ?? '—'}</div>
          </div>
        </div>
        <div>
          <label class="block text-xs font-medium text-gray-500 mb-1">New Table</label>
          <select bind:value={moveTableId} onchange={() => { moveSeatNum = getNextSeatNum(); }} class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm bg-white focus:border-gold outline-none">
            {#each moveTables as t}
              <option value={String(t.id)}>{t.name}</option>
            {/each}
          </select>
        </div>
        {#if moveTableId}
          <div>
            <label class="block text-xs font-medium text-gray-500 mb-1">Seat Number (1–{getTableCapacity()})</label>
            <input type="number" min="1" max={getTableCapacity()} bind:value={moveSeatNum}
              class="w-full px-3 py-2 border rounded-lg text-sm bg-white outline-none transition-all {isSeatOccupied(moveSeatNum) ? 'border-red focus:ring-2 focus:ring-red/15' : 'border-gray-200 focus:border-gold focus:ring-2 focus:ring-gold/15'}" />
            {#if isSeatOccupied(moveSeatNum)}
              <p class="mt-1 text-xs text-red flex items-center gap-1">⚠ Seat {moveSeatNum} is occupied</p>
            {/if}
          </div>
          <div class="text-xs text-gray-400">
            Occupied: {getOccupiedSeats().size}/{getTableCapacity()} seats
          </div>
        {/if}
      </div>
      <div class="flex justify-end gap-2">
        <button onclick={() => showMoveModal = false} class="px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg">Cancel</button>
        <button onclick={confirmMoveTable} disabled={!moveTableId || moveSaving || isSeatOccupied(moveSeatNum) || moveSeatNum < 1 || moveSeatNum > getTableCapacity()}
          class="px-4 py-2 text-sm font-medium text-white bg-red rounded-lg hover:bg-red-light disabled:opacity-50 transition-colors">
          {moveSaving ? 'Moving...' : 'Move Guest'}
        </button>
      </div>
    </div>
  </div>
{/if}
