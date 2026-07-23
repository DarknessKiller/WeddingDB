<script lang="ts">
  import { DEFAULT_TABLES } from '$lib/constants';
  import { getTableOccupancy } from '$lib/mock/data';
  import { goto } from '$app/navigation';
  import { cn } from '$lib/utils';
  import { Star, Users } from 'lucide-svelte';

  const RING_R = 24;
  const RING_CIRCUM = 2 * Math.PI * RING_R;
</script>

<svelte:head><title>Tables – WeddingDB</title></svelte:head>

<div class="p-4 sm:p-7 max-w-[1400px]">
  <div class="flex items-center justify-between mb-6">
    <div>
      <h1 class="text-xl font-bold text-gray-900">Banquet Tables</h1>
      <p class="text-sm text-gray-500 mt-0.5">Overview of all {DEFAULT_TABLES.length} tables</p>
    </div>
    <button class="px-4 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center gap-2">
      <Users class="w-4 h-4" /> Manage Seating
    </button>
  </div>

  <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
    {#each DEFAULT_TABLES as table (table.id)}
      {@const occ = getTableOccupancy(table.id)}
      <button
        onclick={() => goto('/seating')}
        class={cn(
          "bg-white border rounded-2xl p-5 flex flex-col items-center gap-3 transition-all hover:shadow-lg hover:-translate-y-0.5 cursor-pointer group",
          table.isVip ? "border-gold-200 hover:border-gold" : "border-gray-200 hover:border-gold/30"
        )}
      >
        <!-- Occupancy Ring -->
        <div class="relative">
          <svg width="56" height="56" viewBox="0 0 56 56">
            <circle cx="28" cy="28" r={RING_R} fill="none" stroke="#F3F4F6" stroke-width="5" />
            <circle
              cx="28" cy="28" r={RING_R}
              fill="none"
              stroke={occ.percentage >= 90 ? '#A11217' : occ.percentage >= 60 ? '#D4AF37' : '#059669'}
              stroke-width="5"
              stroke-dasharray={RING_CIRCUM}
              stroke-dashoffset={RING_CIRCUM * (1 - occ.percentage / 100)}
              stroke-linecap="round"
              class="transition-all duration-700"
              style="transform: rotate(-90deg); transform-origin: center;"
            />
            <text x="28" y="28" text-anchor="middle" dominant-baseline="central"
              class="font-bold text-gray-800 group-hover:text-red transition-colors"
              font-size="15"
            >{table.id}</text>
          </svg>
        </div>

        <div class="text-center">
          <div class="font-semibold text-sm text-gray-800">Table {table.id}</div>
          <div class="text-xs text-gray-500">{occ.occupied}/{table.capacity} seats</div>
        </div>

        {#if table.isVip}
          <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-gold-50 text-gold border border-gold-200 rounded-full text-[11px] font-bold">
            <Star class="w-3 h-3 fill-gold" /> VIP
          </span>
        {/if}
      </button>
    {/each}
  </div>
</div>
