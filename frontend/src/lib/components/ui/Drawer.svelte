<script lang="ts">
  import type { Guest, RSVPStatus } from '$lib/types';
  import { X, Phone, Mail, Utensils, StickyNote, Banknote, Gift, Pencil, Check } from 'lucide-svelte';
  import Badge from './Badge.svelte';
  import { getInitials, cn } from '$lib/utils';
  import { addToast } from '$lib/stores';
  import { weddingId } from '$lib/stores/weddingId';
  import { updateGuest } from '$lib/api/guests';
  import { get } from 'svelte/store';
  let { guest, onClose, startEditing = false }: { guest: Guest; onClose: () => void; startEditing?: boolean } = $props();

  let editing = $state(false);
  let saving = $state(false);

  // ponytail: effect watches startEditing prop, syncs editing state
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

  // ponytail: sync form when guest prop changes
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
      // update parent guest via props mutation
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
<div class="fixed inset-0 z-50 flex justify-end" onclick={onClose}>
  <div class="absolute inset-0 bg-black/30 backdrop-blur-sm"></div>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="relative w-[400px] max-w-full bg-white shadow-2xl overflow-y-auto"
    onclick={(e) => e.stopPropagation()}
  >
    <div class="sticky top-0 bg-white z-10 px-6 py-5 border-b border-gray-100 flex items-center justify-between">
      <h2 class="text-lg font-bold text-gray-900">{editing ? 'Edit Guest' : 'Guest Details'}</h2>
      <div class="flex items-center gap-2">
        {#if editing}
          <button onclick={cancel} class="p-2 rounded-lg hover:bg-gray-100 text-gray-500 text-sm">Cancel</button>
          <button onclick={save} disabled={saving}
            class="flex items-center gap-1.5 px-3 py-1.5 bg-red text-white rounded-lg text-sm font-semibold hover:bg-red-light disabled:opacity-50 transition-colors">
            <Check class="w-4 h-4" /> {saving ? 'Saving...' : 'Save'}
          </button>
        {:else}
          <button onclick={() => editing = true} class="p-2 rounded-lg hover:bg-gray-100 text-gray-500" aria-label="Edit">
            <Pencil class="w-5 h-5" />
          </button>
          <button onclick={onClose} class="p-2 rounded-lg hover:bg-gray-100 text-gray-500" aria-label="Close">
            <X class="w-5 h-5" />
          </button>
        {/if}
      </div>
    </div>

    <div class="p-6 space-y-6">
      {#if editing}
        <!-- Edit Form -->
        <div class="space-y-4">
          <div>
            <label class="text-xs font-semibold text-gray-500 mb-1 block">Name</label>
            <input bind:value={form.name} class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none" />
          </div>
          <div>
            <label class="text-xs font-semibold text-gray-500 mb-1 block">Phone</label>
            <input bind:value={form.phone} class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none" />
          </div>
          <div>
            <label class="text-xs font-semibold text-gray-500 mb-1 block">Email</label>
            <input bind:value={form.email} class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none" />
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="text-xs font-semibold text-gray-500 mb-1 block">Pax</label>
              <input type="number" min="1" bind:value={form.pax} class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none" />
            </div>
            <div>
              <label class="text-xs font-semibold text-gray-500 mb-1 block">RSVP</label>
              <select bind:value={form.rsvp} class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm focus:border-gold outline-none bg-white">
                <option value="confirmed">Confirmed</option>
                <option value="pending">Pending</option>
                <option value="declined">Declined</option>
                <option value="no_response">No Response</option>
              </select>
            </div>
          </div>
          <div>
            <label class="flex items-center gap-2 text-sm text-gray-700">
              <input type="checkbox" bind:checked={form.isVip} class="rounded" /> VIP Guest
            </label>
          </div>
          <div>
            <label class="text-xs font-semibold text-gray-500 mb-1 block">Notes</label>
            <textarea bind:value={form.notes} rows="2" class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none resize-none"></textarea>
          </div>
          <div>
            <label class="text-xs font-semibold text-gray-500 mb-1 block">Dietary</label>
            <div class="flex flex-wrap gap-2">
              {#each ['Halal', 'Vegetarian', 'Vegan', 'Gluten-Free', 'Nut-Free', 'No Seafood'] as opt}
                <button type="button"
                  onclick={() => { form.dietary = form.dietary.includes(opt) ? form.dietary.filter(d => d !== opt) : [...form.dietary, opt]; }}
                  class={cn(
                    "px-3 py-1.5 rounded-lg text-xs font-medium border transition-all",
                    form.dietary.includes(opt)
                      ? "bg-gold-50 border-gold text-gold"
                      : "bg-white border-gray-200 text-gray-600 hover:border-gray-300"
                  )}
                >{opt}</button>
              {/each}
            </div>
          </div>
          <div>
            <label class="text-xs font-semibold text-gray-500 mb-1 block">Angbao Amount (RM)</label>
            <input type="number" min="0" bind:value={form.angbaoAmt} placeholder="0" class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none" />
          </div>
          <div>
            <label class="text-xs font-semibold text-gray-500 mb-1 block">Gift Item</label>
            <input bind:value={form.giftItem} placeholder="e.g. Kitchenware set" class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none" />
          </div>
        </div>
      {:else}
        <!-- View Mode -->
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
            <p class="text-sm text-gray-600 bg-gray-50 rounded-xl p-4">{guest.notes}</p>
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
      {/if}
    </div>
  </div>
</div>
