<script lang="ts">
  import { X, Check } from 'lucide-svelte';

  let {
    title = 'Add Table',
    name = $bindable(''),
    capacity = $bindable(10),
    isVip = $bindable(false),
    saving = false,
    onSave,
    onClose,
  }: {
    title?: string;
    name?: string;
    capacity?: number;
    isVip?: boolean;
    saving?: boolean;
    onSave: () => void;
    onClose: () => void;
  } = $props();
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

    <div class="drawer-body">
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
    </div>

    <div class="drawer-footer flex">
      <button onclick={onSave} disabled={saving}
        class="w-full py-3 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2">
        <Check class="w-4 h-4" /> {saving ? 'Saving...' : title === 'Edit Table' ? 'Save Changes' : 'Create Table'}
      </button>
    </div>
  </div>
</div>

<style>
  @import './drawer.css';
</style>
