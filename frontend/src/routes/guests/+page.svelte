<script lang="ts">
  import { guests } from '$lib/mock/data';
  import type { Guest, RSVPStatus } from '$lib/types';
  import { selectedGuest, isDrawerOpen, addToast } from '$lib/stores';
  import Badge from '$lib/components/ui/Badge.svelte';
  import { getInitials } from '$lib/utils';
  import {
    Search, Download, Upload, Plus, ChevronLeft, ChevronRight,
    MoreHorizontal, Pencil, Trash2, ArrowUpDown
  } from 'lucide-svelte';

  let searchQuery = $state('');
  let rsvpFilter = $state<RSVPStatus | 'all'>('all');
  let currentPage = $state(0);
  let pageSize = $state(20);
  let sortCol = $state('name');
  let sortDir = $state<'asc' | 'desc'>('asc');
  let selectedIds = $state<Set<string>>(new Set());
  let contextMenu = $state<{ x: number; y: number; guest: Guest } | null>(null);

  const columns = [
    { key: 'name', label: 'Name' },
    { key: 'phone', label: 'Phone' },
    { key: 'rsvp', label: 'RSVP' },
    { key: 'pax', label: 'Pax' },
    { key: 'tableId', label: 'Table' },
    { key: 'seatNumber', label: 'Seat' },
    { key: 'checkedIn', label: 'Check In' },
  ] as const;

  let filtered = $derived.by(() => {
    let r = [...guests];
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      r = r.filter(g => g.name.toLowerCase().includes(q) || g.phone.includes(q));
    }
    if (rsvpFilter !== 'all') r = r.filter(g => g.rsvp === rsvpFilter);
    r.sort((a, b) => {
      const av = a[sortCol as keyof Guest] ?? '';
      const bv = b[sortCol as keyof Guest] ?? '';
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

  function openGuest(guest: Guest) {
    $selectedGuest = guest;
    $isDrawerOpen = true;
  }

  function handleCtx(e: MouseEvent, guest: Guest) {
    e.preventDefault();
    contextMenu = { x: e.clientX, y: e.clientY, guest };
  }

  function deleteGuest(guest: Guest) {
    addToast(`${guest.name} deleted`, 'info');
    contextMenu = null;
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
      <button class="flex items-center gap-2 px-4 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors">
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
              <td class="px-4 py-3.5"><Badge status={guest.rsvp} /></td>
              <td class="px-4 py-3.5 text-gray-700 font-medium">{guest.pax}</td>
              <td class="px-4 py-3.5 text-gray-700 font-medium">{guest.tableId ?? '—'}</td>
              <td class="px-4 py-3.5 text-gray-700 font-medium">{guest.seatNumber ?? '—'}</td>
              <td class="px-4 py-3.5">
                {#if guest.checkedIn}
                  <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-emerald-50 text-emerald-700 rounded-full text-xs font-semibold border border-emerald-200">
                    <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span> Yes
                  </span>
                {:else}
                  <span class="text-gray-400">No</span>
                {/if}
              </td>
              <td class="px-4 py-3.5 text-gray-500 max-w-[140px] truncate">{guest.notes || '—'}</td>
              <td class="px-4 py-3.5" onclick={(e) => e.stopPropagation()}>
                <button class="p-1.5 rounded-lg hover:bg-gray-100" aria-label="Actions">
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
</div>

<!-- Context Menu -->
{#if contextMenu}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="fixed z-[600] bg-white border border-gray-200 rounded-xl shadow-xl py-1.5 min-w-[180px]"
    style="left: {contextMenu.x}px; top: {contextMenu.y}px;" onclick={(e) => e.stopPropagation()}>
    <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={() => { openGuest(contextMenu!.guest); contextMenu = null; }}>
      <Pencil class="w-4 h-4" /> Edit
    </button>
    <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={() => contextMenu = null}>
      <ArrowUpDown class="w-4 h-4" /> Move Table
    </button>
    <hr class="my-1 border-gray-100" />
    <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-red hover:bg-red-50" onclick={() => deleteGuest(contextMenu!.guest)}>
      <Trash2 class="w-4 h-4" /> Delete
    </button>
  </div>
{/if}
