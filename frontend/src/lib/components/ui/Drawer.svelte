<script lang="ts">
  import type { Guest, BanquetTable, RSVPStatus } from '$lib/types';
  import { X, Phone, Mail, Utensils, StickyNote, Banknote, Gift, Pencil, Check, UserCheck, CheckCircle2 } from 'lucide-svelte';
  import Badge from './Badge.svelte';
  import { getInitials, cn } from '$lib/utils';
  import { formatSeatRange } from '$lib/utils/seat';
  import { addToast } from '$lib/stores';
  import { weddingId } from '$lib/stores/weddingId';
  import { updateGuest, checkInGuest, checkOutGuest } from '$lib/api/guests';
  import { get } from 'svelte/store';
  let { guest, tables = [], onClose, startEditing = false, onCheckIn, onCheckOut }: { guest: Guest; tables?: BanquetTable[]; onClose: () => void; startEditing?: boolean; onCheckIn?: (g: Guest) => void; onCheckOut?: (g: Guest) => void } = $props();

  let tableName = $derived(tables.find(t => t.id === guest.tableId)?.name ?? guest.tableId ?? '—');

  let editing = $state(false);
  let saving = $state(false);
  let showCheckinModal = $state(false);
  let angbaoAmount = $state('');
  let giftItem = $state('');

  // Touch drag state for mobile dismiss
  let dragY = $state(0);
  let dragging = $state(false);
  let startY = $state(0);

  function onTouchStart(e: TouchEvent) {
    if (window.innerWidth >= 640) return; // desktop only
    startY = e.touches[0].clientY;
    dragging = true;
  }

  function onTouchMove(e: TouchEvent) {
    if (!dragging) return;
    const delta = e.touches[0].clientY - startY;
    if (delta > 0) dragY = delta;
  }

  function onTouchEnd() {
    if (!dragging) return;
    dragging = false;
    if (dragY > 100) {
      onClose();
    }
    dragY = 0;
  }

  $effect(() => { editing = startEditing; });
  let form = $state({
    name: guest.name,
    phone: guest.phone,
    email: guest.email ?? '',
    pax: guest.pax,
    rsvp: guest.rsvp,
    isVip: guest.isVip,
    notes: guest.notes,
    dietary: guest.dietaryRequirements,
    angbaoAmt: guest.angbaoAmount != null ? String(guest.angbaoAmount) : '',
    giftItem: guest.giftItem ?? '',
  });

  $effect(() => {
    form.name = guest.name;
    form.phone = guest.phone;
    form.email = guest.email ?? '';
    form.pax = guest.pax;
    form.rsvp = guest.rsvp;
    form.isVip = guest.isVip;
    form.notes = guest.notes;
    form.dietary = guest.dietaryRequirements;
    form.angbaoAmt = guest.angbaoAmount != null ? String(guest.angbaoAmount) : '';
    form.giftItem = guest.giftItem ?? '';
  });

  async function save() {
    saving = true;
    try {
      const wid = get(weddingId);
      const updated = await updateGuest(wid, guest.id, {
        name: form.name,
        phone: form.phone,
        email: form.email || undefined,
        pax: form.pax,
        rsvp: form.rsvp,
        isVip: form.isVip,
        notes: form.notes,
        dietary: form.dietary,
        angbaoAmt: form.angbaoAmt ? Number(form.angbaoAmt) : null,
        giftItem: form.giftItem || null,
      });
      Object.assign(guest, {
        name: updated.name,
        phone: updated.phone,
        email: updated.email,
        pax: updated.pax,
        rsvp: updated.rsvp,
        isVip: updated.isVip,
        notes: updated.notes,
        dietaryRequirements: updated.dietary,
        angbaoAmount: updated.angbaoAmt ?? undefined,
        giftItem: updated.giftItem ?? undefined,
      });
      Object.assign(form, {
        angbaoAmt: updated.angbaoAmt != null ? String(updated.angbaoAmt) : '',
        giftItem: updated.giftItem ?? '',
      });
      editing = false;
      addToast('Guest updated', 'success');
    } catch (e: any) {
      addToast(e.message ?? 'Update failed', 'error');
    } finally {
      saving = false;
    }
  }

  function cancel() {
    editing = false;
    onClose();
  }

  function openCheckinModal() {
    angbaoAmount = guest.angbaoAmount != null ? String(guest.angbaoAmount) : '';
    giftItem = guest.giftItem ?? '';
    showCheckinModal = true;
  }

  async function confirmCheckIn() {
    const wid = get(weddingId);
    try {
      await checkInGuest(wid, guest.id);
      guest.checkedIn = true;
      guest.checkedInAt = new Date();
      if (angbaoAmount) guest.angbaoAmount = Number(angbaoAmount);
      if (giftItem) guest.giftItem = giftItem;
      onCheckIn?.(guest);
      showCheckinModal = false;
      addToast(`${guest.name} checked in`, 'success');
    } catch (e: any) {
      addToast(e.message ?? 'Check-in failed', 'error');
    }
  }

  async function handleCheckOut() {
    const wid = get(weddingId);
    try {
      await checkOutGuest(wid, guest.id);
      guest.checkedIn = false;
      guest.checkedInAt = undefined;
      onCheckOut?.(guest);
      addToast(`${guest.name} checked out`, 'success');
    } catch (e: any) {
      addToast(e.message ?? 'Check-out failed', 'error');
    }
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="drawer-overlay" onclick={onClose}>
  <div class="drawer-backdrop"></div>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="drawer-panel" onclick={(e) => e.stopPropagation()}
    ontouchstart={onTouchStart}
    ontouchmove={onTouchMove}
    ontouchend={onTouchEnd}
    style="transform: translateY({dragY}px); transition: {dragging ? 'none' : 'transform 300ms cubic-bezier(0.2, 0.8, 0.2, 1)'}"
  >
    <!-- Pill dismiss (mobile only) -->
    <div class="drawer-pill sm:hidden" onclick={onClose} role="presentation">
      <div class="drawer-pill-bar"></div>
    </div>
    <div class="drawer-header">
      <h2 class="drawer-title">{editing ? 'Edit Guest' : 'Guest Details'}</h2>
      <div class="drawer-actions">
        {#if editing}
          <button onclick={cancel} class="drawer-btn-secondary">Cancel</button>
          <button onclick={save} disabled={saving}
            class="drawer-btn-primary">
            <Check class="w-4 h-4" /> {saving ? 'Saving...' : 'Save'}
          </button>
        {:else}
          <button onclick={() => editing = true} class="drawer-icon-btn" aria-label="Edit">
            <Pencil class="w-5 h-5" />
          </button>
          <button onclick={onClose} class="drawer-icon-btn hidden sm:flex" aria-label="Close">
            <X class="w-5 h-5" />
          </button>
        {/if}
      </div>
    </div>

    <div class="drawer-body">
      {#if editing}
        <div class="form-grid">
          <div class="form-field">
            <label class="form-label">Name</label>
            <input bind:value={form.name} class="form-input" />
          </div>
          <div class="form-field">
            <label class="form-label">Phone</label>
            <input bind:value={form.phone} class="form-input" />
          </div>
          <div class="form-field">
            <label class="form-label">Email</label>
            <input bind:value={form.email} class="form-input" />
          </div>
          <div class="form-row-2">
            <div class="form-field">
              <label class="form-label">Pax</label>
              <input type="number" min="1" bind:value={form.pax} class="form-input" />
            </div>
            <div class="form-field">
              <label class="form-label">RSVP</label>
              <select bind:value={form.rsvp} class="form-input">
                <option value="confirmed">Confirmed</option>
                <option value="pending">Pending</option>
                <option value="declined">Declined</option>
                <option value="no_response">No Response</option>
              </select>
            </div>
          </div>
          <div class="form-field">
            <label class="form-check-label">
              <input type="checkbox" bind:checked={form.isVip} class="rounded" /> VIP Guest
            </label>
          </div>
          <div class="form-field">
            <label class="form-label">Notes</label>
            <textarea bind:value={form.notes} rows="2" class="form-input form-textarea"></textarea>
          </div>
          <div class="form-field">
            <label class="form-label">Dietary</label>
            <div class="dietary-chips">
              {#each ['Halal', 'Vegetarian', 'Vegan', 'Gluten-Free', 'Nut-Free', 'No Seafood'] as opt}
                <button type="button"
                  onclick={() => { form.dietary = form.dietary.includes(opt) ? form.dietary.filter(d => d !== opt) : [...form.dietary, opt]; }}
                  class={cn(
                    "dietary-chip",
                    form.dietary.includes(opt) ? "dietary-chip-active" : ""
                  )}
                >{opt}</button>
              {/each}
            </div>
          </div>
          <div class="form-field">
            <label class="form-label">Angbao Amount (RM)</label>
            <input type="number" min="0" bind:value={form.angbaoAmt} placeholder="0" class="form-input" />
          </div>
          <div class="form-field">
            <label class="form-label">Gift Item</label>
            <input bind:value={form.giftItem} placeholder="e.g. Kitchenware set" class="form-input" />
          </div>
        </div>
      {:else}
        <!-- View Mode -->
        <div class="guest-hero">
          <div class={cn(
            "guest-avatar-lg",
            guest.checkedIn ? "avatar-checked" :
            guest.isVip ? "avatar-vip" : "avatar-default"
          )}>
            {getInitials(guest.name)}
          </div>
          <div>
            <h3 class="guest-name-lg">{#if guest.isVip}<span class="text-gold">★</span>{/if} {guest.name}</h3>
            <Badge status={guest.rsvp} />
          </div>
        </div>

        <div class="info-rows">
          <div class="info-row">
            <Phone class="info-icon" />
            <span>{guest.phone}</span>
          </div>
          {#if guest.email}
            <div class="info-row">
              <Mail class="info-icon" />
              <span>{guest.email}</span>
            </div>
          {/if}
        </div>

        <div class="detail-grid">
          <div class="detail-card">
            <div class="detail-label">Table</div>
            <div class="detail-value">{tableName}</div>
          </div>
          <div class="detail-card">
            <div class="detail-label">Seat{guest.pax > 1 ? 's' : ''}</div>
            <div class="detail-value">{formatSeatRange(guest.seatNumber, guest.pax)}</div>
          </div>
          <div class="detail-card">
            <div class="detail-label">Party Size</div>
            <div class="detail-value">{guest.pax}</div>
          </div>
          <div class="detail-card">
            <div class="detail-label">Checked In</div>
            <div class="detail-value {guest.checkedIn ? 'text-emerald-600' : 'text-gray-400'}">
              {guest.checkedIn ? '✓' : '—'}
            </div>
          </div>
        </div>

        {#if guest.isVip}
          <div class="vip-banner">⭐ VIP Guest</div>
        {/if}

        {#if guest.dietaryRequirements.length > 0}
          <div class="section">
            <div class="section-header">
              <Utensils class="section-icon" />
              Dietary Requirements
            </div>
            <div class="chip-group">
              {#each guest.dietaryRequirements as req}
                <span class="dietary-tag">{req}</span>
              {/each}
            </div>
          </div>
        {/if}

        {#if guest.notes}
          <div class="section">
            <div class="section-header">
              <StickyNote class="section-icon" />
              Notes
            </div>
            <p class="notes-text">{guest.notes}</p>
          </div>
        {/if}

        {#if guest.angbaoAmount || guest.giftItem}
          <div class="section">
            <div class="section-header">Gift Details</div>
            <div class="gift-card">
              {#if guest.angbaoAmount}
                <div class="gift-row">
                  <Banknote class="gift-icon text-emerald-600" />
                  <span class="gift-label">Angbao:</span>
                  <span class="gift-value text-emerald-700">RM {guest.angbaoAmount}</span>
                </div>
              {/if}
              {#if guest.giftItem}
                <div class="gift-row">
                  <Gift class="gift-icon text-gold" />
                  <span class="gift-label">Gift:</span>
                  <span class="gift-value text-gold-dark">{guest.giftItem}</span>
                </div>
              {/if}
            </div>
          </div>
        {/if}

        <!-- Check-in Action -->
        <div class="pt-2 pb-4">
          {#if guest.checkedIn}
            <button onclick={handleCheckOut} class="w-full py-3 bg-emerald-50 text-emerald-700 rounded-xl text-sm font-semibold border border-emerald-200 hover:bg-emerald-100 transition-colors flex items-center justify-center gap-2">
              <CheckCircle2 class="w-4 h-4" /> Checked In — Tap to Check Out
            </button>
          {:else}
            <button onclick={openCheckinModal} class="w-full py-3 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2">
              <UserCheck class="w-4 h-4" /> Check In
            </button>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>

<!-- Check-in Modal -->
{#if showCheckinModal}
  <div class="fixed inset-0 z-[60] flex items-end sm:items-center justify-center p-0 sm:p-4">
    <div class="absolute inset-0 bg-black/30 backdrop-blur-md" onclick={() => showCheckinModal = false} role="presentation"></div>
    <div class="relative bg-white/95 backdrop-blur-xl rounded-t-2xl sm:rounded-2xl shadow-2xl w-full sm:max-w-md overflow-hidden">
      <!-- Pill dismiss (mobile only) -->
      <div class="flex justify-center pt-3 sm:hidden cursor-pointer" onclick={() => showCheckinModal = false} role="presentation">
        <div class="w-10 h-1 bg-gray-300 rounded-full"></div>
      </div>
      <div class="flex items-center justify-between p-5 border-b border-gray-100">
        <h3 class="font-bold text-gray-900">Check In {guest.name}</h3>
        <button onclick={() => showCheckinModal = false} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors hidden sm:flex">
          <X class="w-4 h-4 text-gray-400" />
        </button>
      </div>
      <div class="p-5 space-y-4">
        <div>
          <label class="text-sm font-semibold text-gray-700 mb-1.5 block">Angbao Amount (RM)</label>
          <div class="relative">
            <Banknote class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
            <input type="number" min="0" bind:value={angbaoAmount} placeholder="0"
              class="w-full pl-10 pr-4 py-3 border border-black/[0.08] rounded-xl text-sm bg-white/80 focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all min-h-[48px]" />
          </div>
        </div>
        <div>
          <label class="text-sm font-semibold text-gray-700 mb-1.5 block">Gift Item</label>
          <div class="relative">
            <Gift class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
            <input bind:value={giftItem} placeholder="Optional"
              class="w-full pl-10 pr-4 py-3 border border-black/[0.08] rounded-xl text-sm bg-white/80 focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all min-h-[48px]" />
          </div>
        </div>
      </div>
      <div class="flex gap-3 p-5 pt-0">
        <button onclick={confirmCheckIn}
          class="flex-1 py-3 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2">
          <UserCheck class="w-4 h-4" /> Confirm Check In
        </button>
        <button onclick={() => showCheckinModal = false}
          class="px-6 py-3 border border-black/[0.06] bg-white/90 rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
          Cancel
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .drawer-overlay {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: flex;
    align-items: flex-end;
    justify-content: flex-end;
  }

  @media (min-width: 640px) {
    .drawer-overlay {
      align-items: stretch;
    }
  }

  .drawer-backdrop {
    position: absolute;
    inset: 0;
    background: rgba(0, 0, 0, 0.3);
    backdrop-filter: blur(4px);
    -webkit-backdrop-filter: blur(4px);
    animation: fadeIn 200ms ease;
  }

  .drawer-panel {
    position: relative;
    width: 100%;
    max-height: 85vh;
    border-radius: 1.25rem 1.25rem 0 0;
    background: rgba(255, 255, 255, 0.95);
    backdrop-filter: blur(24px) saturate(200%);
    -webkit-backdrop-filter: blur(24px) saturate(200%);
    box-shadow: 0 -8px 48px rgba(0, 0, 0, 0.15), 0 -2px 8px rgba(0, 0, 0, 0.08);
    overflow-y: auto;
    animation: slideUp 300ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }

  @media (min-width: 640px) {
    .drawer-panel {
      width: 400px;
      max-height: 100%;
      border-radius: 0;
      animation: slideInRight 300ms cubic-bezier(0.2, 0.8, 0.2, 1);
      box-shadow: -12px 0 48px rgba(0, 0, 0, 0.15), -2px 0 8px rgba(0, 0, 0, 0.08);
    }
  }

  .drawer-pill {
    display: flex;
    justify-content: center;
    padding: 0.5rem 0 0.75rem;
    cursor: pointer;
  }

  .drawer-pill-bar {
    width: 2.5rem;
    height: 0.25rem;
    background: rgba(0, 0, 0, 0.15);
    border-radius: 9999px;
    transition: background 150ms ease;
  }

  .drawer-pill:active .drawer-pill-bar {
    background: rgba(0, 0, 0, 0.25);
  }

  .drawer-header {
    position: sticky;
    top: 0;
    z-index: 10;
    background: rgba(255, 255, 255, 0.95);
    backdrop-filter: blur(24px) saturate(200%);
    -webkit-backdrop-filter: blur(24px) saturate(200%);
    padding: 1.25rem 1.5rem;
    border-bottom: 1px solid rgba(0, 0, 0, 0.05);
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .drawer-title {
    font-size: 1.125rem;
    font-weight: 700;
    color: #111827;
    letter-spacing: -0.01em;
  }

  .drawer-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .drawer-icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2.25rem;
    height: 2.25rem;
    border-radius: 0.5rem;
    color: #6b7280;
    transition: background 100ms ease, transform 100ms ease;
    min-width: 44px;
    min-height: 44px;
  }

  .drawer-icon-btn:active {
    transform: scale(0.9);
    background: rgba(0, 0, 0, 0.06);
  }

  .drawer-icon-btn:hover {
    background: rgba(0, 0, 0, 0.04);
  }

  .drawer-btn-secondary {
    padding: 0.5rem 1rem;
    border-radius: 0.5rem;
    font-size: 0.875rem;
    font-weight: 500;
    color: #6b7280;
    transition: background 100ms ease, transform 100ms ease;
  }

  .drawer-btn-secondary:active {
    transform: scale(0.97);
  }

  .drawer-btn-primary {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.5rem 0.875rem;
    background: #A11217;
    color: white;
    border-radius: 0.5rem;
    font-size: 0.875rem;
    font-weight: 600;
    transition: background 100ms ease, transform 100ms ease;
  }

  .drawer-btn-primary:active {
    transform: scale(0.97);
  }

  .drawer-btn-primary:disabled {
    opacity: 0.5;
  }

  .drawer-body {
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }

  /* View mode */
  .guest-hero {
    display: flex;
    align-items: center;
    gap: 1rem;
  }

  .guest-avatar-lg {
    width: 3.5rem;
    height: 3.5rem;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: 1rem;
    flex-shrink: 0;
  }

  .avatar-default { background: #FDEAEA; color: #A11217; border: 2px solid #FAC5C5; }
  .avatar-vip { background: #FDF8E8; color: #B8941F; border: 2px solid #E8CC6E; }
  .avatar-checked { background: #ECFDF5; color: #059669; border: 2px solid #A7F3D0; }

  .guest-name-lg {
    font-size: 1.25rem;
    font-weight: 700;
    color: #111827;
    letter-spacing: -0.01em;
  }

  .info-rows {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .info-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    font-size: 0.875rem;
    color: #4b5563;
  }

  .info-icon {
    width: 1rem;
    height: 1rem;
    color: #9ca3af;
    flex-shrink: 0;
  }

  .detail-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
  }

  .detail-card {
    background: rgba(249, 250, 251, 0.8);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid rgba(0, 0, 0, 0.04);
    border-radius: 0.75rem;
    padding: 1rem;
    text-align: center;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  }

  .detail-label {
    font-size: 0.75rem;
    color: #6b7280;
    font-weight: 500;
    margin-bottom: 0.25rem;
  }

  .detail-value {
    font-size: 1.5rem;
    font-weight: 700;
    color: #111827;
    letter-spacing: -0.02em;
  }

  .vip-banner {
    background: #FDF8E8;
    border: 1px solid rgba(212, 175, 55, 0.3);
    border-radius: 0.75rem;
    padding: 1rem;
    text-align: center;
    font-weight: 700;
    color: #B8941F;
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .section-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem;
    font-weight: 600;
    color: #374151;
  }

  .section-icon {
    width: 1rem;
    height: 1rem;
    color: #9ca3af;
  }

  .chip-group {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .dietary-tag {
    padding: 0.375rem 0.75rem;
    background: #FFFBEB;
    color: #D97706;
    border-radius: 9999px;
    font-size: 0.75rem;
    font-weight: 500;
    border: 1px solid #FDE68A;
  }

  .notes-text {
    font-size: 0.875rem;
    color: #4b5563;
    background: rgba(249, 250, 251, 0.8);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid rgba(0, 0, 0, 0.04);
    border-radius: 0.75rem;
    padding: 1rem;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  }

  .gift-card {
    background: #FDF8E8;
    border: 1px solid rgba(212, 175, 55, 0.2);
    border-radius: 0.75rem;
    padding: 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .gift-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem;
  }

  .gift-icon { width: 1rem; height: 1rem; flex-shrink: 0; }
  .gift-label { color: #6b7280; }
  .gift-value { font-weight: 700; }

  /* Edit mode */
  .form-grid {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .form-field {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
  }

  .form-label {
    font-size: 0.75rem;
    font-weight: 600;
    color: #6b7280;
  }

  .form-check-label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem;
    color: #374151;
  }

  .form-input {
    width: 100%;
    padding: 0.75rem 1rem;
    border: 1.5px solid rgba(0, 0, 0, 0.08);
    border-radius: 0.75rem;
    font-size: 0.9375rem;
    color: #111827;
    background: rgba(255, 255, 255, 0.8);
    outline: none;
    transition: border-color 200ms ease, box-shadow 200ms ease, transform 100ms ease;
    min-height: 48px;
  }

  .form-input:focus {
    border-color: #A11217;
    box-shadow: 0 0 0 3px rgba(161, 18, 23, 0.1);
  }

  .form-input:active {
    transform: scale(0.99);
  }

  .form-textarea {
    resize: none;
    min-height: 3.5rem;
  }

  .form-row-2 {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
  }

  .dietary-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .dietary-chip {
    padding: 0.375rem 0.75rem;
    border-radius: 0.5rem;
    font-size: 0.75rem;
    font-weight: 500;
    border: 1.5px solid rgba(0, 0, 0, 0.08);
    color: #4b5563;
    background: white;
    transition: all 150ms ease, transform 100ms ease;
  }

  .dietary-chip:active {
    transform: scale(0.95);
  }

  .dietary-chip-active {
    background: #FDF8E8;
    border-color: #D4AF37;
    color: #B8941F;
  }

  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  @keyframes slideUp {
    from { transform: translateY(100%); }
    to { transform: translateY(0); }
  }

  @keyframes slideInRight {
    from { transform: translateX(100%); }
    to { transform: translateX(0); }
  }

  :global(.animate-slideUp) {
    animation: slideUp 300ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }
</style>
