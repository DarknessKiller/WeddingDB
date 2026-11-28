<script lang="ts">
  import { onDestroy } from 'svelte';
  import { get } from 'svelte/store';
  import type { Guest, RSVPStatus } from '#lib/types/index.ts';
  import { weddingTitle } from '#lib/stores/weddingTitle.ts';
  import { selectedGuest, isDrawerOpen, drawerStartEditing, drawerCreateMode, addToast, showSeatNumbers, weddingTables } from '#lib/stores/index.ts';
  import { weddingId } from '#lib/stores/weddingId.ts';
  import { guestList, guestMap, refreshGuests } from '#lib/stores/guestEvents.ts';
  import { goto } from '$app/navigation';
  import { deleteGuest as apiDeleteGuest, assignSeat, unassignSeat, bulkImportGuests, fetchAllGuests, genId } from '#lib/api/guests.ts';
  import { syncQueue } from '#lib/offline/queue.ts';
  import type { GuestImportData } from '#lib/api/guests.ts';
  import { rankGuests } from '#lib/utils/guestSearch.ts';
  import { parseCsv, csvToGuests, resolveImportTables, type CsvGuestRow } from '#lib/utils/csv.ts';
  import { trapFocus, registerEscape } from '#lib/utils/focus.ts';
  import { listTables } from '#lib/api/tables.ts';
  import type { BanquetTable } from '#lib/types/index.ts';
  import Badge from '#lib/components/ui/Badge.svelte';
  import ConfirmDialog from '#lib/components/ui/ConfirmDialog.svelte';
  import MoveGuestDrawer from '#lib/components/ui/MoveGuestDrawer.svelte';
  import { getInitials } from '#lib/utils/index.ts';
  import { toGuest } from '#lib/utils/guestTransform.ts';
  import { getQueue } from '#lib/offline/queue.ts';
  import Search from 'lucide-svelte/icons/search';
import Download from 'lucide-svelte/icons/download';
import Upload from 'lucide-svelte/icons/upload';
import Plus from 'lucide-svelte/icons/plus';
import MoreHorizontal from 'lucide-svelte/icons/more-horizontal';
import Pencil from 'lucide-svelte/icons/pencil';
import Trash2 from 'lucide-svelte/icons/trash-2';
import ArrowUpDown from 'lucide-svelte/icons/arrow-up-down';
import FileText from 'lucide-svelte/icons/file-text';
import AlertCircle from 'lucide-svelte/icons/alert-circle';
import X from 'lucide-svelte/icons/x';
import Users from 'lucide-svelte/icons/users';

  let searchQuery = $state('');
  let rsvpFilter = $state<RSVPStatus | 'all'>('all');
  // Infinite scroll: rows render in steps of PAGE as the sentinel comes into view.
  const PAGE = 50;
  let visible = $state(PAGE);
  let sortCol = $state<string>('name');
  let sortDir = $state<'asc' | 'desc'>('asc');
  let selectedIds = $state<Set<string>>(new Set());
  let contextMenu = $state<{ x: number; y: number; guest: Guest } | null>(null);
  let contextMenuEl = $state<HTMLElement>();
  let menuReturnFocus: HTMLElement | null = null;
  let menuWidth = 180;
  let menuHeight = 200;
  let showUnassignConfirm = $state(false);
  let unassignTarget = $state<Guest | null>(null);
  let showDeleteConfirm = $state(false);
  let showBulkDeleteConfirm = $state(false);
  let deleteTarget = $state<Guest | null>(null);

  // Derive guest list from SSE store — updates in real time.
  let allGuests = $derived($guestList);
  let totalGuests = $derived(allGuests.length);

  // Back to the first rows whenever the result set changes.
  $effect(() => {
    searchQuery; rsvpFilter;
    visible = PAGE;
  });

  // Grows the rendered rows while the node is in view. Re-observing on each
  // update re-checks a sentinel that is still visible after a short step.
  function loadMoreOnView(node: HTMLElement, _shown: number) {
    const io = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting)) visible += PAGE;
    });
    io.observe(node);
    return {
      update() { io.unobserve(node); io.observe(node); },
      destroy: () => io.disconnect()
    };
  }

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

  // Another wedding's guest ids must not stay selected for bulk delete or move.
  const unsubscribeWedding = weddingId.subscribe(v => { if (v !== wid) selectedIds = new Set(); wid = v; });
  onDestroy(unsubscribeWedding);

  let prevDrawerOpen = $state(false);
  $effect(() => {
    const isOpen = $isDrawerOpen;
    if (prevDrawerOpen && !isOpen) {
      visible = PAGE;
    }
    prevDrawerOpen = isOpen;
  });

  let tables = $derived($weddingTables.wid === wid ? $weddingTables.tables : []);

  // Local search (no request while typing), then the status filter.
  // A query keeps its relevance order; without one the column sort applies.
  let filtered = $derived.by(() => {
    const q = searchQuery.trim();
    let r = q ? rankGuests(allGuests, q) : [...allGuests];
    if (rsvpFilter !== 'all') r = r.filter(g => g.rsvp === rsvpFilter);
    if (q) return r;
    r.sort((a, b) => {
      let av: string | number | null, bv: string | number | null;
      switch (sortCol) {
        case 'name': av = a.name; bv = b.name; break;
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

  let displayGuests = $derived(filtered.slice(0, visible));

  function clearFilters() {
    searchQuery = '';
    rsvpFilter = 'all';
  }

  const allColumns = [
    { key: 'name', label: 'Name' },
    { key: 'rsvp', label: 'RSVP' },
    { key: 'pax', label: 'Pax' },
    { key: 'tableId', label: 'Table' },
    { key: 'seatNum', label: 'Seat' },
    { key: 'checkedInAt', label: 'Check In' },
  ] as const;

  let columns = $derived(
    $showSeatNumbers ? allColumns : allColumns.filter(c => c.key !== 'seatNum')
  );

  // Quote every field: double quotes around the value, internal quotes escaped as "".
  // Excel and Sheets run a cell starting with =, +, -, @ or a control character
  // as a formula, so guest text is prefixed with an apostrophe (numbers are not).
  function csvField(v: unknown): string {
    if (v === null || v === undefined) return '""';
    if (typeof v !== 'string') return `"${v}"`;
    const s = /^[=+\-@\t\r]/.test(v) ? `'${v}` : v;
    return `"${s.replace(/"/g, '""')}"`;
  }

  function exportCSV() {
    const headers = ['Name', 'Email', 'Table', 'Seat', 'Pax', 'RSVP', 'VIP', 'Checked In', 'Angbao', 'Gift', 'Notes'];
    const rows = filtered.map(g => {
      const table = tables.find(t => String(t.id) === String(g.tableId));
      return [
        g.name,
        g.email || '',
        table?.name || '',
        g.seatNumber ?? '',
        g.pax,
        g.rsvp,
        g.isVip ? 'Yes' : 'No',
        g.checkedInAt ? g.checkedInAt.toLocaleString() : '',
        g.angbaoAmount ?? '',
        g.giftItem || '',
        g.notes || ''
      ].map(csvField);
    });
    const csv = [headers.map(csvField), ...rows].map(r => r.join(',')).join('\n');
    // BOM so Excel reads the file as UTF-8 and shows Chinese names; parseCsv strips it.
    const blob = new Blob(['\uFEFF' + csv], { type: 'text/csv' });
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
    menuReturnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    let { clientX: x, clientY: y } = e;
    // Keyboard activation (Enter/Space on the trigger) reports (0,0): anchor
    // the menu to the trigger instead of the top-left corner.
    if (x === 0 && y === 0 && e.currentTarget instanceof Element) {
      const r = e.currentTarget.getBoundingClientRect();
      x = r.left;
      y = r.bottom;
    }
    contextMenu = { x, y, guest };
  }

  function menuKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') { e.stopPropagation(); contextMenu = null; }
    // Tab leaves the menu entirely instead of walking behind the overlay.
    else if (e.key === 'Tab') contextMenu = null;
    else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      const items = [...(contextMenuEl?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])];
      if (!items.length) return;
      const i = items.indexOf(document.activeElement as HTMLElement);
      const next = e.key === 'ArrowDown' ? (i + 1) % items.length : (i <= 0 ? items.length - 1 : i - 1);
      items[next]?.focus();
    }
  }

  // Keyboard path: focus the first item on open, restore the trigger on close
  // unless a dialog (drawer/confirm) took over focus.
  $effect(() => {
    if (contextMenu) {
      contextMenuEl?.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
    } else if (menuReturnFocus) {
      if (!document.querySelector('[role="dialog"]')) menuReturnFocus.focus();
      menuReturnFocus = null;
    }
  });

  async function deleteGuest(guest: Guest) {
    try {
      await apiDeleteGuest(wid, guest.id);
      addToast(`${guest.name} deleted`, 'info');
    } catch (e: any) {
      addToast(e.message ?? 'Delete failed', 'error');
    }
    contextMenu = null;
  }

  function requestDelete(guest: Guest) {
    contextMenu = null;
    deleteTarget = guest;
    showDeleteConfirm = true;
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    showDeleteConfirm = false;
    await deleteGuest(deleteTarget);
    deleteTarget = null;
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

  async function confirmMoveTable(tableId: string, seatNum: number) {
    if (!moveGuest) return;
    // Capture the guest: closing the drawer clears moveGuest while the request runs.
    const guest = moveGuest;
    const target = moveTables.find(t => String(t.id) === tableId);
    const cap = target?.capacity ?? 10;
    if (seatNum < 1 || seatNum + guest.pax - 1 > cap) {
      addToast(`No room for ${guest.pax} pax starting at seat ${seatNum}`, 'error');
      return;
    }
    moveSaving = true;
    try {
      await assignSeat(wid, guest.id, tableId, seatNum);
      // SSE will update the store automatically.
      addToast(`${guest.name} moved to table`, 'success');
      showMoveModal = false;
    } catch (e: any) {
      addToast(e.message ?? 'Move failed', 'error');
    } finally {
      moveSaving = false;
    }
  }

  function parseCSV(text: string): { guests: CsvGuestRow[]; dropped: number } {
    return csvToGuests(parseCsv(text));
  }

  let importReadId = 0;

  function hasQueuedEdits(w: string): boolean {
    return getQueue(w).length > 0;
  }

  function handleImportFile(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    importFile = file;
    importError = '';
    // A failed read must not leave the previous file's rows importable.
    importPreview = [];
    const readId = ++importReadId;
    const reader = new FileReader();
    reader.onload = () => {
      if (readId !== importReadId) return;
      try {
        const { guests, dropped } = parseCSV(reader.result as string);
        if (guests.length === 0) {
          importError = 'No valid guests found. Make sure CSV has a "name" column and values within the field limits.';
          importPreview = [];
          return;
        }
        importError = dropped > 0 ? `${dropped} row(s) skipped: missing name or a value over the field limit.` : '';
        // IDs are minted once per file, so pressing Import again after a lost
        // response skips the rows the server already wrote.
        importPreview = guests.map(g => ({ ...g, id: genId() }));
      } catch {
        importError = 'Failed to parse CSV file.';
      }
    };
    reader.onerror = () => {
      if (readId !== importReadId) return;
      importError = 'Failed to read CSV file.';
    };
    // Reset so choosing the same file again fires change after a failed parse.
    input.value = '';
    reader.readAsText(file);
  }

  async function handleImport() {
    if (importPreview.length === 0) return;
    importing = true;
    try {
      // Resolve table names to IDs
      const { rows: resolved, unresolved } = resolveImportTables(importPreview, tables);
      if (unresolved > 0) {
        addToast(`${unresolved} row(s) kept unseated: table name missing or not found`, 'error');
      }
      const result = await bulkImportGuests(resolved.map(g => ({
        id: g.id,
        name: g.name,
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
      // Do not wait for the SSE echo: reseed so tables and seating update even
      // when the stream is down. Queued edits must survive, as in the layout.
      try {
        // Mirror the layout: drain queued edits before reseeding. A drain that
        // leaves retryable mutations queued must not lose their optimistic
        // values, so keep the current list and tell the user.
        if (hasQueuedEdits(wid)) {
          await syncQueue(wid);
          if (hasQueuedEdits(wid)) {
            addToast('Imported guests; the list refreshes after queued edits sync', 'error');
            return;
          }
        }
        await refreshGuests(async () => (await fetchAllGuests(wid)).map(toGuest), wid);
      } catch {
        addToast('Imported guests; the list refreshes after queued edits sync', 'error');
      }
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

  // Only the topmost dialog handles Escape: the move drawer and menu register
  // their own, these two are inline modals.
  $effect(() => {
    if (!showBulkMoveModal) return;
    return registerEscape(() => showBulkMoveModal = false);
  });

  $effect(() => {
    if (!showImportModal) return;
    return registerEscape(() => showImportModal = false);
  });

  function requestBulkDelete() {
    if (selectedIds.size === 0) return;
    showBulkDeleteConfirm = true;
  }

  async function confirmBulkDelete() {
    showBulkDeleteConfirm = false;
    await bulkDelete();
  }

  async function bulkDelete() {
    if (selectedIds.size === 0) return;
    const ids = [...selectedIds];
    // allSettled: one refused guest must not hide the ones that did delete.
    const results = await Promise.allSettled(ids.map(id => apiDeleteGuest(wid, id)));
    const failedIds = ids.filter((_, i) => results[i].status === 'rejected');
    const deleted = ids.length - failedIds.length;
    if (deleted) addToast(`Deleted ${deleted} guest${deleted === 1 ? '' : 's'}`, 'info');
    if (failedIds.length) {
      const reason = (results.find(r => r.status === 'rejected') as PromiseRejectedResult).reason;
      addToast(`${failedIds.length} of ${ids.length} guests could not be deleted${reason?.message ? `: ${reason.message}` : ''}`, 'error');
    }
    selectedIds = new Set(failedIds);
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
    const occ = allGuests.filter(g => g.tableId === bulkMoveTableId && !selectedIds.has(g.id) && g.seatNumber != null)
      .flatMap(g => Array.from({ length: g.pax }, (_, i) => g.seatNumber! + i));
    for (let s = 1; s <= cap; s++) {
      if (!occ.includes(s)) return s;
    }
    return cap + 1;
  }

  let bulkOccupiedSeats = $derived.by((): Set<number> => {
    if (!bulkMoveTableId) return new Set();
    return new Set(
      allGuests
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
    // Selection survives filtering, so pax must come from the full list: reading
    // it from the filtered rows undercounts seats and moves half the batch.
    const moving = ids.map(id => get(guestMap).get(id)).filter((g): g is Guest => !!g);
    if (moving.length !== ids.length) {
      addToast('Some selected guests are no longer loaded; refresh and try again', 'error');
      return;
    }
    const seatsNeeded = moving.reduce((sum, g) => sum + g.pax, 0);
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
      for (const g of moving) {
        await assignSeat(wid, g.id, bulkMoveTableId, seat);
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
<!-- svelte-ignore a11y_no_static_element_interactions -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="p-4 sm:p-7 max-w-[1400px]" onclick={() => contextMenu = null}>
  <h1 class="sr-only">Guests</h1>
  <!-- Toolbar -->
  <div class="flex items-center justify-between gap-2 sm:gap-4 mb-5 flex-wrap">
    <div class="relative flex-1 min-w-[160px] sm:min-w-[200px] max-w-md">
      <Search class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 sm:w-[18px] sm:h-[18px] text-gray-500 pointer-events-none " />
      <input
        type="text" placeholder="Search guests..." bind:value={searchQuery}
        aria-label="Search guests by name"
        class="w-full pl-10 sm:pl-11 pr-3 sm:pr-4 py-2.5 sm:py-3 border border-gray-200 rounded-xl text-sm bg-white focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all min-h-[44px]"
      />
    </div>
    <div class="flex items-center gap-1 sm:gap-2 flex-wrap">
      <select bind:value={rsvpFilter} aria-label="Filter by RSVP status" class="px-2 sm:px-3 py-2 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold outline-none">
        <option value="all">All Status</option>
        <option value="confirmed">Confirmed</option>
        <option value="pending">Pending</option>
        <option value="declined">Declined</option>
        <option value="no_response">No Response</option>
      </select>
      <button onclick={() => { importReadId++; showImportModal = true; importPreview = []; importFile = null; importError = ''; }}
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
      <button onclick={requestBulkDelete} class="px-3 py-1.5 bg-white border border-red-200 rounded-lg text-xs font-medium text-red hover:bg-red-50">Delete</button>
    </div>
  {/if}

  {#if allGuests.length === 0}
    <div class="flex flex-col items-center justify-center py-20 text-center">
      <div class="w-16 h-16 bg-gray-100 rounded-full flex items-center justify-center mb-4">
        <Users class="w-8 h-8 text-gray-500" />
      </div>
      <p class="text-gray-500 font-medium">No guests yet</p>
      <p class="text-sm text-gray-500 mt-1 mb-4">Add your first guest to get started.</p>
      <button onclick={() => { $drawerCreateMode = true; $isDrawerOpen = true; }} class="px-4 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center gap-2">
        <Plus class="w-4 h-4" /> Add Guest
      </button>
    </div>
  {:else if filtered.length === 0}
    <div class="flex flex-col items-center justify-center py-20 text-center">
      <div class="w-16 h-16 bg-gray-100 rounded-full flex items-center justify-center mb-4">
        <Search class="w-8 h-8 text-gray-500" />
      </div>
      <p class="text-gray-500 font-medium">No guests match</p>
      <p class="text-sm text-gray-500 mt-1 mb-4">Try another name, or clear the status filter.</p>
      <button onclick={clearFilters} class="px-4 py-2.5 bg-white border border-gray-200 text-gray-700 rounded-xl text-sm font-semibold hover:bg-gray-50 transition-colors">
        Clear search and filters
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
                <input type="checkbox" aria-label="Select all guests" checked={selectedIds.size === filtered.length && filtered.length > 0} onchange={toggleSelectAll} class="rounded w-6 h-6" />
              </th>
              {#each columns as col}
                <th class="px-4 py-3 text-left">
                  <button
                    type="button"
                    class="inline-flex items-center gap-1 text-[12px] font-semibold uppercase tracking-wider text-gray-500 cursor-pointer select-none hover:text-gray-700"
                    onclick={() => toggleSort(col.key)}
                  >
                    {col.label} <ArrowUpDown class="w-3 h-3" />
                  </button>
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
                  <input type="checkbox" aria-label="Select {guest.name}" checked={selectedIds.has(guest.id)} onchange={() => toggleSelect(guest.id)} class="rounded w-6 h-6" />
                </td>
                <td class="px-4 py-3.5">
                  <div class="flex items-center gap-3">
                    <div class="w-8 h-8 rounded-full bg-red-50 flex items-center justify-center text-red text-xs font-bold flex-shrink-0">{getInitials(guest.name)}</div>
                    <button type="button" class="font-semibold text-gray-900 text-left hover:underline" onclick={(e) => { e.stopPropagation(); openGuest(guest); }}>{guest.name}</button>
                  </div>
                </td>
                <td class="px-4 py-3.5"><Badge status={guest.rsvp as RSVPStatus} /></td>
                <td class="px-4 py-3.5 text-gray-700 font-medium">{guest.pax}</td>
                <td class="px-4 py-3.5 text-gray-700 font-medium">{tables.find(t => t.id === guest.tableId)?.name || (guest.tableId ?? '—')}</td>
                {#if $showSeatNumbers}
                  <td class="px-4 py-3.5 text-gray-700 font-medium">{guest.seatNumber ?? '—'}</td>
                {/if}
                <td class="px-4 py-3.5">
                  {#if guest.checkedInAt}
                    <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-emerald-50 text-emerald-700 rounded-full text-xs font-semibold border border-emerald-200">
                      <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span> Yes
                    </span>
                  {:else}
                    <span class="text-gray-500">No</span>
                  {/if}
                </td>
                <td class="px-4 py-3.5 text-gray-500 max-w-[140px] truncate">{guest.notes || '—'}</td>
                <td class="px-4 py-3.5" onclick={(e) => e.stopPropagation()}>
                  <button class="p-1.5 rounded-lg hover:bg-gray-100" aria-label="Actions"
                    onclick={(e) => { e.stopPropagation(); handleCtx(e, guest); }}>
                    <MoreHorizontal class="w-4 h-4 text-gray-500" />
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      {#if displayGuests.length < filtered.length}
        <div use:loadMoreOnView={visible} class="px-5 py-4 border-t border-gray-100 text-sm text-gray-500" aria-live="polite">
          Showing {displayGuests.length} of {filtered.length} guests
        </div>
      {:else}
        <div class="px-5 py-4 border-t border-gray-100 text-sm text-gray-500">{filtered.length} guests</div>
      {/if}
  </div>
{/if}

<!-- Bulk Move Table Modal -->
{#if showBulkMoveModal}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="fixed inset-0 z-[700] flex items-center justify-center bg-black/30 backdrop-blur-md" onclick={() => showBulkMoveModal = false}>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div use:trapFocus role="dialog" aria-modal="true" aria-label="Move {selectedIds.size} guests" tabindex="-1" class="bg-white/95 backdrop-blur-xl rounded-2xl shadow-2xl w-full max-w-md p-6" onclick={(e) => e.stopPropagation()}>
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
          <div class="text-xs text-gray-500">
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
<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="fixed inset-0 z-[599]" onclick={() => contextMenu = null} role="presentation"></div>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div bind:this={contextMenuEl} role="menu" aria-label="Guest actions" tabindex="-1"
    class="fixed z-[600] bg-white/95 backdrop-blur-xl border border-black/[0.06] rounded-xl shadow-xl py-1.5 min-w-[180px]"
    style={getMenuStyle(contextMenu.x, contextMenu.y)} onclick={(e) => e.stopPropagation()}
    onkeydown={menuKeydown}>
    <button type="button" role="menuitem" class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={() => { $drawerStartEditing = true; openGuest(contextMenu!.guest); contextMenu = null; }}>
      <Pencil class="w-4 h-4" /> Edit
    </button>
    <button type="button" role="menuitem" class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={() => openMoveTable(contextMenu!.guest)}>
      <ArrowUpDown class="w-4 h-4" /> Move Table
    </button>
    {#if contextMenu!.guest.tableId}
      <button type="button" role="menuitem" class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={() => unassignGuest(contextMenu!.guest)}>
        <ArrowUpDown class="w-4 h-4" /> Unassign Table
      </button>
    {/if}
    <hr class="my-1 border-gray-100" />
    <button type="button" role="menuitem" class="w-full flex items-center gap-2.5 px-4 py-2 text-sm text-red hover:bg-red-50" onclick={() => requestDelete(contextMenu!.guest)}>
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
    guests={allGuests}
    currentTableName={tables.find(t => t.id === g.tableId)?.name ?? '—'}
    saving={moveSaving}
    onSave={confirmMoveTable}
    onClose={() => { showMoveModal = false; moveGuest = null; }}
  />
{/if}

{#if showImportModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="absolute inset-0 bg-black/30 backdrop-blur-md" onclick={() => showImportModal = false} role="presentation"></div>
    <div use:trapFocus role="dialog" aria-modal="true" aria-label="Import guests from CSV" tabindex="-1" class="relative bg-white/95 backdrop-blur-xl rounded-2xl shadow-2xl w-full max-w-lg overflow-hidden">
      <div class="flex items-center justify-between p-5 border-b border-gray-100">
        <h3 class="font-bold text-gray-900">Import Guests from CSV</h3>
        <button onclick={() => showImportModal = false} aria-label="Close" class="w-11 h-11 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
          <X class="w-4 h-4 text-gray-500" />
        </button>
      </div>
      <div class="p-5 space-y-4">
        <p class="text-sm text-gray-600">Upload a CSV file with columns: <strong>name</strong> (required), email, pax, rsvp, vip, notes, dietary, table, seat</p>

        <div class="border-2 border-dashed border-gray-200 rounded-xl p-6 text-center hover:border-red/50 transition-colors">
          <input type="file" accept=".csv" onchange={handleImportFile} class="hidden" id="csv-input" />
          <label for="csv-input" class="cursor-pointer">
            <FileText class="w-8 h-8 text-gray-500 mx-auto mb-2" />
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
              <p class="text-xs text-gray-500 text-center py-2">...and {importPreview.length - 20} more</p>
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

<ConfirmDialog
  open={showDeleteConfirm}
  title="Delete Guest"
  message={`Delete ${deleteTarget?.name ?? 'this guest'}? This cannot be undone.`}
  confirmLabel="Delete"
  cancelLabel="Cancel"
  variant="danger"
  onConfirm={confirmDelete}
  onCancel={() => { showDeleteConfirm = false; deleteTarget = null; }}
/>

<ConfirmDialog
  open={showBulkDeleteConfirm}
  title="Delete Guests"
  message={`Delete ${selectedIds.size} selected guest${selectedIds.size === 1 ? '' : 's'}? This cannot be undone.`}
  confirmLabel="Delete"
  cancelLabel="Cancel"
  variant="danger"
  onConfirm={confirmBulkDelete}
  onCancel={() => { showBulkDeleteConfirm = false; }}
/>
