<script lang="ts">
  import { X, AlertTriangle } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { fade } from 'svelte/transition';

  let {
    open = false,
    title = 'Confirm Action',
    message = '',
    confirmLabel = 'Confirm',
    cancelLabel = 'Cancel',
    variant = 'danger',
    onConfirm,
    onCancel
  }: {
    open: boolean;
    title?: string;
    message?: string;
    confirmLabel?: string;
    cancelLabel?: string;
    variant?: 'danger' | 'warning' | 'info';
    onConfirm: () => void;
    onCancel: () => void;
  } = $props();

  let dialogEl: HTMLDivElement | null = $state(null);

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onCancel();
  }

  function handleBackdropClick() {
    onCancel();
  }

  function handleConfirm() {
    onConfirm();
  }

  function handleOverlayKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      handleConfirm();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="fixed inset-0 z-[800] flex items-center justify-center p-4"
    onclick={handleBackdropClick}
    role="dialog"
    aria-modal="true"
    aria-labelledby="confirm-title"
  >
    <!-- Backdrop -->
    <div class="absolute inset-0 bg-black/30 backdrop-blur-md" transition:fade={{ duration: 200 }}></div>

    <!-- Dialog -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      bind:this={dialogEl}
      class="relative bg-white/95 backdrop-blur-xl rounded-2xl shadow-2xl w-full max-w-sm overflow-hidden dialog-motion"
      onclick={(e) => e.stopPropagation()}
    >
      <!-- Icon + Content -->
      <div class="p-6 text-center">
        {#if variant === 'danger'}
          <div class="w-12 h-12 mx-auto mb-4 rounded-full bg-red-50 flex items-center justify-center">
            <AlertTriangle class="w-6 h-6 text-red" />
          </div>
        {:else if variant === 'warning'}
          <div class="w-12 h-12 mx-auto mb-4 rounded-full bg-amber-50 flex items-center justify-center">
            <AlertTriangle class="w-6 h-6 text-amber-500" />
          </div>
        {:else}
          <div class="w-12 h-12 mx-auto mb-4 rounded-full bg-blue-50 flex items-center justify-center">
            <AlertTriangle class="w-6 h-6 text-blue-500" />
          </div>
        {/if}

        <h3 id="confirm-title" class="text-lg font-semibold text-gray-900 mb-2">{title}</h3>
        {#if message}
          <p class="text-sm text-gray-600 leading-relaxed">{message}</p>
        {/if}
      </div>

      <!-- Actions -->
      <div class="flex border-t border-gray-100">
        <button
          onclick={onCancel}
          class="flex-1 px-4 py-3.5 text-sm font-medium text-gray-700 hover:bg-gray-50 active:bg-gray-100 transition-colors"
        >
          {cancelLabel}
        </button>
        <div class="w-px bg-gray-100"></div>
        <button
          onclick={handleConfirm}
          class="flex-1 px-4 py-3.5 text-sm font-medium transition-colors
            {variant === 'danger'
              ? 'text-red hover:bg-red-50 active:bg-red-100'
              : variant === 'warning'
                ? 'text-amber-600 hover:bg-amber-50 active:bg-amber-100'
                : 'text-blue-600 hover:bg-blue-50 active:bg-blue-100'}"
        >
          {confirmLabel}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  /* Modal: exempt from origin rules, stays centered. Entry via @starting-style,
     exit is instant (confirmation dialogs close on decision — no lingering motion). */
  .dialog-motion {
    transition:
      opacity 200ms cubic-bezier(0.2, 0.8, 0.2, 1),
      transform 200ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }

  @starting-style {
    .dialog-motion {
      opacity: 0;
      transform: scale(0.96);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .dialog-motion {
      transform: none;
    }
  }
</style>
