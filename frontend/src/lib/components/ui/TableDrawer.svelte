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

  @keyframes slideUp { from { transform: translateY(100%); } to { transform: translateY(0); } }
  @keyframes slideInRight { from { transform: translateX(100%); } to { transform: translateX(0); } }
</style>
