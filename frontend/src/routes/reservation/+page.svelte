<script lang="ts">
  import { addToast } from '$lib/stores';
  import { cn } from '$lib/utils';
  import { z } from 'zod';
  import { CheckCircle2, AlertCircle } from 'lucide-svelte';
  import { tables, getSeatGuest, getTableOccupancy, guests, addGuest } from '$lib/mock/data';
  import { getInitials } from '$lib/utils';

  const schema = z.object({
    name: z.string().min(1, 'Name is required'),
    phone: z.string().optional().refine(val => !val || /^\+?[\d\s\-()]+$/.test(val), 'Invalid phone number'),
    email: z.string().email('Invalid email').optional().or(z.literal('')),
    pax: z.number().min(1, 'At least 1 guest').max(10, 'Max 10 guests'),
    rsvp: z.enum(['confirmed', 'pending', 'declined']),
    tableId: z.number().nullable().optional(),
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

  // Seat picker state
  const HOVERED_TABLE_RADIUS = 30;
  const HOVERED_SEAT_RADIUS = 10;
  const HOVERED_ORBIT = HOVERED_TABLE_RADIUS + HOVERED_SEAT_RADIUS + 8;
  const HOVERED_SVG = (HOVERED_ORBIT + HOVERED_SEAT_RADIUS + 4) * 2;
  const HOVERED_CENTER = HOVERED_SVG / 2;

  const selectedTableDef = $derived(form.tableId ? tables.find(t => t.id === form.tableId) : null);
  const selectedTableOccupancy = $derived(form.tableId ? getTableOccupancy(form.tableId) : null);
  const pickerRingR = $derived(HOVERED_TABLE_RADIUS + 3);
  const pickerRingCircum = $derived(2 * Math.PI * pickerRingR);
  const pickerOccPct = $derived(selectedTableOccupancy ? selectedTableOccupancy.occupied / selectedTableOccupancy.capacity : 0);

  function seatPosInPicker(i: number, capacity: number) {
    const angle = (2 * Math.PI * i) / capacity - Math.PI / 2;
    return {
      x: HOVERED_CENTER + Math.cos(angle) * HOVERED_ORBIT,
      y: HOVERED_CENTER + Math.sin(angle) * HOVERED_ORBIT,
    };
  }

  function isSeatAvailable(seatNum: number): boolean {
    if (!form.tableId) return false;
    return getSeatGuest(form.tableId, seatNum) === undefined;
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
  function onTableChange(tableId: number | null) {
    form.tableId = tableId;
    form.seatNumbers = [];
  }

  function onPaxChange(pax: number) {
    form.pax = pax;
    // Trim selected seats if pax decreased
    if (form.seatNumbers && form.seatNumbers.length > pax) {
      form.seatNumbers = form.seatNumbers.slice(0, pax);
    }
  }

  const remainingSeats = $derived(form.pax - (form.seatNumbers?.length ?? 0));

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
    await new Promise(r => setTimeout(r, 800));

    // Assign seats: first selected seat = seatNumber, pax = seatNumbers.length
    const firstSeat = form.seatNumbers?.[0] ?? null;
    const assignedPax = form.seatNumbers?.length ?? form.pax;

    addGuest({
      name: form.name,
      phone: form.phone ?? '',
      email: form.email || undefined,
      pax: assignedPax,
      rsvp: form.rsvp,
      tableId: form.tableId,
      seatNumber: firstSeat,
      checkedIn: false,
      notes: form.notes ?? '',
      dietaryRequirements: form.dietary ?? [],
      isVip: form.isVip ?? false,
    });

    addToast(`Reservation for ${form.name} saved successfully`, 'success');
    form = { name: '', phone: '', email: '', pax: 1, rsvp: 'pending', tableId: null, seatNumbers: [], dietary: [], notes: '', isVip: false };
    isSubmitting = false;
  }
</script>

<svelte:head><title>New Reservation – WeddingDB</title></svelte:head>

<div class="p-4 sm:p-7 max-w-2xl mx-auto">
  <div class="mb-8">
    <h1 class="text-xl font-bold text-gray-900">New Reservation</h1>
    <p class="text-sm text-gray-500 mt-0.5">Add a new guest reservation with dietary preferences</p>
  </div>

  <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-6">
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
        onchange={(e) => onTableChange(e.currentTarget.value ? Number(e.currentTarget.value) : null)}
        class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all"
      >
        <option value="">No table (unassigned)</option>
        {#each tables as t}
          {@const occ = getTableOccupancy(t.id)}
          <option value={t.id} selected={form.tableId === t.id}>
            Table {t.id} {t.isVip ? '★' : ''} – {occ.occupied}/{occ.capacity} seats
          </option>
        {/each}
      </select>
    </div>

    <!-- Seat Picker -->
    {#if selectedTableDef && selectedTableOccupancy}
      <div>
        <label class="block text-sm font-semibold text-gray-700 mb-1.5">
          Select Seats
          {#if remainingSeats > 0}
            <span class="text-gray-400 font-normal">– {remainingSeats} of {form.pax} remaining</span>
          {:else}
            <span class="text-emerald-600 font-normal">– all {form.pax} seats selected</span>
          {/if}
        </label>

        {#if errors.seatNumbers}
          <p class="mb-2 text-xs text-red flex items-center gap-1"><AlertCircle class="w-3 h-3" />{errors.seatNumbers}</p>
        {/if}

        <!-- Mini seat map -->
        <div class="bg-gray-50 rounded-2xl p-4 border border-gray-100">
          <svg width={HOVERED_SVG} height={HOVERED_SVG} viewBox="0 0 {HOVERED_SVG} {HOVERED_SVG}" class="mx-auto">
            <!-- Occupancy ring -->
            <circle cx={HOVERED_CENTER} cy={HOVERED_CENTER} r={pickerRingR} fill="none" stroke="#E5E7EB" stroke-width="2.5" />
            <circle cx={HOVERED_CENTER} cy={HOVERED_CENTER} r={pickerRingR} fill="none"
              stroke={pickerOccPct >= 0.9 ? '#A11217' : pickerOccPct >= 0.6 ? '#D4AF37' : '#059669'}
              stroke-width="2.5" stroke-dasharray={pickerRingCircum} stroke-dashoffset={pickerRingCircum * (1 - pickerOccPct)}
              stroke-linecap="round" style="transform: rotate(-90deg); transform-origin: center;" />

            <!-- Table circle -->
            <circle cx={HOVERED_CENTER} cy={HOVERED_CENTER} r={HOVERED_TABLE_RADIUS} fill="white" stroke="#E5E7EB" stroke-width="2" />
            <text x={HOVERED_CENTER} y={HOVERED_CENTER - 2} text-anchor="middle" class="fill-gray-800 font-extrabold" font-size="14">{selectedTableDef.id}</text>
            <text x={HOVERED_CENTER} y={HOVERED_CENTER + 10} text-anchor="middle" class="fill-gray-400" font-size="8">
              {selectedTableOccupancy.occupied}/{selectedTableOccupancy.capacity}
            </text>

            <!-- Seats -->
            {#each Array(selectedTableDef.capacity) as _, i}
              {@const seatNum = i + 1}
              {@const pos = seatPosInPicker(i, selectedTableDef.capacity)}
              {@const occupant = getSeatGuest(selectedTableDef.id, seatNum)}
              {@const available = !occupant}
              {@const selected = isSeatSelected(seatNum)}
              <g
                role="button"
                tabindex="-1"
                class={cn("cursor-pointer", !available && !selected && "cursor-not-allowed")}
                onclick={() => toggleSeat(seatNum)}
              >
                <circle
                  cx={pos.x} cy={pos.y} r={HOVERED_SEAT_RADIUS}
                  fill={selected ? '#D4AF37' : occupant ? (occupant.checkedIn ? '#ECFDF5' : '#FDEAEA') : '#F3F4F6'}
                  stroke={selected ? '#B8941F' : occupant ? (occupant.checkedIn ? '#059669' : '#A11217') : '#E5E7EB'}
                  stroke-width={selected ? 2.5 : 1.5}
                  class={cn("transition-all duration-150", available || selected ? "hover:scale-125" : "opacity-50")}
                />
                {#if selected}
                  <text x={pos.x} y={pos.y + 3} text-anchor="middle" fill="white" font-size="6" font-weight="800">✓</text>
                {:else if occupant}
                  <text x={pos.x} y={pos.y + 3} text-anchor="middle"
                    fill={occupant.checkedIn ? '#059669' : '#A11217'} font-size="6" font-weight="700"
                  >{getInitials(occupant.name)}</text>
                {:else}
                  <text x={pos.x} y={pos.y + 3} text-anchor="middle" fill="#9CA3AF" font-size="6">{seatNum}</text>
                {/if}
              </g>
            {/each}
          </svg>

          <!-- Legend -->
          <div class="flex justify-center gap-4 mt-3 text-xs text-gray-500">
            <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-gold border-2 border-gold/70 inline-block"></span> Selected</span>
            <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-red-50 border border-red inline-block"></span> Occupied</span>
            <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-gray-100 border border-gray-300 inline-block"></span> Available</span>
          </div>
        </div>
      </div>
    {/if}

    <!-- RSVP -->
    <div>
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
        class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all resize-none"
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
      <button type="button" class="px-6 py-3 border border-gray-200 bg-white rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
        Cancel
      </button>
    </div>
  </form>
</div>
