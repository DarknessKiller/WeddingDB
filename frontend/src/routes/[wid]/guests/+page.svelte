<script lang="ts">
  import type { Guest, RSVPStatus } from '$lib/types';
  import { weddingTitle } from '$lib/stores/weddingTitle';
  import { selectedGuest, isDrawerOpen, drawerStartEditing, drawerCreateMode, addToast } from '$lib/stores';
  import { weddingId } from '$lib/stores/weddingId';
  import { guestList } from '$lib/stores/guestEvents';
  import { goto } from '$app/navigation';
  import { deleteGuest as apiDeleteGuest, searchGuests, assignSeat, unassignSeat, bulkImportGuests } from '$lib/api/guests';
  import type { GuestResponse, GuestImportData } from '$lib/api/guests';
  import { getWedding } from '$lib/api/weddings';
  import { listTables } from '$lib/api/tables';
  import type { BanquetTable } from '$lib/types';
  import Badge from '$lib/components/ui/Badge.svelte';
  import ConfirmDialog from '$lib/components/ui/ConfirmDialog.svelte';
  import MoveGuestDrawer from '$lib/components/ui/MoveGuestDrawer.svelte';
  import { getInitials } from '$lib/utils';
  import {
    Search, Download, Upload, Plus, ChevronLeft, ChevronRight,
    MoreHorizontal, Pencil, Trash2, ArrowUpDown,
    FileText, AlertCircle, X, Users
  } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let searchQuery = $state('');
  let rsvpFilter = $state<RSVPStatus | 'all'>('all');
  let currentPage = $state(0);
  let pageSize = $state(20);
  let sortCol = $state<string>('name');
  let sortDir = $state<'asc' | 'desc'>('asc');
  let selectedIds = $state<Set<string>>(new Set());
  let contextMenu = $state<{ x: number; y: number; guest: Guest } | null>(null);
  let menuWidth = 180;
  let menuHeight = 200;
  let showUnassignConfirm = $state(false);
  let unassignTarget = $state<Guest | null>(null);
  let loading = $state(true);
  let errored = $state(false);
  let error = $state<string | null>(null);

  // Server-side search results (pinyin-aware)
  let searchResults = $state<Guest[] | null>(null);
  let searching = $state(false);
  let searchError = $state<string | null>(null);
  let searchSeq = 0; // ponytail: stale-response guard, monotonically increasing

  // Derive guest list from SSE store — updates in real time.
  let allGuests = $derived($guestList);
  let totalGuests = $derived(allGuests.length);

  async function doSearch(q: string) {
    const seq = ++searchSeq;
    try {
      const results = await searchGuests(wid, q);
      if (seq !== searchSeq) return; // stale — discard
      searchResults = results.map(toGuest);
      searchError = null;
      currentPage = 0;
    } catch (cause) {
      if (seq !== searchSeq) return;
      searchResults = [];
      searchError = cause instanceof Error ? cause.message : 'Failed to search guests';
    } finally {
      if (seq === searchSeq) searching = false;
    }
  }

  // Server-side search with debounce (pinyin support)
  let searchTimer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    const q = searchQuery.trim();
    clearTimeout(searchTimer);
    ++searchSeq;
    searchError = null;
    if (!q) {
      searchResults = null;
      searching = false;
      currentPage = 0;
      return;
    }
    searching = true;
    searchTimer = setTimeout(() => { doSearch(q); }, 300);
    return () => clearTimeout(searchTimer);
  });

  let showMoveModal = $state(false);
  let moveGuest = $state<Guest | null>(null);
  let moveTables = $state<BanquetTable[]>([]);
  let moveSaving = $state(false);

  let showImportModal = $state(false);
  let importFile = $state<File | null>(null);
  let importPreview = $state<GuestImportData[]>([]);
  let importError = $state('');
  let importing = $state(false);

  let wid = $state('');

  weddingId.subscribe(v => { wid = v; });

  let prevDrawerOpen = $state(false);
  $effect(() => {
    const isOpen = $isDrawerOpen;
    if (prevDrawerOpen && !isOpen) {
      currentPage = 0;
    }
    prevDrawerOpen = isOpen;
  });

  let showSeatNumbers = $state(true);
  let tables = $state<BanquetTable[]>([]);

  onMount(() => {
    loadTables();
    getWedding(wid).then(w => { showSeatNumbers = w.showSeatNumbers ?? true; }).catch(() => {});
    loading = false;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        contextMenu = null;
        showMoveModal = false;
      }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  async function loadTables() {
    try {
      tables = await listTables(wid);
    } catch {}
  }

  // Search and status filters apply before client-side pagination.
  let filtered = $derived.by(() => {
    const latest = new Map(allGuests.map(g => [g.id, g]));
    let r = searchResults
      ? searchResults.filter(g => latest.has(g.id)).map(g => latest.get(g.id)!)
      : [...allGuests];
    if (rsvpFilter !== 'all') r = r.filter(g => g.rsvp === rsvpFilter);
    r.sort((a, b) => {
      let av: string | number | null, bv: string | number | null;
      switch (sortCol) {
        case 'name': av = a.name; bv = b.name; break;
        case 'phone': av = a.phone; bv = b.phone; break;
        case 'rsvp': av = a.rsvp; bv = b.rsvp; break;
        case 'pax': av = a.pax; bv = b.pax; break;
        case 'tableId': av = a.tableId ?? ''; bv = b.tableId ?? ''; break;
        case 'seatNum': av = a.seatNumber ?? 0; bv = b.seatNumber ?? 0; break;
        case 'checkedInAt': av = a.checkedInAt?.getTime() ?? 0; bv = b.checkedInAt?.getTime() ?? 0; break;
        default: av = a.name; bv = b.name;
      }
      const cmp = av < bv ? -1 : av > bv ? 1 : 0;
      return sortDir === 'asc' ? cmp : -cmp;
    });
    return r;
  });

  // Paginated slice of the already-filtered list.
  let guests = $derived(filtered.slice(currentPage * pageSize, (currentPage + 1) * pageSize));
  let hasNextPage = $derived((currentPage + 1) * pageSize < filtered.length);

  function nextPage() {
    if (!hasNextPage) return;
    currentPage++;
  }

  function prevPage() {
    if (currentPage === 0) return;
    currentPage--;
  }

  let displayGuests = $derived(guests);
  let displayTotal = $derived(filtered.length);
  let totalPages = $derived(Math.ceil(displayTotal / pageSize));

  function toGuest(r: GuestResponse): Guest {
    return {
      id: r.id,
      name: r.name,
      phone: r.phone,
      email: r.email || undefined,
      rsvp: r.rsvp as RSVPStatus,
      pax: r.pax,
      tableId: r.tableId ?? null,
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

  const allColumns = [
    { key: 'name', label: 'Name' },
    { key: 'phone', label: 'Phone' },
    { key: 'rsvp', label: 'RSVP' },
    { key: 'pax', label: 'Pax' },
    { key: 'tableId', label: 'Table' },
    { key: 'seatNum', label: 'Seat' },
    { key: 'checkedInAt', label: 'Check In' },
  ] as const;

  let columns = $derived(
    showSeatNumbers ? allColumns : allColumns.filter(c => c.key !== 'seatNum')
  );

  function exportCSV() {
    const headers = ['Name', 'Phone', 'Email', 'Table', 'Seat', 'Pax', 'RSVP', 'VIP', 'Checked In', 'Angbao', 'Gift', 'Notes'];
    const rows = guests.map(g => {
      const table = tables.find(t => String(t.id) === String(g.tableId));
      return [
        g.name,
        g.phone,
        g.email || '',
        table?.name || '',
        g.seatNumber ?? '',
        g.pax,
        g.rsvp,
        g.isVip ? 'Yes' : 'No',
        g.checkedInAt ? g.checkedInAt.toLocaleString() : '',
        g.angbaoAmount ?? '',
        g.giftItem || '',
        (g.notes || '').replace(/,/g, ';')
      ];
    });
    const csv = [headers, ...rows].map(r => r.join(',')).join('\n');
    const blob = new Blob([csv], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `guests-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  function toggleSort(col: string) {
    if (sortCol === col) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
    else { sortCol = col; sortDir = 'asc'; }
  }

  function toggleSelectAll() {
    selectedIds = selectedIds.size === filtered.length ? new Set() : new Set(filtered.map(g => g.id));
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

  async function deleteGuest(guest: Guest) {
    try {
      await apiDeleteGuest(wid, guest.id);
      guestList.update(list => list.filter(g => g.id !== guest.id));
      addToast(`${guest.name} deleted`, 'info');
    } catch (e: any) {
      addToast(e.message ?? 'Delete failed', 'error');
    }
    contextMenu = null;
  }

  async function unassignGuest(guest: Guest) {
    contextMenu = null;
    unassignTarget = guest;
    showUnassignConfirm = true;
  }

  async function confirmUnassign() {
    if (!unassignTarget) return;
    showUnassignConfirm = false;
    try {
      await unassignSeat(wid, unassignTarget.id);
      guestList.update(list => list.map(g => g.id === unassignTarget!.id ? { ...g, tableId: null, seatNumber: null } : g));
      addToast(`${unassignTarget.name} unassigned from table`, 'success');
    } catch (e: any) {
      addToast(e.message ?? 'Unassign failed', 'error');
    }
    unassignTarget = null;
  }

  function getMenuStyle(x: number, y: number): string {
    const vw = typeof window !== 'undefined' ? window.innerWidth : 1024;
    const vh = typeof window !== 'undefined' ? window.innerHeight : 768;
    const left = x + menuWidth > vw ? x - menuWidth : x;
    const top = y + menuHeight > vh ? y - menuHeight : y;
    return `left: ${Math.max(0, left)}px; top: ${Math.max(0, top)}px;`;
  }

  async function openMoveTable(guest: Guest) {
    moveGuest = guest;
    contextMenu = null;
    try {
      moveTables = await listTables(wid);
    } catch (e: any) {
      addToast(e.message ?? 'Failed to load tables', 'error');
      return;
    }
    showMoveModal = true;
  }

  let occupiedSeats = $derived.by((): Set<number> => {
    if (!moveGuest) return new Set();
    const tid = moveGuest.tableId != null ? String(moveGuest.tableId) : '';
    if (!tid) return new Set();
    return new Set(
      guests
        .filter(g => g.tableId === tid && g.id !== moveGuest?.id && g.seatNumber != null)
        .flatMap(g => {
          const start = g.seatNumber!;
          return Array.from({ length: g.pax }, (_, i) => start + i);
        })
    );
  });

  async function confirmMoveTable(tableId: string, seatNum: number) {
    if (!moveGuest) return;
    const target = moveTables.find(t => String(t.id) === tableId);
    const cap = target?.capacity ?? 10;
    if (seatNum < 1 || seatNum + moveGuest.pax - 1 > cap) {
      addToast(`No room for ${moveGuest.pax} pax starting at seat ${seatNum}`, 'error');
      return;
    }
    moveSaving = true;
    try {
      await assignSeat(wid, moveGuest.id, tableId, seatNum);
      // SSE will update the store automatically.
      addToast(`${moveGuest.name} moved to table`, 'success');
      showMoveModal = false;
    } catch (e: any) {
      addToast(e.message ?? 'Move failed', 'error');
    } finally {
      moveSaving = false;
    }
  }

  function parseCSV(text: string): GuestImportData[] {
    const lines = text.trim().split('\n');
    if (lines.length < 2) return [];
    const headers = lines[0].split(',').map(h => h.trim().toLowerCase());
    const nameIdx = headers.findIndex(h => h === 'name');
    if (nameIdx === -1) return [];
    const phoneIdx = headers.findIndex(h => h === 'phone');
    const emailIdx = headers.findIndex(h => h === 'email');
    const paxIdx = headers.findIndex(h => h === 'pax');
    const rsvpIdx = headers.findIndex(h => h === 'rsvp');
    const vipIdx = headers.findIndex(h => h === 'vip' || h === 'isvip');
    const notesIdx = headers.findIndex(h => h === 'notes');
    const dietaryIdx = headers.findIndex(h => h === 'dietary');
    const tableIdx = headers.findIndex(h => h === 'table' || h === 'tablename');
    const seatIdx = headers.findIndex(h => h === 'seat' || h === 'seatnum' || h === 'seat_number');

    return lines.slice(1).filter(l => l.trim()).map(line => {
      const cols = line.split(',').map(c => c.trim().replace(/^"|"$/g, ''));
      return {
        name: cols[nameIdx] || '',
        phone: phoneIdx >= 0 ? cols[phoneIdx] : undefined,
        email: emailIdx >= 0 ? cols[emailIdx] : undefined,
        pax: paxIdx >= 0 ? parseInt(cols[paxIdx]) || 1 : 1,
        rsvp: rsvpIdx >= 0 ? cols[rsvpIdx] : 'no_response',
        isVip: vipIdx >= 0 ? cols[vipIdx]?.toLowerCase() === 'yes' || cols[vipIdx] === '1' || cols[vipIdx]?.toLowerCase() === 'true' : false,
        notes: notesIdx >= 0 ? cols[notesIdx] : undefined,
        dietary: dietaryIdx >= 0 ? cols[dietaryIdx]?.split(';').map(d => d.trim()).filter(Boolean) : [],
        table: tableIdx >= 0 ? cols[tableIdx] : undefined,
        seat: seatIdx >= 0 ? parseInt(cols[seatIdx]) || undefined : undefined,
      };
    }).filter(g => g.name);
  }

  function handleImportFile(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    importFile = file;
    importError = '';
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const guests = parseCSV(reader.result as string);
        if (guests.length === 0) {
          importError = 'No valid guests found. Make sure CSV has a "name" column.';
          return;
        }
        importPreview = guests;
      } catch {
        importError = 'Failed to parse CSV file.';
      }
    };
    reader.readAsText(file);
  }

  async function handleImport() {
    if (importPreview.length === 0) return;
    importing = true;
    try {
      // Resolve table names to IDs
      const resolved = importPreview.map(g => {
        if (!g.table) return g;
        const match = tables.find(t => t.name.toLowerCase() === g.table!.toLowerCase());
        return { ...g, tableId: match?.id };
      });
      const result = await bulkImportGuests(resolved.map(g => ({
        name: g.name,
        phone: g.phone,
        email: g.email,
        pax: g.pax,
        rsvp: g.rsvp,
        isVip: g.isVip,
        notes: g.notes,
        dietary: g.dietary,
        tableId: g.tableId,
        seatNum: g.seat,
      })));
      addToast(`Imported ${result.imported} guests`, 'success');
      showImportModal = false;
      importPreview = [];
      importFile = null;
      // SSE will update the store with new guests automatically.
    } catch (e: any) {
      addToast(e.message || 'Import failed', 'error');
    } finally {
      importing = false;
    }
  }

  let showBulkMoveModal = $state(false);
  let bulkMoveTableId = $state('');
  let bulkMoveSeatStart = $state(1);
  let bulkMoveSaving = $state(false);

  async function bulkDelete() {
    if (selectedIds.size === 0) return;
    const ids = [...selectedIds];
    try {
      await Promise.all(ids.map(id => apiDeleteGuest(wid, id)));
      guestList.update(list => list.filter(g => !selectedIds.has(g.id)));
      addToast(`Deleted ${ids.length} guests`, 'info');
      selectedIds = new Set();
    } catch (e: any) {
      addToast(e.message ?? 'Bulk delete failed', 'error');
    }
  }

  function openBulkMove() {
    if (selectedIds.size === 0) return;
    bulkMoveTableId = tables.length ? String(tables[0].id) : '';
    bulkMoveSeatStart = getNextBulkSeatNum();
    showBulkMoveModal = true;
  }

  function getNextBulkSeatNum(): number {
    if (!bulkMoveTableId) return 1;
    const cap = getBulkTableCapacity();
    const occ = guests.filter(g => g.tableId === bulkMoveTableId && !selectedIds.has(g.id) && g.seatNumber != null)
      .flatMap(g => Array.from({ length: g.pax }, (_, i) => g.seatNumber! + i));
    for (let s = 1; s <= cap; s++) {
      if (!occ.includes(s)) return s;
    }
    return cap + 1;
  }

  let bulkOccupiedSeats = $derived.by((): Set<number> => {
    if (!bulkMoveTableId) return new Set();
    return new Set(
      guests
        .filter(g => g.tableId === bulkMoveTableId && !selectedIds.has(g.id) && g.seatNumber != null)
        .flatMap(g => {
          const start = g.seatNumber!;
          return Array.from({ length: g.pax }, (_, i) => start + i);
        })
    );
  });

  function getBulkTableCapacity(): number {
    if (!bulkMoveTableId) return 10;
    const t = tables.find(t => t.id === bulkMoveTableId);
    return t?.capacity ?? 10;
  }

  async function confirmBulkMove() {
    if (!bulkMoveTableId || selectedIds.size === 0) return;
    const capacity = getBulkTableCapacity();
    const ids = [...selectedIds];
    const seatsNeeded = ids.reduce((sum, id) => {
      const g = guests.find(g => g.id === id);
      return sum + (g?.pax ?? 1);
    }, 0);
    // reject unless every pax fits after the starting seat
    if (bulkMoveSeatStart + seatsNeeded - 1 > capacity) {
      addToast(`Not enough seats. Need ${seatsNeeded}, available from seat ${bulkMoveSeatStart}: ${capacity - bulkMoveSeatStart + 1}`, 'error');
      return;
    }
    // reject if any seat in the run is occupied
    for (let s = bulkMoveSeatStart; s < bulkMoveSeatStart + seatsNeeded; s++) {
      if (bulkOccupiedSeats.has(s)) {
        addToast(`Seat ${s} is occupied; pick a free starting seat`, 'error');
        return;
      }
    }
    bulkMoveSaving = true;
    try {
      let seat = bulkMoveSeatStart;
      for (const id of ids) {
        const g = guests.find(g => g.id === id);
        if (!g) continue;
        await assignSeat(wid, id, bulkMoveTableId, seat);
        guests = guests.map(gg => gg.id === id ? { ...gg, tableId: bulkMoveTableId, seatNumber: seat } : gg);
        seat += g.pax;
      }
      addToast(`Moved ${ids.length} guests to table`, 'success');
      selectedIds = new Set();
      showBulkMoveModal = false;
    } catch (e: any) {
      addToast(e.message ?? 'Bulk move failed', 'error');
    } finally {
      bulkMoveSaving = false;
    }
  }
</script>

<svelte:head> <title>{$weddingTitle ? `${$weddingTitle} – Guests` : 'Guests – WeddingDB'}</title></svelte:head>
<!-- svelte-ignore a11y_no_static_element_interactions a11y_click_events_have_key_events -->
<div class="p-4 sm:p-7 max-w-[1400px]" onclick={() => contextMenu = null}>
  <!-- Toolbar -->
  <div class="flex items-center justify-between gap-2 sm:gap-4 mb-5 flex-wrap">
    <div class="relative flex-1 min-w-[160px] sm:min-w-[200px] max-w-md">
      <Search class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 sm:w-[18px] sm:h-[18px] text-gray-400 pointer-events-none {searching ? 'animate-pulse' : ''}" />
      <input
        type="text" placeholder="Search guests..." bind:value={searchQuery}
        class="w-full pl-10 sm:pl-11 pr-3 sm:pr-4 py-2.5 sm:py-3 border border-gray-200 rounded-xl text-sm bg-white focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all min-h-[44px]"
      />
    </div>
    <div class="flex items-center gap-1 sm:gap-2 flex-wrap">
      <select bind:value={rsvpFilter} class="px-2 sm:px-3 py-1.5 sm:py-2 border border-gray-200 rounded-xl text-xs sm:text-sm bg-white focus:border-gold outline-none">
        <option value="all">All Status</option>
        <option value="confirmed">Confirmed</option>
        <option value="pending">Pending</option>
        <option value="declined">Declined</option>
        <option value="no_response">No Response</option>
      </select>
      <button onclick={() => { showImportModal = true; importPreview = []; importFile = null; importError = ''; }}
        class="px-2 sm:px-3 py-1.5 sm:py-2.5 bg-gray-100 hover:bg-gray-200 text-gray-700 rounded-xl text-xs sm:text-sm font-semibold transition-colors flex items-center gap-1 sm:gap-1.5">
        <Upload class="w-3.5 h-3.5 sm:w-4 sm:h-4" /> <span class="hidden sm:inline">Import CSV</span><span class="sm:hidden">Import</span>
      </button>
      <button onclick={exportCSV} class="px-2 sm:px-3 py-1.5 sm:py-2.5 bg-gray-100 hover:bg-gray-200 text-gray-700 rounded-xl text-xs sm:text-sm font-semibold transition-colors flex items-center gap-1 sm:gap-1.5">
        <Download class="w-3.5 h-3.5 sm:w-4 sm:h-4" /> <span class="hidden sm:inline">Export CSV</span><span class="sm:hidden">Export</span>
      </button>
      <button onclick={() => { $drawerCreateMode = true; $isDrawerOpen = true; }} class="flex items-center gap-1 sm:gap-2 px-2.5 sm:px-4 py-1.5 sm:py-2.5 bg-red text-white rounded-xl text-xs sm:text-sm font-semibold hover:bg-red-light transition-colors">
        <Plus class="w-3.5 h-3.5 sm:w-4 sm:h-4" /> <span class="hidden sm:inline">Add Guest</span><span class="sm:hidden">Add</span>
      </button>
    </div>
  </div>

  {#if selectedIds.size > 0}
    <div class="mb-4 px-4 py-3 bg-red-50 border border-red-100 rounded-xl flex items-center gap-3 text-sm">
      <span class="font-semibold text-red">{selectedIds.size} selected</span>
      <button onclick={openBulkMove} class="px-3 py-1.5 bg-white/90 border border-black/[0.06] rounded-lg text-xs font-medium hover:bg-gray-50">Move Table</button>
      <button onclick={bulkDelete} class="px-3 py-1.5 bg-white border border-red-200 rounded-lg text-xs font-medium text-red hover:bg-red-50">Delete</button>
    </div>
  {/if}

  {#if loading}
    <div class="flex items-center justify-center py-20 text-gray-400">Loading guests...</div>
  {:else if errored}
    <div class="flex flex-col items-center justify-center py-20 text-center">
      <div class="w-16 h-16 bg-red-50 rounded-full flex items-center justify-center mb-4">
        <AlertCircle class="w-8 h-8 text-red" />
      </div>
      <p class="text-red font-medium">Failed to load guests</p>
      <p class="text-sm text-gray-500 mt-1 mb-4">{error}</p>
      <button onclick={() => { currentPage = 0; }} class="px-4 py-2 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors">
        Retry
      </button>
    </div>
  {:else if searchError}
    <div class="flex flex-col items-center justify-center py-20 text-center">
      <AlertCircle class="w-8 h-8 text-red mb-4" />
      <p class="text-red font-medium">Search failed</p>
      <p class="text-sm text-gray-500 mt-1">{searchError}</p>
    </div>
  {:else if guests.length === 0}
    <div class="flex flex-col items-center justify-center py-20 text-center">
      <div class="w-16 h-16 bg-gray-100 rounded-full flex items-center justify-center mb-4">
        <Users class="w-8 h-8 text-gray-400" />
      </div>
      <p class="text-gray-500 font-medium">No guests yet</p>
      <p class="text-sm text-gray-400 mt-1 mb-4">Add your first guest to get started.</p>
      <button onclick={() => { $drawerCreateMode = true; $isDrawerOpen = true; }} class="px-4 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center gap-2">
        <Plus class="w-4 h-4" /> Add Guest
      </button>
    </div>
  {:else}
    <!-- Table -->
    <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl overflow-hidden">
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="bg-gray-50 border-b border-gray-200">
              <th class="pl-5 pr-3 py-3 text-left">
                <input type="checkbox" checked={selectedIds.size === filtered.length && filtered.length > 0} onchange={toggleSelectAll} class="rounded" />
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
            {#each displayGuests as guest (guest.id)}
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
                {#if showSeatNumbers}
                  <td class="px-4 py-3.5 text-gray-700 font-medium">{guest.seatNumber ?? '—'}</td>
                {/if}
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
          Page {currentPage + 1}{totalPages > 0 ? ` of ${totalPages}` : ''} · {displayTotal} guests
        </span>
        <div class="flex items-center gap-2">
          <button onclick={prevPage} disabled={currentPage === 0}
            class="p-2 rounded-lg border border-gray-200 hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed" aria-label="Previous page">
            <ChevronLeft class="w-4 h-4" />
          </button>
          <button onclick={nextPage} disabled={!hasNextPage}
            class="p-2 rounded-lg border border-gray-200 hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed" aria-label="Next page">
            <ChevronRight class="w-4 h-4" />
          </button>
      </div>
    </div>
  </div>
{/if}

<!-- Bulk Move Table Modal -->
<!-- svelte-ignore a11y_no_static_element_interactions a11y_click_events_have_key_events -->
{#if showBulkMoveModal}
  <div class="fixed inset-0 z-[700] flex items-center justify-center bg-black/30 backdrop-blur-md" onclick={() => showBulkMoveModal = false}>
    <!-- svelte-ignore a11y_no_static_element_interactions a11y_click_events_have_key_events -->
    <div class="bg-white/95 backdrop-blur-xl rounded-2xl shadow-2xl w-full max-w-md p-6" onclick={(e) => e.stopPropagation()}>
      <h3 class="text-lg font-semibold text-gray-900 mb-4">Move {selectedIds.size} Guests</h3>
      <div class="space-y-3 mb-6">
        <div>
          <label for="bulk-move-table" class="block text-xs font-medium text-gray-500 mb-1">New Table</label>
          <select id="bulk-move-table" bind:value={bulkMoveTableId} onchange={() => { bulkMoveSeatStart = getNextBulkSeatNum(); }} class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm bg-white focus:border-gold outline-none">
            {#each tables as t}
              <option value={String(t.id)}>{t.name}</option>
            {/each}
          </select>
        </div>
        {#if bulkMoveTableId}
          <div>
            <label for="bulk-move-seat" class="block text-xs font-medium text-gray-500 mb-1">Starting Seat Number (1–{getBulkTableCapacity()})</label>
            <input id="bulk-move-seat" type="number" min="1" max={getBulkTableCapacity()} bind:value={bulkMoveSeatStart}
              class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all" />
          </div>
          <div class="text-xs text-gray-400">
            Occupied: {bulkOccupiedSeats.size}/{getBulkTableCapacity()} seats
          </div>
        {/if}
      </div>
      <div class="flex justify-end gap-2">
        <button onclick={() => showBulkMoveModal = false} class="px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg">Cancel</button>
        <button onclick={confirmBulkMove} disabled={!bulkMoveTableId || bulkMoveSaving}
          class="px-4 py-2 text-sm font-medium text-white bg-red rounded-lg hover:bg-red-light disabled:opacity-50 transition-colors">
          {bulkMoveSaving ? 'Moving...' : `Move ${selectedIds.size} Guests`}
        </button>
      </div>
    </div>
  </div>
{/if}
</div>

<!-- Context Menu -->
{#if contextMenu}
<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="fixed inset-0 z-[599]" onclick={() => contextMenu = null} role="presentation"></div>
  <!-- svelte-ignore a11y_no_static_element_interactions a11y_click_events_have_key_events -->
  <div class="fixed z-[600] bg-white/95 backdrop-blur-xl border border-black/[0.06] rounded-xl shadow-xl py-1.5 min-w-[180px]"
    style={getMenuStyle(contextMenu.x, contextMenu.y)} onclick={(e) => e.stopPropagation()}>
    <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={() => { $drawerStartEditing = true; openGuest(contextMenu!.guest); contextMenu = null; }}>
      <Pencil class="w-4 h-4" /> Edit
    </button>
    <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={() => openMoveTable(contextMenu!.guest)}>
      <ArrowUpDown class="w-4 h-4" /> Move Table
    </button>
    {#if contextMenu!.guest.tableId}
      <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={() => unassignGuest(contextMenu!.guest)}>
        <ArrowUpDown class="w-4 h-4" /> Unassign Table
      </button>
    {/if}
    <hr class="my-1 border-gray-100" />
    <button class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-red hover:bg-red-50" onclick={() => deleteGuest(contextMenu!.guest)}>
      <Trash2 class="w-4 h-4" /> Delete
    </button>
  </div>
{/if}

<!-- Move Table Drawer -->
{#if showMoveModal && moveGuest}
  {@const g = moveGuest}
  <MoveGuestDrawer
    guest={g}
    tables={moveTables}
    occupiedSeats={occupiedSeats}
    currentTableName={tables.find(t => t.id === g.tableId)?.name ?? '—'}
    onSave={confirmMoveTable}
    onClose={() => { showMoveModal = false; moveGuest = null; }}
  />
{/if}

{#if showImportModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="absolute inset-0 bg-black/30 backdrop-blur-md" onclick={() => showImportModal = false} role="presentation"></div>
    <div class="relative bg-white/95 backdrop-blur-xl rounded-2xl shadow-2xl w-full max-w-lg overflow-hidden">
      <div class="flex items-center justify-between p-5 border-b border-gray-100">
        <h3 class="font-bold text-gray-900">Import Guests from CSV</h3>
        <button onclick={() => showImportModal = false} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
          <X class="w-4 h-4 text-gray-400" />
        </button>
      </div>
      <div class="p-5 space-y-4">
        <p class="text-sm text-gray-600">Upload a CSV file with columns: <strong>name</strong> (required), phone, email, pax, rsvp, vip, notes, dietary, table, seat</p>

        <div class="border-2 border-dashed border-gray-200 rounded-xl p-6 text-center hover:border-red/50 transition-colors">
          <input type="file" accept=".csv" onchange={handleImportFile} class="hidden" id="csv-input" />
          <label for="csv-input" class="cursor-pointer">
            <FileText class="w-8 h-8 text-gray-400 mx-auto mb-2" />
            {#if importFile}
              <p class="text-sm font-semibold text-gray-900">{importFile.name}</p>
              <p class="text-xs text-gray-500">{importPreview.length} guests found</p>
            {:else}
              <p class="text-sm text-gray-600">Click to select CSV file</p>
            {/if}
          </label>
        </div>

        {#if importError}
          <div class="flex items-center gap-2 text-sm text-red bg-red-50 rounded-xl p-3">
            <AlertCircle class="w-4 h-4 flex-shrink-0" /> {importError}
          </div>
        {/if}

        {#if importPreview.length > 0}
          <div class="max-h-48 overflow-y-auto border border-gray-200 rounded-xl">
            <table class="w-full text-xs">
              <thead><tr class="bg-gray-50 border-b border-gray-200">
                <th class="px-3 py-2 text-left font-semibold text-gray-600">Name</th>
                <th class="px-3 py-2 text-left font-semibold text-gray-600">Phone</th>
                <th class="px-3 py-2 text-left font-semibold text-gray-600">Pax</th>
                <th class="px-3 py-2 text-left font-semibold text-gray-600">VIP</th>
                {#if importPreview.some(g => g.table)}
                  <th class="px-3 py-2 text-left font-semibold text-gray-600">Table</th>
                {/if}
                {#if importPreview.some(g => g.seat)}
                  <th class="px-3 py-2 text-left font-semibold text-gray-600">Seat</th>
                {/if}
              </tr></thead>
              <tbody>
                {#each importPreview.slice(0, 20) as g}
                  <tr class="border-b border-gray-100">
                    <td class="px-3 py-2 font-medium text-gray-900">{g.name}</td>
                    <td class="px-3 py-2 text-gray-500">{g.phone || '—'}</td>
                    <td class="px-3 py-2 text-gray-500">{g.pax}</td>
                    <td class="px-3 py-2 text-gray-500">{g.isVip ? '★' : ''}</td>
                    {#if importPreview.some(g => g.table)}
                      <td class="px-3 py-2 text-gray-500">{g.table || '—'}</td>
                    {/if}
                    {#if importPreview.some(g => g.seat)}
                      <td class="px-3 py-2 text-gray-500">{g.seat ?? '—'}</td>
                    {/if}
                  </tr>
                {/each}
              </tbody>
            </table>
            {#if importPreview.length > 20}
              <p class="text-xs text-gray-400 text-center py-2">...and {importPreview.length - 20} more</p>
            {/if}
          </div>
        {/if}
      </div>
      <div class="flex gap-3 p-5 pt-0">
        <button onclick={handleImport} disabled={importing || importPreview.length === 0}
          class="flex-1 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors disabled:opacity-50 flex items-center justify-center gap-2">
          {#if importing}
            <div class="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin"></div> Importing...
          {:else}
            <Upload class="w-4 h-4" /> Import {importPreview.length} Guests
          {/if}
        </button>
        <button onclick={() => showImportModal = false}
          class="px-5 py-2.5 border border-black/[0.06] bg-white/90 rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
          Cancel
        </button>
      </div>
    </div>
  </div>
{/if}

<ConfirmDialog
  open={showUnassignConfirm}
  title="Unassign Table"
  message={`Remove ${unassignTarget?.name ?? 'this guest'} from their table? They will no longer have an assigned seat.`}
  confirmLabel="Unassign"
  cancelLabel="Cancel"
  variant="warning"
  onConfirm={confirmUnassign}
  onCancel={() => { showUnassignConfirm = false; unassignTarget = null; }}
/>
