<script lang="ts">
  import type { Guest } from '$lib/types';
  import { X, Phone, Mail, Utensils, StickyNote, Banknote, Gift } from 'lucide-svelte';
  import Badge from './Badge.svelte';
  import { getInitials } from '$lib/utils';

  let { guest, onClose }: { guest: Guest; onClose: () => void } = $props();
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="fixed inset-0 z-50 flex justify-end" onclick={onClose}>
  <div class="absolute inset-0 bg-black/30 backdrop-blur-sm"></div>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="relative w-[400px] max-w-full bg-white shadow-2xl overflow-y-auto"
    onclick={(e) => e.stopPropagation()}
  >
    <div class="sticky top-0 bg-white z-10 px-6 py-5 border-b border-gray-100 flex items-center justify-between">
      <h2 class="text-lg font-bold text-gray-900">Guest Details</h2>
      <button onclick={onClose} class="p-2 rounded-lg hover:bg-gray-100 transition-colors" aria-label="Close">
        <X class="w-5 h-5 text-gray-500" />
      </button>
    </div>

    <div class="p-6 space-y-6">
      <!-- Avatar + Name -->
      <div class="flex items-center gap-4">
        <div class="w-14 h-14 rounded-full bg-red-50 flex items-center justify-center text-red font-bold text-lg">
          {getInitials(guest.name)}
        </div>
        <div>
          <h3 class="text-xl font-bold text-gray-900">{guest.name}</h3>
          <Badge status={guest.rsvp} />
        </div>
      </div>

      <!-- Info -->
      <div class="space-y-3">
        <div class="flex items-center gap-3 text-sm text-gray-600">
          <Phone class="w-4 h-4 text-gray-400" />
          {guest.phone}
        </div>
        {#if guest.email}
          <div class="flex items-center gap-3 text-sm text-gray-600">
            <Mail class="w-4 h-4 text-gray-400" />
            {guest.email}
          </div>
        {/if}
      </div>

      <!-- Details Grid -->
      <div class="grid grid-cols-2 gap-4">
        <div class="bg-gray-50 rounded-xl p-4">
          <div class="text-xs text-gray-500 font-medium mb-1">Table</div>
          <div class="text-2xl font-bold text-gray-900">{guest.tableId ?? '—'}</div>
        </div>
        <div class="bg-gray-50 rounded-xl p-4">
          <div class="text-xs text-gray-500 font-medium mb-1">Seat{guest.pax > 1 ? 's' : ''}</div>
          <div class="text-2xl font-bold text-gray-900">
            {#if guest.seatNumber !== null}
              {guest.pax > 1 ? `${guest.seatNumber}–${guest.seatNumber + guest.pax - 1}` : guest.seatNumber}
            {:else}
              —
            {/if}
          </div>
        </div>
        <div class="bg-gray-50 rounded-xl p-4">
          <div class="text-xs text-gray-500 font-medium mb-1">Party Size</div>
          <div class="text-2xl font-bold text-gray-900">{guest.pax}</div>
        </div>
        <div class="bg-gray-50 rounded-xl p-4">
          <div class="text-xs text-gray-500 font-medium mb-1">Checked In</div>
          <div class="text-2xl font-bold {guest.checkedIn ? 'text-emerald-600' : 'text-gray-400'}">
            {guest.checkedIn ? '✓' : '—'}
          </div>
        </div>
      </div>

      {#if guest.isVip}
        <div class="bg-gold-50 border border-gold/30 rounded-xl p-4 text-center">
          <span class="text-gold-dark font-bold">⭐ VIP Guest</span>
        </div>
      {/if}

      {#if guest.dietaryRequirements.length > 0}
        <div>
          <div class="flex items-center gap-2 text-sm font-semibold text-gray-700 mb-2">
            <Utensils class="w-4 h-4" />
            Dietary Requirements
          </div>
          <div class="flex flex-wrap gap-2">
            {#each guest.dietaryRequirements as req}
              <span class="px-3 py-1 bg-amber-50 text-amber-700 rounded-full text-xs font-medium border border-amber-200">
                {req}
              </span>
            {/each}
          </div>
        </div>
      {/if}

      {#if guest.notes}
        <div>
          <div class="flex items-center gap-2 text-sm font-semibold text-gray-700 mb-2">
            <StickyNote class="w-4 h-4" />
            Notes
          </div>
          <p class="text-sm text-gray-600 bg-gray-50 rounded-xl p-4">guest.notes}</p>
        </div>
      {/if}

      {#if guest.angbaoAmount || guest.giftItem}
        <div>
          <div class="text-sm font-semibold text-gray-700 mb-2">Gift Details</div>
          <div class="bg-gold-50 border border-gold/20 rounded-xl p-4 space-y-2">
            {#if guest.angbaoAmount}
              <div class="flex items-center gap-2 text-sm">
                <Banknote class="w-4 h-4 text-emerald-600" />
                <span class="text-gray-600">Angbao:</span>
                <span class="font-bold text-emerald-700">RM {guest.angbaoAmount}</span>
              </div>
            {/if}
            {#if guest.giftItem}
              <div class="flex items-center gap-2 text-sm">
                <Gift class="w-4 h-4 text-gold" />
                <span class="text-gray-600">Gift:</span>
                <span class="font-bold text-gold-dark">{guest.giftItem}</span>
              </div>
            {/if}
          </div>
        </div>
      {/if}
    </div>
  </div>
</div>
