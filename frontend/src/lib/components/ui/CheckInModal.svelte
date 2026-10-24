<script lang="ts">
  import X from 'lucide-svelte/icons/x';
import UserCheck from 'lucide-svelte/icons/user-check';
import Loader2 from 'lucide-svelte/icons/loader-2';
  import { fade } from 'svelte/transition';
  import { onMount } from 'svelte';
  import { slideOut as sheetSlideOut } from '$lib/utils/motion';

  function outfade(node: HTMLElement) {
    return { duration: sheetSlideOut(node, { dir: 'y', duration: 300 }) };
  }

  let {
    guestName,
    onConfirm,
    onClose,
    loading = false,
  }: {
    guestName: string;
    onConfirm: (notes: string) => void;
    onClose: () => void;
    loading?: boolean;
  } = $props();

  let notes = $state('');
  let confirmBtn: HTMLButtonElement;

  function onNotesKeydown(e: KeyboardEvent) {
    // Enter confirms; isComposing guards IME (Chinese) input.
    if (e.key === 'Enter' && !e.isComposing && !loading) {
      e.preventDefault();
      onConfirm(notes);
    }
  }

  // Swipe-to-dismiss on mobile
  let dragY = $state(0);
  let dragging = $state(false);
  let startY = $state(0);

  function onTouchStart(e: TouchEvent) {
    if (window.innerWidth >= 640) return;
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
    if (dragY > 80) onClose();
    dragY = 0;
  }
  function onDialogKeydown(e: KeyboardEvent) {
    if (e.key !== 'Tab') return;
    const nodes = (e.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('button:not([disabled]), input');
    if (!nodes.length) return;
    const first = nodes[0];
    const last = nodes[nodes.length - 1];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  }
  onMount(() => confirmBtn?.focus());
</script>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape' && !loading) onClose(); }} />
<div class="fixed inset-0 z-[60] flex items-end sm:items-center justify-center p-0 sm:p-4" role="dialog" aria-modal="true" aria-labelledby="check-in-title">
  <div class="absolute inset-0 bg-black/30 backdrop-blur-md" onclick={onClose} role="presentation" transition:fade={{ duration: 250 }}></div>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    onkeydown={onDialogKeydown}
    class="relative bg-white/95 backdrop-blur-xl rounded-t-2xl sm:rounded-2xl shadow-2xl w-full sm:max-w-md overflow-hidden pb-[env(safe-area-inset-bottom)] sm:pb-0"
    out={outfade}
    style="transform: translateY({dragY}px); transition: {dragging ? 'none' : 'transform 300ms cubic-bezier(0.2, 0.8, 0.2, 1)'}"
    ontouchstart={onTouchStart}
    ontouchmove={onTouchMove}
    ontouchend={onTouchEnd}
  >
    <!-- Pill dismiss (mobile only) - swipe handle -->
    <div class="flex justify-center pt-3 sm:hidden" role="presentation">
      <div class="w-10 h-1 bg-gray-300 rounded-full"></div>
    </div>
    <div class="flex items-center justify-between p-5 border-b border-gray-100">
      <h3 id="check-in-title" class="font-bold text-gray-900">Check In {guestName}</h3>
      <button onclick={onClose} aria-label="Close" class="min-w-[44px] min-h-[44px] rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
        <X class="w-4 h-4 text-gray-400" />
      </button>
    </div>
    <div class="p-5 space-y-4">
      <p class="text-sm text-gray-600">Mark {guestName} as arrived. Angpao and gifts are recorded separately from the guest list.</p>
      <div>
        <label for="checkin-notes" class="block text-sm font-medium text-gray-700 mb-1.5">
          备注 <span class="text-gray-400 font-normal">(可选)</span>
        </label>
        <input
          id="checkin-notes"
          type="text"
          bind:value={notes}
          onkeydown={onNotesKeydown}
          placeholder="只到1人 / 加凳 / 缺席但是送礼"
          class="w-full px-4 py-3 rounded-xl border border-black/[0.08] bg-white text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none focus:ring-2 focus:ring-red/20 focus:border-red transition-shadow"
        />
      </div>
    </div>
    <div class="flex gap-3 p-5 pt-0">
      <button bind:this={confirmBtn} onclick={() => onConfirm(notes)} disabled={loading}
        class="flex-1 py-3 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2 disabled:opacity-50">
        {#if loading}
          <Loader2 class="w-4 h-4 text-white animate-spin" /> Processing...
        {:else}
          <UserCheck class="w-4 h-4" /> Confirm Check In
        {/if}
      </button>
      <button onclick={onClose}
        class="px-6 py-3 border border-black/[0.06] bg-white/90 rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
        Cancel
      </button>
    </div>
  </div>
</div>
