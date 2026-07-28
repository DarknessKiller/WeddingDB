<script lang="ts">
  import { X, Banknote, Gift, UserCheck, Loader2 } from 'lucide-svelte';

  let {
    guestName,
    angbaoAmount = $bindable(''),
    giftItem = $bindable(''),
    onConfirm,
    onClose,
    loading = false,
  }: {
    guestName: string;
    angbaoAmount?: string;
    giftItem?: string;
    onConfirm: () => void;
    onClose: () => void;
    loading?: boolean;
  } = $props();

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
</script>

<div class="fixed inset-0 z-[60] flex items-end sm:items-center justify-center p-0 sm:p-4">
  <div class="absolute inset-0 bg-black/30 backdrop-blur-md" onclick={onClose} role="presentation"></div>
  <div
    class="relative bg-white/95 backdrop-blur-xl rounded-t-2xl sm:rounded-2xl shadow-2xl w-full sm:max-w-md overflow-hidden pb-[env(safe-area-inset-bottom)] sm:pb-0"
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
      <h3 class="font-bold text-gray-900">Check In {guestName}</h3>
      <button onclick={onClose} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors hidden sm:flex">
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
      <button onclick={onConfirm} disabled={loading}
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
