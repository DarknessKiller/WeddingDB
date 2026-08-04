<script lang="ts">
  import { addToast } from '$lib/stores';
  import { weddingTitle } from '$lib/stores/weddingTitle';
  import { goto } from '$app/navigation';
  import { cn } from '$lib/utils';
  import { z } from 'zod';
  import { CheckCircle2, AlertCircle } from 'lucide-svelte';
  import { fetchAllGuests, createGuest, type GuestResponse } from '$lib/api/guests';
  import { listTables } from '$lib/api/tables';
  import { getLayout } from '$lib/api/layout';
  import { weddingId } from '$lib/stores/weddingId';
  import { get } from 'svelte/store';
  import { onMount } from 'svelte';
  import type { BanquetTable, Guest, HallElement, RSVPStatus } from '$lib/types';
  import HallMap from '$lib/components/seating/HallMap.svelte';

  let apiTables = $state<BanquetTable[]>([]);
  let apiGuests = $state<Guest[]>([]);
  let elements = $state<HallElement[]>([]);
  let hallWidth = $state(860);
  let hallHeight = $state(1000);
  let dataLoaded = $state(false);

  function mapGuest(r: GuestResponse): Guest {
    return {
      id: r.id,
      name: r.name,
      phone: r.phone,
      email: r.email,
      rsvp: (r.rsvp as RSVPStatus) ?? 'no_response',
      pax: r.pax,
      tableId: r.tableId ?? null,
      seatNumber: r.seatNum,
      checkedIn: r.checkedInAt !== null,
      checkedInAt: r.checkedInAt ? new Date(r.checkedInAt) : undefined,
      notes: r.notes,
      dietaryRequirements: r.dietary ?? [],
      isVip: r.isVip,
      angbaoAmount: r.angbaoAmt ?? undefined,
      giftItem: r.giftItem ?? undefined,
      createdAt: new Date(r.createdAt),
    };
  }

  onMount(async () => {
    const wid = get(weddingId);
    try {
      const [tablesRes, guestRows, layout] = await Promise.all([listTables(wid), fetchAllGuests(wid), getLayout(wid)]);
      apiTables = tablesRes;
      apiGuests = guestRows.map(mapGuest);
      elements = layout.elements;
      hallWidth = layout.hallWidth;
      hallHeight = layout.hallHeight;
      dataLoaded = true;
    } catch (e) {
      addToast('Failed to load data', 'error');
    }
  });

  let tableGuests = $derived.by(() => {
    const obj: Record<string, Guest[]> = {};
    for (const g of apiGuests) {
      if (g.tableId === null) continue;
      const key = String(g.tableId);
      if (!obj[key]) obj[key] = [];
      obj[key].push(g);
    }
    return obj;
  });

  function findClosestEmptySeats(tableId: string, count: number): number[] {
    const table = apiTables.find(t => t.id === tableId);
    if (!table) return [];
    const occupied = new Set(
      apiGuests
        .filter(g => g.tableId === tableId && g.seatNumber !== null)
        .flatMap(g => Array.from({ length: g.pax }, (_, i) => g.seatNumber! + i))
    );
    const seats: number[] = [];
    for (let i = 1; i <= table.capacity && seats.length < count; i++) {
      if (!occupied.has(i)) seats.push(i);
    }
    return seats;
  }

  function handleSeatClick(tableId: string, seatNum: number, guest: Guest | null) {
    if (!guest && form.tableId === tableId) {
      toggleSeat(seatNum);
    } else if (!guest) {
      form.tableId = tableId;
      form.seatNumbers = [seatNum];
    }
  }

  function handleTableClick(id: string) {
    form.tableId = id;
    form.seatNumbers = findClosestEmptySeats(id, form.pax);
  }

  const schema = z.object({
    name: z.string().min(1, 'Name is required'),
    phone: z.string().optional().refine(val => !val || /^\+?[\d\s\-()]+$/.test(val), 'Invalid phone number'),
    email: z.string().email('Invalid email').optional().or(z.literal('')),
    pax: z.number().min(1, 'At least 1 guest').max(10, 'Max 10 guests'),
    rsvp: z.enum(['confirmed', 'pending', 'declined']),
    tableId: z.string().nullable().optional(),
    seatNumbers: z.array(z.number()).optional(),
    dietary: z.array(z.string()).optional(),
    notes: z.string().optional(),
    isVip: z.boolean().optional(),
  });

  type FormData = z.infer<typeof schema>;

  let form = $state<FormData>({
    name: '',
    phone: '',
    email: '',
    pax: 1,
    rsvp: 'pending',
    tableId: null,
    seatNumbers: [],
    dietary: [],
    notes: '',
    isVip: false,
  });

  let errors = $state<Record<string, string>>({});
  let isSubmitting = $state(false);

  function getSeatGuestLocal(tableId: string, seatNum: number): Guest | undefined {
    return apiGuests.find(g =>
      g.tableId === tableId &&
      g.seatNumber !== null &&
      seatNum >= g.seatNumber &&
      seatNum < g.seatNumber + g.pax
    );
  }

  function isSeatAvailable(seatNum: number): boolean {
    if (!form.tableId) return false;
    return getSeatGuestLocal(form.tableId, seatNum) === undefined;
  }

  function isSeatSelected(seatNum: number): boolean {
    return form.seatNumbers?.includes(seatNum) ?? false;
  }

  function toggleSeat(seatNum: number) {
    if (!isSeatAvailable(seatNum) && !isSeatSelected(seatNum)) return;
    const current = form.seatNumbers ?? [];
    if (isSeatSelected(seatNum)) {
      form.seatNumbers = current.filter(s => s !== seatNum);
    } else {
      if (current.length >= form.pax) return; // max pax seats
      form.seatNumbers = [...current, seatNum].sort((a, b) => a - b);
    }
  }

  // Reset seat selection when table or pax changes
  function onTableChange(tableId: string | null) {
    form.tableId = tableId;
    form.seatNumbers = tableId ? findClosestEmptySeats(tableId, form.pax) : [];
  }

  function onPaxChange(pax: number) {
    form.pax = pax;
    if (form.tableId) {
      // Re-select closest empty seats for new pax
      form.seatNumbers = findClosestEmptySeats(form.tableId, pax);
    } else if (form.seatNumbers && form.seatNumbers.length > pax) {
      form.seatNumbers = form.seatNumbers.slice(0, pax);
    }
  }

  const DIETARY_OPTIONS = [
    { id: 'halal', label: 'Halal' },
    { id: 'vegetarian', label: 'Vegetarian' },
    { id: 'vegan', label: 'Vegan' },
    { id: 'gluten_free', label: 'Gluten-Free' },
    { id: 'nut_free', label: 'Nut-Free' },
    { id: 'seafood_free', label: 'No Seafood' },
  ];

  function validate() {
    const result = schema.safeParse(form);
    if (result.success) {
      errors = {};
      return true;
    }
    const fieldErrors: Record<string, string> = {};
    for (const issue of result.error.issues) {
      const field = issue.path[0] as string;
      if (!fieldErrors[field]) fieldErrors[field] = issue.message;
    }
    // Custom validation
    if (form.tableId && (!form.seatNumbers || form.seatNumbers.length === 0)) {
      fieldErrors.seatNumbers = 'Select at least one seat';
    }
    if (form.tableId && form.seatNumbers && form.seatNumbers.length < form.pax) {
      fieldErrors.seatNumbers = `Select ${form.pax} seat(s) for ${form.pax} guest(s)`;
    }
    errors = fieldErrors;
    return false;
  }

  function toggleDietary(id: string) {
    form.dietary = form.dietary?.includes(id)
      ? form.dietary.filter(d => d !== id)
      : [...(form.dietary ?? []), id];
  }

  async function handleSubmit() {
    if (!validate()) return;
    isSubmitting = true;
    try {
      const wid = get(weddingId);
      const firstSeat = form.seatNumbers?.[0] ?? null;
      const assignedPax = form.seatNumbers?.length ?? form.pax;
      
      await createGuest(wid, {
        name: form.name,
        phone: form.phone ?? '',
        email: form.email || undefined,
        pax: assignedPax,
        rsvp: form.rsvp,
        tableId: form.tableId,
        seatNum: firstSeat,
        notes: form.notes ?? '',
        dietary: form.dietary ?? [],
        isVip: form.isVip ?? false,
      });
      
      addToast(`Reservation for ${form.name} saved successfully`, 'success');
      goto(`/${wid}/guests`);
    } catch (e) {
      addToast('Failed to save reservation', 'error');
    } finally {
      isSubmitting = false;
    }
  }
</script>

  <svelte:head> <title>{$weddingTitle ? `${$weddingTitle} – Reservation` : 'Reservation – WeddingDB'}</title></svelte:head>


<div class="flex flex-col sm:flex-row h-full">
  <!-- Left: Hall Map (hidden on mobile, shown on desktop) -->
  <div class="hidden sm:flex flex-1 flex-col relative">
    {#if apiTables.length > 0}
      <HallMap
        tables={apiTables}
        {elements}
        {hallWidth}
        {hallHeight}
        tableGuests={tableGuests}
        selectedTableId={form.tableId}
        onTableClick={handleTableClick}
        onSeatClick={handleSeatClick}
      />
    {/if}
  </div>

  <!-- Right: Form -->
  <div class="flex-1 sm:w-full sm:max-w-lg bg-white/90 backdrop-blur-xl sm:border-l border-black/[0.06] overflow-y-auto">
    <div class="p-6">
      <div class="mb-6">
        <h1 class="text-xl font-bold text-gray-900">New Reservation</h1>
        <p class="text-sm text-gray-500 mt-0.5">Add a new guest reservation</p>
      </div>

      <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-5">
        <!-- Name + Phone -->
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label for="name" class="block text-sm font-semibold text-gray-700 mb-1.5">Full Name *</label>
            <input
              id="name"
              type="text"
              bind:value={form.name}
              class={cn("w-full px-4 py-2.5 border rounded-xl text-sm bg-white outline-none transition-all",
                errors.name ? "border-red focus:ring-2 focus:ring-red/15" : "border-gray-200 focus:border-gold focus:ring-2 focus:ring-gold/15"
              )}
              placeholder="Guest name"
            />
            {#if errors.name}
              <p class="mt-1 text-xs text-red flex items-center gap-1"><AlertCircle class="w-3 h-3" />{errors.name}</p>
            {/if}
          </div>
          <div>
            <label for="phone" class="block text-sm font-semibold text-gray-700 mb-1.5">Phone</label>
            <input
              id="phone"
              type="tel"
              bind:value={form.phone}
              class={cn("w-full px-4 py-2.5 border rounded-xl text-sm bg-white outline-none transition-all",
                errors.phone ? "border-red focus:ring-2 focus:ring-red/15" : "border-gray-200 focus:border-gold focus:ring-2 focus:ring-gold/15"
              )}
              placeholder="+60 12-345 6789"
            />
            {#if errors.phone}
              <p class="mt-1 text-xs text-red flex items-center gap-1"><AlertCircle class="w-3 h-3" />{errors.phone}</p>
            {/if}
          </div>
        </div>

        <!-- Email + Pax -->
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label for="email" class="block text-sm font-semibold text-gray-700 mb-1.5">Email</label>
            <input
              id="email"
              type="email"
              bind:value={form.email}
              class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all"
              placeholder="Optional"
            />
          </div>
          <div>
            <label for="pax" class="block text-sm font-semibold text-gray-700 mb-1.5">Pax *</label>
            <input
              id="pax"
              type="number"
              min="1"
              max="10"
              oninput={(e) => onPaxChange(Number(e.currentTarget.value) || 1)}
              value={form.pax}
              class={cn("w-full px-4 py-2.5 border rounded-xl text-sm bg-white outline-none transition-all",
                errors.pax ? "border-red focus:ring-2 focus:ring-red/15" : "border-gray-200 focus:border-gold focus:ring-2 focus:ring-gold/15"
              )}
            />
            {#if errors.pax}
              <p class="mt-1 text-xs text-red flex items-center gap-1"><AlertCircle class="w-3 h-3" />{errors.pax}</p>
            {/if}
          </div>
        </div>

        <!-- Table Assignment -->
        <div>
          <label for="table" class="block text-sm font-semibold text-gray-700 mb-1.5">Assign Table</label>
          <select
            id="table"
            onchange={(e) => onTableChange(e.currentTarget.value || null)}
            class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all"
          >
            <option value="">No table (unassigned)</option>
            {#each apiTables as t}
              {@const tg = tableGuests[String(t.id)] ?? []}
              {@const occ = { occupied: tg.reduce((sum, g) => sum + g.pax, 0), capacity: t.capacity }}
              <option value={t.id} selected={form.tableId === t.id}>
                {t.name || `Table ${t.id}`} {t.isVip ? '★' : ''} – {occ.occupied}/{occ.capacity} seats
              </option>
            {/each}
          </select>
        </div>

        <!-- Seat info (from map selection) -->
        {#if form.tableId && form.seatNumbers && form.seatNumbers.length > 0}
          <div class="bg-gold-50 border border-gold/30 rounded-xl p-3 text-sm">
            <span class="font-semibold text-gray-700">Selected seats:</span>
            <span class="text-gold font-medium">{form.seatNumbers.join(', ')}</span>
            <span class="text-gray-400 ml-1">({form.seatNumbers.length} of {form.pax})</span>
          </div>
        {/if}

        {#if errors.seatNumbers}
          <p class="text-xs text-red flex items-center gap-1"><AlertCircle class="w-3 h-3" />{errors.seatNumbers}</p>
        {/if}

        <!-- RSVP -->
        <div>
          <!-- svelte-ignore a11y_label_has_associated_control -->
          <label class="block text-sm font-semibold text-gray-700 mb-2">RSVP Status *</label>
          <div class="flex gap-3 flex-wrap sm:flex-nowrap">
            {#each [
              { value: 'confirmed', label: 'Confirmed', color: 'emerald' },
              { value: 'pending', label: 'Pending', color: 'gold' },
              { value: 'declined', label: 'Declined', color: 'red' },
            ] as opt}
              <button
                type="button"
                onclick={() => form.rsvp = opt.value as FormData['rsvp']}
                class={cn(
                  "flex-1 py-2.5 rounded-xl text-sm font-semibold border-2 transition-all",
                  form.rsvp === opt.value
                    ? opt.color === 'emerald' ? "bg-emerald-50 border-emerald-400 text-emerald-700" :
                      opt.color === 'gold' ? "bg-gold-50 border-gold text-gold" :
                      "bg-red-50 border-red text-red"
                    : "bg-white border-gray-200 text-gray-600 hover:border-gray-300"
                )}
              >
                {opt.label}
              </button>
            {/each}
          </div>
        </div>

        <!-- Dietary -->
        <div>
          <!-- svelte-ignore a11y_label_has_associated_control -->
          <label class="block text-sm font-semibold text-gray-700 mb-2">Dietary Restrictions</label>
          <div class="flex flex-wrap gap-2">
            {#each DIETARY_OPTIONS as opt}
              <button
                type="button"
                onclick={() => toggleDietary(opt.id)}
                class={cn(
                  "px-3.5 py-2 rounded-xl text-sm font-medium border transition-all",
                  form.dietary?.includes(opt.id)
                    ? "bg-gold-50 border-gold text-gold"
                    : "bg-white border-gray-200 text-gray-600 hover:border-gray-300"
                )}
              >
                {opt.label}
              </button>
            {/each}
          </div>
        </div>

        <!-- VIP -->
        <div class="flex items-center gap-3">
          <label class="relative inline-flex items-center cursor-pointer">
            <input type="checkbox" bind:checked={form.isVip} class="sr-only peer" />
            <div class="w-11 h-6 bg-gray-200 rounded-full peer peer-checked:bg-gold transition-colors after:content-[''] after:absolute after:top-0.5 after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:after:translate-x-full"></div>
          </label>
          <span class="text-sm font-semibold text-gray-700">VIP Guest</span>
        </div>

        <!-- Notes -->
        <div>
          <label for="notes" class="block text-sm font-semibold text-gray-700 mb-1.5">Notes</label>
          <textarea
            id="notes"
            bind:value={form.notes}
            rows="3"
            class="w-full px-4 py-3 border border-black/[0.08] rounded-xl text-sm bg-white/80 focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all resize-none min-h-[80px]"
            placeholder="Any special requirements or notes..."
          ></textarea>
        </div>

        <!-- Submit -->
        <div class="flex gap-3 pt-2">
          <button
            type="submit"
            disabled={isSubmitting}
            class="flex-1 py-3 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
          >
            {#if isSubmitting}
              <div class="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin"></div>
              Saving...
            {:else}
              <CheckCircle2 class="w-4 h-4" /> Save Reservation
            {/if}
          </button>
          <button type="button" onclick={() => goto(`/${$weddingId}/guests`)} class="px-6 py-3 border border-black/[0.06] bg-white/90 rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
            Cancel
          </button>
        </div>
      </form>
    </div>
  </div>
</div>
