<script lang="ts">
  import type { Guest, BanquetTable, RSVPStatus } from '$lib/types';
  import { X, Phone, Mail, Utensils, StickyNote, Banknote, Gift, Pencil, Check } from 'lucide-svelte';
  import Badge from './Badge.svelte';
  import { getInitials, cn } from '$lib/utils';
  import { formatSeatRange } from '$lib/utils/seat';
  import { addToast } from '$lib/stores';
  import { weddingId } from '$lib/stores/weddingId';
  import { updateGuest } from '$lib/api/guests';
  import { get } from 'svelte/store';
  let { guest, tables = [], onClose, startEditing = false }: { guest: Guest; tables?: BanquetTable[]; onClose: () => void; startEditing?: boolean } = $props();

  let tableName = $derived(tables.find(t => t.id === guest.tableId)?.name ?? guest.tableId ?? '—');

  let editing = $state(false);
  let saving = $state(false);

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
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="drawer-overlay" onclick={onClose}>
  <div class="drawer-backdrop"></div>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="drawer-panel" onclick={(e) => e.stopPropagation()}>
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
          <button onclick={onClose} class="drawer-icon-btn" aria-label="Close">
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
      {/if}
    </div>
  </div>
</div>

<style>
  .drawer-overlay {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: flex;
    justify-content: flex-end;
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
    width: 400px;
    max-width: 100%;
    background: rgba(255, 255, 255, 0.96);
    backdrop-filter: blur(20px) saturate(180%);
    -webkit-backdrop-filter: blur(20px) saturate(180%);
    box-shadow: -8px 0 32px rgba(0, 0, 0, 0.12);
    overflow-y: auto;
    animation: slideInRight 300ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }

  .drawer-header {
    position: sticky;
    top: 0;
    z-index: 10;
    background: rgba(255, 255, 255, 0.88);
    backdrop-filter: blur(20px) saturate(180%);
    -webkit-backdrop-filter: blur(20px) saturate(180%);
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
    background: #f9fafb;
    border-radius: 0.75rem;
    padding: 1rem;
    text-align: center;
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
    background: #f9fafb;
    border-radius: 0.75rem;
    padding: 1rem;
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
    padding: 0.625rem 0.875rem;
    border: 1.5px solid rgba(0, 0, 0, 0.08);
    border-radius: 0.625rem;
    font-size: 0.875rem;
    color: #111827;
    background: white;
    outline: none;
    transition: border-color 200ms ease, box-shadow 200ms ease;
  }

  .form-input:focus {
    border-color: #D4AF37;
    box-shadow: 0 0 0 3px rgba(212, 175, 55, 0.12);
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

  @keyframes slideInRight {
    from { transform: translateX(100%); }
    to { transform: translateX(0); }
  }
</style>
