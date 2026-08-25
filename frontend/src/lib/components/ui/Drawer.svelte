<script lang="ts">
  import type { Guest, BanquetTable, RSVPStatus } from '$lib/types';
  import { X, Phone, Mail, Utensils, StickyNote, Banknote, Gift, Pencil, Check, UserCheck, CheckCircle2, ArrowUpDown } from 'lucide-svelte';
  import Badge from './Badge.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import { getInitials, cn } from '$lib/utils';
  import { formatSeatRange } from '$lib/utils/seat';
  import { addToast } from '$lib/stores';
  import { track } from '$lib/analytics';
  import { weddingId } from '$lib/stores/weddingId';
  import { updateGuest, createGuest, checkInGuest, checkOutGuest, unassignSeat, getGuest, ConflictError } from '$lib/api/guests';
  import { get } from 'svelte/store';
  let { guest, tables = [], onClose, startEditing = false, createMode = false, readonly = false, showSeatNumbers = true }: { guest?: Guest; tables?: BanquetTable[]; onClose: () => void; startEditing?: boolean; createMode?: boolean; readonly?: boolean; showSeatNumbers?: boolean } = $props();

  let tableName = $derived(guest ? (tables.find(t => t.id === guest.tableId)?.name ?? guest.tableId ?? '—') : '—');

  let editing = $state(startEditing || createMode);
  let saving = $state(false);

  let localGuest = $state(guest);
  $effect(() => { localGuest = guest; });

  let showUnassignConfirm = $state(false);

  async function refreshGuest() {
    if (!guest?.id) return;
    const wid = get(weddingId);
    try {
      const fresh = await getGuest(wid, guest.id);
      // Build updated guest without mutating the prop directly
      localGuest = {
        ...guest,
        name: fresh.name,
        phone: fresh.phone,
        email: fresh.email,
        pax: fresh.pax,
        rsvp: fresh.rsvp,
        isVip: fresh.isVip,
        notes: fresh.notes,
        dietaryRequirements: fresh.dietary,
        checkedIn: !!fresh.checkedInAt,
        checkedInAt: fresh.checkedInAt ? new Date(fresh.checkedInAt) : undefined,
        angbaoAmount: fresh.angbaoAmt ?? undefined,
        giftItem: fresh.giftItem ?? undefined,
      } as Guest;
    } catch {}
  }

  let dragY = $state(0);
  let dragging = $state(false);
  let pendingDrag = $state(false);
  let startY = $state(0);
  // Velocity tracking: last 3 samples for averaging
  const velSamples: { y: number; t: number }[] = [];
  const DISMISS_THRESHOLD = 100;
  const VELOCITY_THRESHOLD = 0.5; // px/ms — a moderate flick
  const RUBBER_BAND = 0.4;
  const DRAG_THRESHOLD = 8; // px before committing to drawer drag vs scroll

  function rubberband(delta: number): number {
    // Progressive resistance: the further you drag, the less it follows
    return (delta * RUBBER_BAND);
  }

  function projectVelocity(velocity: number): number {
    // Apple's exponential decay projection: v/1000 * d/(1-d), d≈0.998
    const d = 0.998;
    return (velocity / 1000) * d / (1 - d);
  }

  function getScrollableBody(panel: EventTarget | null): HTMLElement | null {
    if (panel instanceof HTMLElement) {
      return panel.querySelector('.drawer-body');
    }
    return null;
  }

  function onTouchStart(e: TouchEvent) {
    if (window.innerWidth >= 640) return;
    const panel = e.currentTarget;
    const scroller = getScrollableBody(panel);
    // If inner content is scrolled, don't hijack the gesture
    if (scroller && scroller.scrollTop > 0) return;
    // Interrupt: read current computed transform to avoid snap-back stutter
    if (panel instanceof HTMLElement) {
      const cs = getComputedStyle(panel);
      const match = cs.transform.match(/matrix.*\((.+)\)/);
      if (match) {
        const m = match[1].split(', ');
        const currentY = parseFloat(m[5]) || 0;
        startY = e.touches[0].clientY - currentY;
      } else {
        startY = e.touches[0].clientY;
      }
    } else {
      startY = e.touches[0].clientY;
    }
    velSamples.length = 0;
    velSamples.push({ y: e.touches[0].clientY, t: performance.now() });
    // Don't commit yet — wait for gesture intent
    pendingDrag = true;
    dragging = false;
  }

  function onTouchMove(e: TouchEvent) {
    if (!pendingDrag && !dragging) return;
    const panel = e.currentTarget;
    const scroller = getScrollableBody(panel);
    // If inner content is scrolled mid-gesture, bail
    if (scroller && scroller.scrollTop > 0) {
      pendingDrag = false;
      dragging = false;
      return;
    }
    // During pending phase: only commit to drag after 8px vertical movement
    if (pendingDrag && !dragging) {
      const dy = e.touches[0].clientY - startY;
      if (dy <= DRAG_THRESHOLD) return; // let browser scroll
      pendingDrag = false;
      dragging = true;
    }
    const raw = e.touches[0].clientY - startY;
    // Rubber-band: apply progressive dampening for downward drag
    dragY = raw > 0 ? rubberband(raw) : raw;
    // Track velocity (keep last 3 samples)
    const now = performance.now();
    velSamples.push({ y: e.touches[0].clientY, t: now });
    if (velSamples.length > 3) velSamples.shift();
  }

  function onTouchEnd() {
    if (!dragging && !pendingDrag) return;
    pendingDrag = false;
    dragging = false;
    // Calculate release velocity from recent samples
    let velocity = 0;
    if (velSamples.length >= 2) {
      const last = velSamples[velSamples.length - 1];
      const first = velSamples[0];
      const dt = last.t - first.t;
      if (dt > 0) velocity = (last.y - first.y) / dt;
    }
    // Project where the gesture is heading
    const projected = dragY + projectVelocity(velocity) * 16;
    // Dismiss if: past threshold OR fast flick downward
    if (projected > DISMISS_THRESHOLD || velocity > VELOCITY_THRESHOLD) {
      onClose();
    } else {
      // Snap back: let CSS transition handle it by resetting dragY next frame
      requestAnimationFrame(() => { dragY = 0; });
    }
  }

  let form = $state({
    name: guest?.name ?? '',
    phone: guest?.phone ?? '',
    email: guest?.email ?? '',
    pax: guest?.pax ?? 1,
    rsvp: (guest?.rsvp ?? 'pending') as RSVPStatus,
    isVip: guest?.isVip ?? false,
    notes: guest?.notes ?? '',
    dietary: guest?.dietaryRequirements ?? [],
    angbaoAmt: guest?.angbaoAmount != null ? String(guest.angbaoAmount) : '',
    giftItem: guest?.giftItem ?? '',
  });

  $effect(() => {
    if (!guest) return;
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
      if (createMode) {
        const created = await createGuest(wid, {
          name: form.name, phone: form.phone, email: form.email || undefined,
          pax: form.pax, rsvp: form.rsvp, isVip: form.isVip, notes: form.notes,
          dietary: form.dietary, angbaoAmt: form.angbaoAmt ? Number(form.angbaoAmt) : null,
          giftItem: form.giftItem || null,
        });
        addToast(`${created.name} created`, 'success');
        track('guest_created', { wedding_id: wid });
        onClose();
      } else {
        const updated = await updateGuest(wid, guest!.id, {
          name: form.name, phone: form.phone, email: form.email || undefined,
          pax: form.pax, rsvp: form.rsvp, isVip: form.isVip, notes: form.notes,
          dietary: form.dietary, angbaoAmt: form.angbaoAmt ? Number(form.angbaoAmt) : null,
          giftItem: form.giftItem || null,
        });
        Object.assign(guest!, {
          name: updated.name, phone: updated.phone, email: updated.email,
          pax: updated.pax, rsvp: updated.rsvp, isVip: updated.isVip,
          notes: updated.notes, dietaryRequirements: updated.dietary,
          angbaoAmount: updated.angbaoAmt ?? undefined, giftItem: updated.giftItem ?? undefined,
        });
        Object.assign(form, {
          angbaoAmt: updated.angbaoAmt != null ? String(updated.angbaoAmt) : '',
          giftItem: updated.giftItem ?? '',
        });
        editing = false;
        addToast('Guest updated', 'success');
        track('guest_updated', { wedding_id: wid });
        await refreshGuest();
      }
    } catch (e: any) {
      addToast(e.message ?? 'Save failed', 'error');
    } finally {
      saving = false;
    }
  }

  function cancel() { editing = false; onClose(); }

  async function handleCheckIn() {
    if (!guest) return;
    const wid = get(weddingId);
    try {
      await checkInGuest(wid, guest.id);
      guest.checkedIn = true;
      guest.checkedInAt = new Date();
      localGuest = { ...guest };
      addToast(`${guest.name} checked in`, 'success');
      track('guest_checked_in', { wedding_id: wid });
    } catch (e: any) {
      if (e instanceof ConflictError) {
        addToast(`${guest.name} was already checked in`, 'info');
      } else {
        addToast(e.message ?? 'Check-in failed', 'error');
      }
    }
  }

  async function handleCheckOut() {
    if (!guest) return;
    const wid = get(weddingId);
    try {
      await checkOutGuest(wid, guest.id);
      guest.checkedIn = false;
      guest.checkedInAt = undefined;
      localGuest = { ...guest };
      addToast(`${guest.name} checked out`, 'success');
      track('guest_checked_out', { wedding_id: wid });
      await refreshGuest();
    } catch (e: any) {
      addToast(e.message ?? 'Check-out failed', 'error');
    }
  }

  async function handleUnassign() {
    if (!guest || !localGuest?.tableId) return;
    showUnassignConfirm = true;
  }

  async function confirmUnassign() {
    if (!guest) return;
    showUnassignConfirm = false;
    const wid = get(weddingId);
    try {
      await unassignSeat(wid, guest.id);
      guest.tableId = null;
      guest.seatNumber = null;
      localGuest = { ...guest };
      addToast(`${guest.name} unassigned from table`, 'success');
    } catch (e: any) {
      addToast(e.message ?? 'Unassign failed', 'error');
    }
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<svelte:window onkeydown={(e) => { if (e.key === 'Escape') onClose(); }} />
<div class="drawer-overlay" onclick={onClose} role="presentation">
  <div class="drawer-backdrop" style="opacity: {dragging ? Math.max(0, 1 - dragY / 400) : 1}" aria-hidden="true"></div>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="drawer-panel" role="dialog" aria-modal="true" aria-labelledby="drawer-title" onclick={(e) => e.stopPropagation()}
    ontouchstart={onTouchStart}
    ontouchmove={onTouchMove}
    ontouchend={onTouchEnd}
    style="transform: translateY({dragY}px); transition: {dragging ? 'none' : 'transform 300ms cubic-bezier(0.2, 0.8, 0.2, 1)'}"
  >
    <div class="drawer-pill flex sm:hidden" onclick={onClose} role="presentation">
      <div class="drawer-pill-bar"></div>
    </div>
    <div class="drawer-header">
      <h2 id="drawer-title" class="drawer-title">{createMode ? 'New Guest' : editing ? 'Edit Guest' : 'Guest Details'}</h2>
      <div class="drawer-actions">
        {#if !readonly}
          <button onclick={() => editing = true} class="drawer-icon-btn flex" aria-label="Edit">
            <Pencil class="w-5 h-5" />
          </button>
          <button onclick={onClose} class="drawer-icon-btn hidden sm:flex" aria-label="Close">
            <X class="w-5 h-5" />
          </button>
        {:else}
          <button onclick={onClose} class="drawer-icon-btn flex" aria-label="Close">
            <X class="w-5 h-5" />
          </button>
        {/if}
      </div>
    </div>

    <div class="drawer-body" class:drawer-body-view={!editing && localGuest}>
      {#if editing}
        <div class="form-grid">
          <div class="form-field">
            <label for="guest-name" class="form-label">Name</label>
            <input id="guest-name" bind:value={form.name} class="form-input" />
          </div>
          <div class="form-field">
            <label for="guest-phone" class="form-label">Phone</label>
            <input id="guest-phone" bind:value={form.phone} class="form-input" />
          </div>
          <div class="form-field">
            <label for="guest-email" class="form-label">Email</label>
            <input id="guest-email" bind:value={form.email} class="form-input" />
          </div>
          <div class="form-row-2">
            <div class="form-field">
              <label for="guest-pax" class="form-label">Pax</label>
              <input id="guest-pax" type="number" min="1" bind:value={form.pax} class="form-input" />
            </div>
            <div class="form-field">
              <label for="guest-rsvp" class="form-label">RSVP</label>
              <select id="guest-rsvp" bind:value={form.rsvp} class="form-input">
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
            <label for="guest-notes" class="form-label">Notes</label>
            <textarea id="guest-notes" bind:value={form.notes} rows="2" class="form-input form-textarea"></textarea>
          </div>
          <div class="form-field">
            <div class="form-label">Dietary</div>
            <div class="dietary-chips">
              {#each ['Halal', 'Vegetarian', 'Vegan', 'Gluten-Free', 'Nut-Free', 'No Seafood'] as opt}
                <button type="button"
                  onclick={() => { form.dietary = form.dietary.includes(opt) ? form.dietary.filter(d => d !== opt) : [...form.dietary, opt]; }}
                  class={cn("dietary-chip", form.dietary.includes(opt) ? "dietary-chip-active" : "")}
                >{opt}</button>
              {/each}
            </div>
          </div>
          <div class="form-field">
            <label for="guest-angbao" class="form-label">Angbao Amount (RM)</label>
            <input id="guest-angbao" type="number" min="0" bind:value={form.angbaoAmt} placeholder="0" class="form-input" />
          </div>
          <div class="form-field">
            <label for="guest-gift" class="form-label">Gift Item</label>
            <input id="guest-gift" bind:value={form.giftItem} placeholder="e.g. Kitchenware set" class="form-input" />
          </div>
        </div>
      {:else if localGuest}
        <!-- View Mode — compact for mobile -->
        <div class="guest-hero">
          <div class={cn(
            "guest-avatar-lg",
            localGuest.checkedIn ? "avatar-checked" : localGuest.isVip ? "avatar-vip" : "avatar-default"
          )}>
            {getInitials(localGuest.name)}
          </div>
          <div class="min-w-0">
            <h3 class="guest-name-lg">{#if localGuest.isVip}<span class="text-gold">★</span>{/if} {localGuest.name}</h3>
            <div class="flex items-center gap-2 mt-0.5">
              <Badge status={localGuest.rsvp} />
              {#if localGuest.pax > 1}
                <span class="text-xs text-gray-400">{localGuest.pax} pax</span>
              {/if}
            </div>
          </div>
        </div>

        <div class="info-rows">
          <div class="info-row">
            <Phone class="info-icon" />
            <span>{localGuest.phone}</span>
          </div>
          {#if localGuest.email}
            <div class="info-row">
              <Mail class="info-icon" />
              <span class="truncate">{localGuest.email}</span>
            </div>
          {/if}
        </div>

        <div class="detail-grid">
          <div class="detail-card">
            <div class="detail-label">Table</div>
            <div class="detail-value">{tableName}</div>
          </div>
          {#if showSeatNumbers}
            <div class="detail-card">
              <div class="detail-label">Seat{localGuest.pax > 1 ? 's' : ''}</div>
              <div class="detail-value">{formatSeatRange(localGuest.seatNumber, localGuest.pax)}</div>
            </div>
          {/if}
          <div class="detail-card">
            <div class="detail-label">Party Size</div>
            <div class="detail-value">{localGuest.pax}</div>
          </div>
          <div class="detail-card">
            <div class="detail-label">Checked In</div>
            <div class="detail-value {localGuest.checkedIn ? 'text-emerald-600' : 'text-gray-400'}">
              {localGuest.checkedIn ? '✓' : '—'}
            </div>
          </div>
        </div>

        {#if localGuest.isVip}
          <div class="vip-banner">⭐ VIP Guest</div>
        {/if}

        {#if localGuest.dietaryRequirements.length > 0}
          <div class="compact-section">
            <div class="section-header">
              <Utensils class="section-icon" />
              Dietary
            </div>
            <div class="chip-group">
              {#each localGuest.dietaryRequirements as req}
                <span class="dietary-tag">{req}</span>
              {/each}
            </div>
          </div>
        {/if}

        {#if localGuest.notes}
          <div class="compact-section">
            <div class="section-header">
              <StickyNote class="section-icon" />
              Notes
            </div>
            <p class="notes-text">{localGuest.notes}</p>
          </div>
        {/if}

        {#if localGuest.angbaoAmount || localGuest.giftItem}
          <div class="compact-section">
            <div class="section-header">Gift Details</div>
            <div class="gift-card">
              {#if localGuest.angbaoAmount}
                <div class="gift-row">
                  <Banknote class="gift-icon text-emerald-600" />
                  <span class="gift-label">Angbao:</span>
                  <span class="gift-value text-emerald-700">RM {localGuest.angbaoAmount}</span>
                </div>
              {/if}
              {#if localGuest.giftItem}
                <div class="gift-row">
                  <Gift class="gift-icon text-gold" />
                  <span class="gift-label">Gift:</span>
                  <span class="gift-value text-gold-dark">{localGuest.giftItem}</span>
                </div>
              {/if}
            </div>
          </div>
        {/if}

        {#if !readonly}
          <div class="pt-1 pb-3 sm:pb-4">
            {#if localGuest.checkedIn}
              <button onclick={handleCheckOut} class="w-full py-2.5 sm:py-3 bg-emerald-50 text-emerald-700 rounded-xl text-sm font-semibold border border-emerald-200 hover:bg-emerald-100 transition-colors flex items-center justify-center gap-2">
                <CheckCircle2 class="w-4 h-4" /> Checked In — Tap to Check Out
              </button>
            {:else}
              <button onclick={handleCheckIn} class="w-full py-2.5 sm:py-3 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2">
                <UserCheck class="w-4 h-4" /> Check In
              </button>
            {/if}
            {#if localGuest.tableId}
              <button onclick={handleUnassign} class="w-full py-2.5 sm:py-3 bg-white text-gray-600 rounded-xl text-sm font-semibold border border-gray-200 hover:bg-gray-50 transition-colors flex items-center justify-center gap-2 mt-2">
                <ArrowUpDown class="w-4 h-4" /> Unassign Table
              </button>
            {/if}
          </div>
        {/if}
      {/if}
    </div>

    {#if editing}
      <div class="drawer-footer flex">
        <button onclick={save} disabled={saving}
          class="w-full py-3 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2">
          <Check class="w-4 h-4" /> {saving ? 'Saving...' : 'Save'}
        </button>
      </div>
    {/if}
  </div>
</div>

<ConfirmDialog
  open={showUnassignConfirm}
  title="Unassign Table"
  message={`Remove ${guest?.name ?? 'this guest'} from their table? They will no longer have an assigned seat.`}
  confirmLabel="Unassign"
  cancelLabel="Cancel"
  variant="warning"
  onConfirm={confirmUnassign}
  onCancel={() => showUnassignConfirm = false}
/>

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
    .drawer-overlay { align-items: stretch; }
  }

  .drawer-backdrop {
    position: absolute;
    inset: 0;
    background: rgba(0, 0, 0, 0.3);
    backdrop-filter: blur(4px);
    -webkit-backdrop-filter: blur(4px);
    transition: opacity 300ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }

  .drawer-panel {
    position: relative;
    width: 100%;
    max-height: 85dvh;
    border-radius: 1.25rem 1.25rem 0 0;
    padding-top: env(safe-area-inset-top);
    padding-bottom: env(safe-area-inset-bottom);
    background: rgba(255, 255, 255, 0.95);
    backdrop-filter: blur(24px) saturate(200%);
    -webkit-backdrop-filter: blur(24px) saturate(200%);
    box-shadow: 0 -8px 48px rgba(0, 0, 0, 0.15), 0 -2px 8px rgba(0, 0, 0, 0.08);
    display: flex;
    flex-direction: column;
    overflow: hidden;
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
    justify-content: center;
    padding: 0.5rem 0 0.75rem;
    cursor: pointer;
  }

  .drawer-pill-bar {
    width: 2.5rem;
    height: 0.25rem;
    background: rgba(0, 0, 0, 0.15);
    border-radius: 9999px;
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
    padding: 1rem 1.5rem;
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

  .drawer-body {
    padding: 1.25rem 1.5rem;
    padding-bottom: 1rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    -webkit-overflow-scrolling: touch;
  }

  /* View mode: tighter spacing on mobile */
  .drawer-body-view {
    gap: 0.75rem;
  }

  @media (min-width: 640px) {
    .drawer-body { padding-bottom: 1rem; gap: 1.25rem; }
    .drawer-body-view { gap: 1rem; }
  }

  .drawer-footer {
    flex-shrink: 0;
    background: rgba(255, 255, 255, 0.95);
    backdrop-filter: blur(24px) saturate(200%);
    -webkit-backdrop-filter: blur(24px) saturate(200%);
    padding: 0.75rem 1.5rem;
    padding-bottom: calc(0.75rem + env(safe-area-inset-bottom));
    border-top: 1px solid rgba(0, 0, 0, 0.05);
    align-items: center;
    gap: 0.75rem;
  }

  /* View mode */
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
  }

  @media (min-width: 640px) {
    .guest-avatar-lg { width: 3.5rem; height: 3.5rem; font-size: 1rem; }
  }

  .avatar-default { background: #FDEAEA; color: #A11217; border: 2px solid #FAC5C5; }
  .avatar-vip { background: #FDF8E8; color: #B8941F; border: 2px solid #E8CC6E; }
  .avatar-checked { background: #ECFDF5; color: #059669; border: 2px solid #A7F3D0; }

  .guest-name-lg {
    font-size: 1.125rem;
    font-weight: 700;
    color: #111827;
    letter-spacing: -0.01em;
  }

  @media (min-width: 640px) {
    .guest-name-lg { font-size: 1.25rem; }
  }

  .info-rows {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
  }

  .info-row {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    font-size: 0.8125rem;
    color: #4b5563;
  }

  .info-icon {
    width: 0.875rem;
    height: 0.875rem;
    color: #9ca3af;
    flex-shrink: 0;
  }

  .detail-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.5rem;
  }

  @media (min-width: 640px) {
    .detail-grid { gap: 0.75rem; }
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

  @media (min-width: 640px) {
    .detail-card { padding: 1rem; }
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

  @media (min-width: 640px) {
    .detail-value { font-size: 1.5rem; }
  }

  .vip-banner {
    background: #FDF8E8;
    border: 1px solid rgba(212, 175, 55, 0.3);
    border-radius: 0.75rem;
    padding: 0.75rem;
    text-align: center;
    font-weight: 700;
    font-size: 0.8125rem;
    color: #B8941F;
  }

  .compact-section {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
  }

  .section-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.8125rem;
    font-weight: 600;
    color: #374151;
  }

  .section-icon {
    width: 0.875rem;
    height: 0.875rem;
    color: #9ca3af;
  }

  .chip-group {
    display: flex;
    flex-wrap: wrap;
    gap: 0.375rem;
  }

  .dietary-tag {
    padding: 0.25rem 0.625rem;
    background: #FFFBEB;
    color: #D97706;
    border-radius: 9999px;
    font-size: 0.6875rem;
    font-weight: 500;
    border: 1px solid #FDE68A;
  }

  .notes-text {
    font-size: 0.8125rem;
    color: #4b5563;
    background: rgba(249, 250, 251, 0.8);
    border: 1px solid rgba(0, 0, 0, 0.04);
    border-radius: 0.75rem;
    padding: 0.75rem;
    line-height: 1.5;
  }

  .gift-card {
    background: #FDF8E8;
    border: 1px solid rgba(212, 175, 55, 0.2);
    border-radius: 0.75rem;
    padding: 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
  }

  .gift-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.8125rem;
  }

  .gift-icon { width: 0.875rem; height: 0.875rem; flex-shrink: 0; }
  .gift-label { color: #6b7280; }
  .gift-value { font-weight: 700; }

  /* Edit mode */
  .form-grid {
    display: flex;
    flex-direction: column;
    gap: 0.875rem;
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
    border-radius: 0.75rem;
    font-size: 0.9375rem;
    color: #111827;
    background: rgba(255, 255, 255, 0.8);
    outline: none;
    transition: border-color 200ms ease, box-shadow 200ms ease, transform 100ms ease;
    min-height: 44px;
  }

  @media (min-width: 640px) {
    .form-input { padding: 0.75rem 1rem; min-height: 48px; }
  }

  .form-input:focus {
    border-color: #A11217;
    box-shadow: 0 0 0 3px rgba(161, 18, 23, 0.1);
  }

  .form-input:active { transform: scale(0.99); }

  .form-textarea {
    resize: none;
    min-height: 3rem;
  }

  .form-row-2 {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
  }

  .dietary-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.375rem;
  }

  .dietary-chip {
    padding: 0.3125rem 0.625rem;
    border-radius: 0.5rem;
    font-size: 0.6875rem;
    font-weight: 500;
    border: 1.5px solid rgba(0, 0, 0, 0.08);
    color: #4b5563;
    background: white;
    transition: all 150ms ease, transform 100ms ease;
  }

  .dietary-chip:active { transform: scale(0.95); }

  .dietary-chip-active {
    background: #FDF8E8;
    border-color: #D4AF37;
    color: #B8941F;
  }

  @keyframes slideUp { from { transform: translateY(100%); } to { transform: translateY(0); } }
  @keyframes slideInRight { from { transform: translateX(100%); } to { transform: translateX(0); } }
</style>
