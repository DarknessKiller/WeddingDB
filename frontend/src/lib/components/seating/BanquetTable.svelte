<script lang="ts">
  import type { BanquetTable, Guest, Seat } from '$lib/types';
  import { getInitials, cn } from '$lib/utils';

  let {
    table,
    guests = [],
    isSelected = false,
    isHighlighted = false,
    selectedTableId = null,
    dark = false,
    hallScale = 1,
    onTableClick,
    onSeatClick,
    hoveredSeat = $bindable(null),
  }: {
    table: BanquetTable;
    guests?: Guest[];
    isSelected?: boolean;
    isHighlighted?: boolean;
    selectedTableId?: string | null;
    dark?: boolean;
    hallScale?: number;
    onTableClick?: () => void;
    onSeatClick?: (seatNum: number, guest: Guest | null) => void;
    hoveredSeat?: { seatNum: number; guest: Guest | null; x: number; y: number } | null;
  } = $props();

  const occupied = $derived(guests.reduce((sum, g) => sum + g.pax, 0));
  const occupancyPct = $derived(occupied / table.capacity);

  const TABLE_RADIUS = 36;
  const SEAT_RADIUS = 12;
  const ORBIT_RADIUS = TABLE_RADIUS + SEAT_RADIUS + 10;
  const SVG_SIZE = (ORBIT_RADIUS + SEAT_RADIUS + 4) * 2;
  const CENTER = SVG_SIZE / 2;

  const RING_R = TABLE_RADIUS + 4;
  const RING_CIRCUM = 2 * Math.PI * RING_R;
  const RING_OFFSET = $derived(RING_CIRCUM * (1 - occupancyPct));

  const tableTransform = $derived(`translate(-50%, -50%) scale(${hallScale})`);
  const scaledSvgSize = $derived(SVG_SIZE * hallScale);

  function seatPos(i: number) {
    const angle = (2 * Math.PI * i) / table.capacity - Math.PI / 2;
    return {
      x: CENTER + Math.cos(angle) * ORBIT_RADIUS,
      y: CENTER + Math.sin(angle) * ORBIT_RADIUS,
    };
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class={cn(
    'absolute cursor-pointer transition-all duration-300 ease-out',
    isSelected ? 'z-20' : 'z-10',
    selectedTableId !== null && !isSelected && !isHighlighted ? 'opacity-30 grayscale blur-[0.3px]' : '',
    isHighlighted ? 'z-20' : ''
  )}
  style="left: {table.x}%; top: {table.y}%; transform: {tableTransform};"
  onclick={() => onTableClick?.()}
  role="button"
  tabindex="0"
  aria-label="Table {table.id}, {occupied} of {table.capacity} occupied"
>
  <svg width={SVG_SIZE} height={SVG_SIZE} viewBox="0 0 {SVG_SIZE} {SVG_SIZE}">
    <circle
      cx={CENTER} cy={CENTER} r={RING_R}
      fill="none" stroke={dark ? '#374151' : '#F3F4F6'} stroke-width="3"
    />
    <circle
      cx={ CENTER} cy={CENTER} r={RING_R}
      fill="none"
      stroke={occupancyPct >= 0.9 ? '#A11217' : occupancyPct >= 0.6 ? '#D4AF37' : '#059669'}
      stroke-width="3"
      stroke-dasharray={RING_CIRCUM}
      stroke-dashoffset={RING_OFFSET}
      stroke-linecap="round"
      class="transition-all duration-700"
      style="transform: rotate(-90deg); transform-origin: center;"
    />

    <circle
      cx={CENTER} cy={CENTER} r={TABLE_RADIUS}
      fill="url(#tableGrad-{table.id})"
      stroke={isSelected || isHighlighted ? '#D4AF37' : dark ? '#4B5563' : '#E5E7EB'}
      stroke-width={isSelected || isHighlighted ? 3 : 2}
      class="transition-all duration-200"
    />

    <defs>
      <radialGradient id="tableGrad-{table.id}" cx="40%" cy="35%">
        <stop offset="0%" stop-color={dark ? '#374151' : '#FFFFFF'} />
        <stop offset="100%" stop-color={dark ? '#1F2937' : '#F9FAFB'} />
      </radialGradient>
    </defs>

    <text x={CENTER} y={CENTER - 4} text-anchor="middle" class="{dark ? 'fill-gray-200' : 'fill-gray-800'} font-extrabold" font-size="16">{table.name || table.id}</text>
    <text x={CENTER} y={CENTER + 10} text-anchor="middle" class="{dark ? 'fill-gray-500' : 'fill-gray-400'}" font-size="9">{occupied}/{table.capacity}</text>

    {#if table.isVip}
      <circle cx={SVG_SIZE - 6} cy={6} r={9} fill="#D4AF37" stroke={dark ? '#1F2937' : 'white'} stroke-width="2" />
      <text x={SVG_SIZE - 6} y={10} text-anchor="middle" fill="white" font-size="8" font-weight="800">★</text>
    {/if}

    {#each Array(table.capacity) as _, i}
      {@const pos = seatPos(i)}
      {@const guest = guests.find(g => g.seatNumber !== null && (i + 1) >= g.seatNumber && (i + 1) < g.seatNumber + g.pax) ?? null}
      <g
        role="button"
        tabindex="-1"
        aria-label="Seat {i + 1}{guest ? `, ${guest.name}` : ', empty'}"
        onmouseenter={(e) => {
          const rect = (e.currentTarget as SVGElement).getBoundingClientRect();
          hoveredSeat = { seatNum: i + 1, guest, x: rect.left + rect.width / 2, y: rect.top };
        }}
        onmouseleave={() => { hoveredSeat = null; }}
        onclick={(e) => { e.stopPropagation(); onSeatClick?.(i + 1, guest ?? null); }}
        class="cursor-pointer"
      >
        <circle cx={pos.x} cy={pos.y} r={SEAT_RADIUS + 4} fill="transparent" />
        <circle
          cx={pos.x} cy={pos.y} r={SEAT_RADIUS}
          fill={guest ? (guest.checkedIn ? (dark ? '#064E3B' : '#ECFDF5') : guest.isVip ? (dark ? '#78350F' : '#FDF8E8') : (dark ? '#7F1D1D' : '#FDEAEA')) : (dark ? '#374151' : '#F3F4F6')}
          stroke={guest ? (guest.checkedIn ? '#059669' : guest.isVip ? '#D4AF37' : '#A11217') : dark ? '#4B5563' : '#E5E7EB'}
          stroke-width="2"
          class="transition-all duration-150 group-hover:stroke-[3]"
        />
        {#if guest}
          <text x={pos.x} y={pos.y + 3} text-anchor="middle"
            fill={guest.checkedIn ? (dark ? '#34D399' : '#059669') : guest.isVip ? (dark ? '#FCD34D' : '#B8941F') : (dark ? '#F87171' : '#A11217')}
            font-size="7" font-weight="700"
          >{getInitials(guest.name)}</text>
        {:else}
          <text x={pos.x} y={pos.y + 3} text-anchor="middle" fill={dark ? '#6B7280' : '#9CA3AF'} font-size="7">{i + 1}</text>
        {/if}
      </g>
    {/each}
  </svg>
</div>
