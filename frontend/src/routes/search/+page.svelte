<script lang="ts">
  import { searchGuests, getGuestById, guests, getSeatGuest } from '$lib/mock/data';
  import { TABLE_DEFINITIONS } from '$lib/constants';
  import { addToast } from '$lib/stores';
  import Badge from '$lib/components/ui/Badge.svelte';
  import { getInitials, cn } from '$lib/utils';
  import { goto } from '$app/navigation';
  import { Search, CheckCircle2, UserCheck, Phone, MapPin, Gift, Banknote, X, MapPinned } from 'lucide-svelte';

  let query = $state('');
  let results = $derived(query.trim().length > 0 ? searchGuests(query) : []);

  // Check-in modal state
  let showCheckinModal = $state(false);
  let showSeatView = $state(false);
  let checkinGuest = $state<Guest | null>(null);
  let angbaoAmount = $state('');
  let giftItem = $state('');

  function openCheckinModal(guestId: string) {
    const guest = getGuestById(guestId);
    if (guest) {
      checkinGuest = guest;
      angbaoAmount = guest.angbaoAmount?.toString() ?? '';
      giftItem = guest.giftItem ?? '';
      showSeatView = false;
      showCheckinModal = true;
    }
  }

  function handleCheckIn() {
    if (!checkinGuest) return;
    const g = guests.find(g => g.id === checkinGuest!.id);
    if (g) {
      g.checkedIn = true;
      g.checkedInAt = new Date();
      if (angbaoAmount) g.angbaoAmount = parseFloat(angbaoAmount) || undefined;
      if (giftItem) g.giftItem = giftItem;
    }
    addToast(`${checkinGuest.name} checked in successfully`, 'success');
    showSeatView = true;
    results = query.trim().length > 0 ? searchGuests(query) : [];
  }

  function closeModal() {
    showCheckinModal = false;
    showSeatView = false;
    checkinGuest = null;
  }

  function viewOnMap() {
    if (!checkinGuest?.tableId) return;
    const tableId = checkinGuest.tableId;
    closeModal();
    goto(`/seating?table=${tableId}`);
  }

  function getSeatOccupants(tableId: number, capacity: number) {
    return Array.from({ length: capacity }, (_, i) => {
      const seatNum = i + 1;
      const guest = getSeatGuest(tableId, seatNum);
      return { seatNum, guest };
    });
  }
</script>

<svelte:head><title>Check In – WeddingDB</title></svelte:head>

<div class="p-4 sm:p-7 max-w-3xl mx-auto">
  <div class="text-center mb-8">
    <h1 class="text-2xl font-bold text-gray-900">Guest Search</h1>
    <p class="text-gray-500 mt-1">Find guests quickly for reception check-in</p>
  </div>

  <div class="relative mb-6">
    <Search class="absolute left-4 top-1/2 -translate-y-1/2 w-6 h-6 text-gray-400 pointer-events-none" />
    <input
      type="text"
      placeholder="Search by name or phone number..."
      bind:value={query}
      class="w-full pl-13 pr-5 py-4 border border-gray-200 rounded-2xl text-lg bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all shadow-sm"
      autofocus
    />
  </div>

  {#if results.length > 0}
    <div class="space-y-3">
      {#each results.slice(0, 10) as guest (guest.id)}
        <div class="bg-white border border-gray-200 rounded-2xl p-4 sm:p-5 flex flex-col sm:flex-row items-start sm:items-center gap-3 sm:gap-4 hover:shadow-md hover:border-gold/30 transition-all">
          <div class={cn(
            "w-12 h-12 sm:w-14 sm:h-14 rounded-full flex items-center justify-center text-lg font-bold flex-shrink-0",
            guest.checkedIn ? "bg-emerald-50 text-emerald-700 border-2 border-emerald-300" :
            guest.isVip ? "bg-gold-50 text-gold border-2 border-gold-300" :
            "bg-red-50 text-red border-2 border-red-200"
          )}>
            {getInitials(guest.name)}
          </div>
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              {#if guest.isVip}
                <span class="text-gold">★</span>
              {/if}
              <span class="font-semibold text-gray-900">{guest.name}</span>
            </div>
            <div class="flex items-center gap-2 sm:gap-4 text-sm text-gray-500 mt-0.5 flex-wrap">
              <span class="flex items-center gap-1"><Phone class="w-3.5 h-3.5" />{guest.phone}</span>
              <span class="flex items-center gap-1"><MapPin class="w-3.5 h-3.5" />Table {guest.tableId ?? '—'}</span>
              <span>{guest.pax} pax</span>
              {#if guest.checkedIn && guest.angbaoAmount}
                <span class="flex items-center gap-1 text-emerald-600"><Banknote class="w-3.5 h-3.5" />RM {guest.angbaoAmount}</span>
              {/if}
              {#if guest.checkedIn && guest.giftItem}
                <span class="flex items-center gap-1 text-gold"><Gift class="w-3.5 h-3.5" />{guest.giftItem}</span>
              {/if}
            </div>
          </div>
          <Badge status={guest.rsvp} />
          <div class="w-full sm:w-auto flex-shrink-0">
          {#if guest.checkedIn}
            <span class="inline-flex items-center gap-1.5 px-3 py-1.5 bg-emerald-50 text-emerald-700 rounded-full text-sm font-semibold border border-emerald-200">
              <CheckCircle2 class="w-4 h-4" /> Checked In
            </span>
          {:else}
            <button
              onclick={() => openCheckinModal(guest.id)}
              class="w-full sm:w-auto flex items-center justify-center gap-1.5 px-4 py-2 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors"
            >
              <UserCheck class="w-4 h-4" /> Check In
            </button>
          {/if}
          </div>
        </div>
      {/each}
    </div>
  {:else if query.trim().length > 0}
    <div class="text-center py-12 text-gray-400">
      <Search class="w-12 h-12 mx-auto mb-3 opacity-40" />
      <p class="font-medium">No guests found for "{query}"</p>
      <p class="text-sm mt-1">Try a different name or phone number</p>
    </div>
  {:else}
    <div class="text-center py-12 text-gray-400">
      <Search class="w-12 h-12 mx-auto mb-3 opacity-40" />
      <p class="font-medium">Start typing to search guests</p>
      <p class="text-sm mt-1">Supports fuzzy matching on name and phone</p>
    </div>
  {/if}
</div>

{#if showCheckinModal && checkinGuest}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="absolute inset-0 bg-black/40 backdrop-blur-sm" onclick={closeModal} role="presentation"></div>

    <div class="relative bg-white rounded-2xl shadow-2xl w-full max-w-md overflow-hidden">
      {#if showSeatView}
        <!-- Seat View after check-in -->
        <div class="flex items-center justify-between p-5 border-b border-gray-100">
          <div class="flex items-center gap-3">
            <div class="w-11 h-11 rounded-full bg-emerald-50 border-2 border-emerald-300 flex items-center justify-center">
              <CheckCircle2 class="w-5 h-5 text-emerald-600" />
            </div>
            <div>
              <h3 class="font-bold text-gray-900">{checkinGuest.name}</h3>
              <p class="text-sm text-emerald-600 font-medium">Checked In Successfully</p>
            </div>
          </div>
          <button onclick={closeModal} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
            <X class="w-4 h-4 text-gray-400" />
          </button>
        </div>

        <div class="p-5 space-y-4">
          {#if checkinGuest.tableId}
            {@const table = TABLE_DEFINITIONS.find(t => t.id === checkinGuest!.tableId)}
            <div class="text-center">
              <div class="text-sm text-gray-500 mb-1">Your Table</div>
              <div class="text-4xl font-extrabold text-red">{checkinGuest.tableId}</div>
              {#if table?.isVip}
                <span class="text-gold font-semibold text-sm">★ VIP Table</span>
              {/if}
              <div class="text-sm text-gray-500 mt-1">
                Seats {checkinGuest.seatNumber}–{(checkinGuest.seatNumber ?? 0) + checkinGuest.pax - 1}
                · {checkinGuest.pax} pax
              </div>
            </div>

            {@const seats = getSeatOccupants(checkinGuest.tableId, table?.capacity ?? 10)}
            <div class="grid grid-cols-5 gap-2">
              {#each seats as { seatNum, guest }}
                {@const isOwn = seatNum >= (checkinGuest.seatNumber ?? 0) && seatNum < (checkinGuest.seatNumber ?? 0) + checkinGuest.pax}
                <div class={cn(
                  "aspect-square rounded-lg flex items-center justify-center text-[11px] font-bold border-2 transition-colors",
                  isOwn ? "bg-red text-white border-red" :
                  guest ? "bg-gray-100 text-gray-500 border-gray-200" :
                  "bg-gray-50 text-gray-300 border-gray-100"
                )}>
                  {seatNum}
                </div>
              {/each}
            </div>

            <button
              onclick={viewOnMap}
              class="w-full py-3 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2"
            >
              <MapPinned class="w-4 h-4" /> View on Seating Map
            </button>
          {:else}
            <div class="text-center py-4 text-gray-500">
              <p class="font-medium">No table assigned yet</p>
              <p class="text-sm mt-1">Please see the reception desk for seating.</p>
            </div>
          {/if}
        </div>

        <div class="p-5 pt-0">
          <button
            onclick={closeModal}
            class="w-full py-2.5 border border-gray-200 bg-white rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors"
          >
            Close
          </button>
        </div>

      {:else}
        <!-- Check-in Form -->
        <div class="flex items-center justify-between p-5 border-b border-gray-100">
          <div class="flex items-center gap-3">
            <div class={cn(
              "w-11 h-11 rounded-full flex items-center justify-center text-sm font-bold",
              checkinGuest.isVip ? "bg-gold-50 text-gold border-2 border-gold-300" : "bg-red-50 text-red border-2 border-red-200"
            )}>
              {getInitials(checkinGuest.name)}
            </div>
            <div>
              <h3 class="font-bold text-gray-900">{checkinGuest.name}</h3>
              <p class="text-sm text-gray-500">Table {checkinGuest.tableId ?? '—'} · {checkinGuest.pax} pax</p>
            </div>
          </div>
          <button onclick={closeModal} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
            <X class="w-4 h-4 text-gray-400" />
          </button>
        </div>

        <div class="p-5 space-y-4">
          <p class="text-sm text-gray-600">Record gift details for this guest's check-in.</p>

          <div>
            <label for="angbao" class="flex items-center gap-1.5 text-sm font-semibold text-gray-700 mb-1.5">
              <Banknote class="w-4 h-4 text-emerald-600" /> Angbao Amount (RM)
            </label>
            <input
              id="angbao"
              type="number"
              min="0"
              step="10"
              bind:value={angbaoAmount}
              placeholder="e.g. 200"
              class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all"
            />
          </div>

          <div>
            <label for="gift" class="flex items-center gap-1.5 text-sm font-semibold text-gray-700 mb-1.5">
              <Gift class="w-4 h-4 text-gold" /> Gift Item
            </label>
            <input
              id="gift"
              type="text"
              bind:value={giftItem}
              placeholder="e.g. Gold bracelet, Red packet, etc."
              class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all"
            />
          </div>
        </div>

        <div class="flex gap-3 p-5 pt-0">
          <button
            onclick={handleCheckIn}
            class="flex-1 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center justify-center gap-2"
          >
            <CheckCircle2 class="w-4 h-4" /> Confirm Check-In
          </button>
          <button
            onclick={closeModal}
            class="px-5 py-2.5 border border-gray-200 bg-white rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors"
          >
            Cancel
          </button>
        </div>
      {/if}
    </div>
  </div>
{/if}
