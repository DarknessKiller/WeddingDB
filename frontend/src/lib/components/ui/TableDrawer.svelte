<script lang="ts">
  import { X, Check, Pencil, Star, Users } from 'lucide-svelte';
  import { cn } from '$lib/utils';

  let {
    title = 'Add Table',
    name = $bindable(''),
    capacity = $bindable(10),
    isVip = $bindable(false),
    occupied = 0,
    saving = false,
    startEditing = false,
    onSave,
    onClose,
  }: {
    title?: string;
    name?: string;
    capacity?: number;
    isVip?: boolean;
    occupied?: number;
    saving?: boolean;
    startEditing?: boolean;
    onSave: () => void;
    onClose: () => void;
  } = $props();

  let editing = $state(startEditing);
  const isEdit = $derived(title === 'Edit Table');
  const percentage = $derived(capacity > 0 ? Math.round((occupied / capacity) * 100) : 0);
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
      <h2 id="drawer-title" class="drawer-title">{title}</h2>
      <div class="drawer-actions">
        <button onclick={onClose} class="drawer-icon-btn flex" aria-label="Close">
          <X class="w-5 h-5" />
        </button>
      </div>
    </div>

    <div class="drawer-body" class:drawer-body-view={!editing && isEdit}>
      {#if editing}
        <div class="form-grid">
          <div class="form-field">
            <label for="table-name" class="form-label">Table Name</label>
            <input id="table-name" type="text" bind:value={name} placeholder="e.g. Table 1, VIP A, 圆桌" class="form-input" />
          </div>
          <div class="form-field">
            <label for="table-capacity" class="form-label">Capacity</label>
            <input id="table-capacity" type="number" min="1" bind:value={capacity} class="form-input" />
          </div>
          <label class="form-check-label">
            <input type="checkbox" bind:checked={isVip} class="rounded" /> VIP Table
          </label>
        </div>
      {:else}
        <!-- Table summary — matches guest drawer's view mode -->
        <div class="table-hero">
          <div class={cn("table-avatar", isVip ? "avatar-vip" : "avatar-table")}>
            {name || 'T'}
          </div>
          <div class="min-w-0">
            <h3 class="table-name-lg">{#if isVip}<span class="text-gold">★</span>{/if} {name || 'Table'}</h3>
            <div class="flex items-center gap-2 mt-0.5">
              <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-emerald-50 text-emerald-700 rounded-full text-xs font-semibold border border-emerald-200">
                <Users class="w-3 h-3" /> {occupied}/{capacity}
              </span>
              {#if isVip}
                <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-gold-50 text-gold border border-gold-200 rounded-full text-[11px] font-bold">
                  <Star class="w-3 h-3 fill-gold" /> VIP
                </span>
              {/if}
            </div>
          </div>
        </div>

        <div class="detail-grid">
          <div class="detail-card">
            <div class="detail-label">Occupancy</div>
            <div class="detail-value">{percentage}%</div>
          </div>
          <div class="detail-card">
            <div class="detail-label">Seats Filled</div>
            <div class="detail-value">{occupied}/{capacity}</div>
          </div>
        </div>

        <div class="pt-1 pb-3 sm:pb-4">
          <button onclick={() => editing = true} class="w-full py-2.5 sm:py-3 bg-white text-gray-600 rounded-xl text-sm font-semibold border border-gray-200 hover:bg-gray-50 transition-colors flex items-center justify-center gap-2">
            <Pencil class="w-4 h-4" /> Edit Table
          </button>
        </div>
      {/if}
    </div>

    {#if editing}
      <div class="drawer-footer flex">
        <button onclick={onSave} disabled={saving}
          class="w-full py-3 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2">
          <Check class="w-4 h-4" /> {saving ? 'Saving...' : 'Save'}
        </button>
      </div>
    {/if}
  </div>
</div>

<style>
  @import './drawer.css';

  /* Table summary — occupancy ring matching the tables grid cards */
  .table-hero {
    display: flex;
    align-items: center;
    gap: 0.875rem;
  }

  .table-avatar {
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
    .table-avatar { width: 3.5rem; height: 3.5rem; font-size: 1rem; }
  }

  .avatar-table { background: #FDEAEA; color: #A11217; border: 2px solid #FAC5C5; }
  .avatar-vip { background: #FDF8E8; color: #B8941F; border: 2px solid #E8CC6E; }

  .table-name-lg {
    font-size: 1.125rem;
    font-weight: 700;
    color: #111827;
    letter-spacing: -0.01em;
  }

  @media (min-width: 640px) {
    .table-name-lg { font-size: 1.25rem; }
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
</style>
