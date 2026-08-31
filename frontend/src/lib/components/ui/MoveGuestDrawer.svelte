<script lang="ts">
  import { X, ArrowUpDown } from 'lucide-svelte';
  import { getInitials } from '$lib/utils';
  import type { Guest, BanquetTable } from '$lib/types';

  let {
    guest,
    tables = [],
    guests = [],
    currentTableName = '—',
    onSave,
    onClose,
  }: {
    guest: Guest;
    tables?: BanquetTable[];
    guests?: Guest[];
    currentTableName?: string;
    onSave: (tableId: string, seatNum: number) => void;
    onClose: () => void;
  } = $props();

  let tableId = $state(guest.tableId != null ? String(guest.tableId) : (tables.length ? String(tables[0].id) : ''));
  let seatNum = $state(guest.seatNumber ?? 1);

  const capacity = $derived(tables.find(t => String(t.id) === tableId)?.capacity ?? 10);

  // Occupancy of the SELECTED table (the guest being moved isn't seated there yet)
  const occupiedSeats = $derived.by((): Set<number> => {
    if (!tableId) return new Set();
    return new Set(
      guests
        .filter(g => String(g.tableId) === tableId && g.id !== guest.id && g.seatNumber != null)
        .flatMap(g => Array.from({ length: g.pax }, (_, i) => g.seatNumber! + i))
    );
  });

  const available = $derived.by(() => {
    const free: number[] = [];
    for (let s = 1; s <= capacity; s++) {
      if (!occupiedSeats.has(s)) free.push(s);
    }
    return free;
  });
  const freeCount = $derived(available.length);

  // Auto-select the lowest empty seat that fits the party size
  $effect(() => {
    if (!tableId) return;
    let best: number | null = null;
    for (const s of available) {
      let ok = true;
      for (let k = 1; k < guest.pax; k++) {
        if (occupiedSeats.has(s + k) || s + k > capacity) { ok = false; break; }
      }
      if (ok) { best = s; break; }
    }
    seatNum = best ?? (capacity > 0 ? capacity + 1 : 1);
  });

  const rangeOk = $derived.by(() => {
    if (seatNum < 1) return false;
    if (seatNum + guest.pax - 1 > capacity) return false;
    for (let k = 0; k < guest.pax; k++) {
      if (occupiedSeats.has(seatNum + k)) return false;
    }
    return true;
  });

  function save() {
    if (!tableId || !rangeOk) return;
    onSave(tableId, seatNum);
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<svelte:window onkeydown={(e) => { if (e.key === 'Escape') onClose(); }} />
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="drawer-overlay" onclick={onClose} role="presentation">
  <div class="drawer-backdrop" aria-hidden="true"></div>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="drawer-panel" role="dialog" aria-modal="true" aria-labelledby="drawer-title" onclick={(e) => e.stopPropagation()}>
    <div class="drawer-pill flex sm:hidden" onclick={onClose} role="presentation">
      <div class="drawer-pill-bar"></div>
    </div>
    <div class="drawer-header">
      <h2 id="drawer-title" class="drawer-title">Move Table</h2>
      <div class="drawer-actions">
        <button onclick={onClose} class="drawer-icon-btn flex" aria-label="Close">
          <X class="w-5 h-5" />
        </button>
      </div>
    </div>

    <div class="drawer-body">
      <div class="guest-hero">
        <div class="guest-avatar-lg">{getInitials(guest.name)}</div>
        <div class="min-w-0">
          <h3 class="guest-name-lg">{guest.name}</h3>
          <div class="text-xs text-gray-500 mt-0.5">
            <span>{guest.phone}</span>
            <span>•</span>
            <span>{guest.pax} pax</span>
          </div>
        </div>
      </div>

      <div class="detail-grid">
        <div class="detail-card">
          <div class="detail-label">Current Table</div>
          <div class="detail-value">{currentTableName}</div>
        </div>
        <div class="detail-card">
          <div class="detail-label">Current Seat</div>
          <div class="detail-value">{guest.seatNumber ?? '—'}</div>
        </div>
      </div>

      <div class="form-grid">
        <div class="form-field">
          <label for="move-table" class="form-label">New Table</label>
          <select id="move-table" bind:value={tableId} class="form-input">
            {#each tables as t}
              <option value={String(t.id)}>{t.name}</option>
            {/each}
          </select>
        </div>
        <div class="form-field">
          <label for="move-seat" class="form-label">Starting Seat (1–{capacity})</label>
          <input id="move-seat" type="number" min="1" max={capacity} bind:value={seatNum} class="form-input {!rangeOk ? 'input-error' : ''}" />
          {#if !rangeOk}
            <p class="text-xs text-red mt-1">No room for {guest.pax} pax starting at seat {seatNum}. Free seats: {freeCount}</p>
          {:else}
            <p class="text-xs text-gray-400 mt-1">{guest.pax > 1 ? `Party of ${guest.pax}: seats ${seatNum}–${seatNum + guest.pax - 1}` : `Seat ${seatNum}`} · {freeCount} free</p>
          {/if}
        </div>
      </div>
    </div>

    <div class="drawer-footer flex">
      <button onclick={save} disabled={!rangeOk}
        class="w-full py-3 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2 disabled:opacity-50">
        <ArrowUpDown class="w-4 h-4" /> Move to Table
      </button>
    </div>
  </div>
</div>

<style>
  @import './drawer.css';

  .guest-hero {
    display: flex;
    align-items: center;
    gap: 0.875rem;
  }

  .guest-avatar-lg {
    width: 3rem;
    height: 3rem;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: 0.9375rem;
    flex-shrink: 0;
    background: #FDEAEA;
    color: #A11217;
    border: 2px solid #FAC5C5;
  }

  .guest-name-lg {
    font-size: 1.125rem;
    font-weight: 700;
    color: #111827;
    letter-spacing: -0.01em;
  }

  .detail-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.5rem;
  }

  .detail-card {
    background: rgba(249, 250, 251, 0.8);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid rgba(0, 0, 0, 0.04);
    border-radius: 0.75rem;
    padding: 0.75rem;
    text-align: center;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  }

  .detail-label {
    font-size: 0.625rem;
    color: #6b7280;
    font-weight: 500;
    margin-bottom: 0.125rem;
  }

  .detail-value {
    font-size: 1.25rem;
    font-weight: 700;
    color: #111827;
    letter-spacing: -0.02em;
  }

  .input-error {
    border-color: #A11217;
  }
</style>
