<script lang="ts">
  import { toasts } from '$lib/stores';
  import { CheckCircle, XCircle, Info } from 'lucide-svelte';
  import { toastOut } from '$lib/utils/motion';
</script>

<div class="toast-container" aria-live="polite" role="status">
  {#each $toasts as toast (toast.id)}
    <div class="toast-item toast-{toast.type}" out:toastOut>
      {#if toast.type === 'success'}
        <CheckCircle class="toast-icon toast-icon-success" />
      {:else if toast.type === 'error'}
        <XCircle class="toast-icon toast-icon-error" />
      {:else}
        <Info class="toast-icon toast-icon-info" />
      {/if}
      <span class="toast-message">{toast.message}</span>
    </div>
  {/each}
</div>

<style>
  .toast-container {
    position: fixed;
    top: 1.25rem;
    right: 1.25rem;
    z-index: 1000;
    display: flex;
    flex-direction: column;
    gap: 0.625rem;
  }

  .toast-item {
    background: rgba(255, 255, 255, 0.92);
    backdrop-filter: blur(20px) saturate(180%);
    -webkit-backdrop-filter: blur(20px) saturate(180%);
    border: 1px solid rgba(0, 0, 0, 0.06);
    border-radius: 0.875rem;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.12), inset 0 1px 0 rgba(255, 255, 255, 0.9);
    padding: 0.875rem 1.25rem;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    min-width: 18rem;
    /* Transition (not keyframes): rapid successive toasts retarget instead of restarting.
       Entry via @starting-style; exit handled by the WAAPI out:toastOut. */
    transition:
      opacity 350ms cubic-bezier(0.2, 0.8, 0.2, 1),
      transform 350ms cubic-bezier(0.2, 0.8, 0.2, 1);
    opacity: 1;
    transform: translateX(0) scale(1);
  }

  @starting-style {
    .toast-item {
      opacity: 0;
      transform: translateX(1rem) scale(0.95);
    }
  }

  .toast-success { border-left: 3px solid #059669; }
  .toast-error { border-left: 3px solid #DC2626; }
  .toast-info { border-left: 3px solid #2563EB; }

  .toast-icon {
    width: 1.25rem;
    height: 1.25rem;
    flex-shrink: 0;
  }

  .toast-icon-success { color: #059669; }
  .toast-icon-error { color: #DC2626; }
  .toast-icon-info { color: #2563EB; }

  .toast-message {
    font-size: 0.875rem;
    font-weight: 500;
    color: #374151;
  }
</style>
