<script lang="ts">
  import { onMount } from 'svelte';
  import type { BanquetTable, Guest } from '$lib/types';
  import { getInitials } from '$lib/utils';

  let {
    table,
    guests = [],
    isSelected = false,
    isHighlighted = false,
    dark = false,
    hallWidth,
    hallHeight,
    mode = 'view',
    onTableClick,
    onSeatClick,
  }: {
    table: BanquetTable;
    guests?: Guest[];
    isSelected?: boolean;
    isHighlighted?: boolean;
    dark?: boolean;
    hallWidth: number;
    hallHeight: number;
    mode?: 'view' | 'edit';
    onTableClick?: () => void;
    onSeatClick?: (seatNum: number, guest: Guest | null) => void;
  } = $props();

  const px = $derived(table.x / 100 * hallWidth);
  const py = $derived(table.y / 100 * hallHeight);

  const occupied = $derived(guests.reduce((sum, g) => sum + g.pax, 0));
  const occupancyPct = $derived(occupied / table.capacity);

  const TABLE_RADIUS = 36;
  const SEAT_RADIUS = 12;
  const ORBIT_RADIUS = TABLE_RADIUS + SEAT_RADIUS + 10;

  const RING_R = TABLE_RADIUS + 4;

  let Group: any = $state(null);
  let Circle: any = $state(null);
  let Arc: any = $state(null);
  let KText: any = $state(null);

  onMount(async () => {
    const mod = await import('svelte-konva');
    Group = mod.Group;
    Circle = mod.Circle;
    Arc = mod.Arc;
    KText = mod.Text;
  });

  function seatPos(i: number) {
    const angleDeg = (360 * i) / table.capacity - 90;
    const angleRad = (angleDeg * Math.PI) / 180;
    return {
      x: Math.cos(angleRad) * ORBIT_RADIUS,
      y: Math.sin(angleRad) * ORBIT_RADIUS,
    };
  }

  function seatColor(guest: Guest | null): { fill: string; stroke: string } {
    if (!guest) return { fill: dark ? '#374151' : '#F3F4F6', stroke: dark ? '#4B5563' : '#E5E7EB' };
    if (guest.checkedIn) return { fill: dark ? '#064E3B' : '#ECFDF5', stroke: '#059669' };
    if (guest.isVip) return { fill: dark ? '#78350F' : '#FDF8E8', stroke: '#D4AF37' };
    return { fill: dark ? '#7F1D1D' : '#FDEAEA', stroke: '#A11217' };
  }

  function seatTextColor(guest: Guest | null): string {
    if (!guest) return dark ? '#6B7280' : '#9CA3AF';
    if (guest.checkedIn) return dark ? '#34D399' : '#059669';
    if (guest.isVip) return dark ? '#FCD34D' : '#B8941F';
    return dark ? '#F87171' : '#A11217';
  }

  const ringColor = $derived(
    occupancyPct >= 0.9 ? '#A11217' : occupancyPct >= 0.6 ? '#D4AF37' : '#059669'
  );

  const strokeColor = $derived(
    isSelected || isHighlighted ? '#D4AF37' : dark ? '#4B5563' : '#E5E7EB'
  );
  const strokeW = $derived(isSelected || isHighlighted ? 3 : 2);
</script>

{#if Group && Circle && Arc && KText}
  <Group
    x={px}
    y={py}
    rotation={table.degree}
    draggable={mode === 'edit'}
    onclick={() => onTableClick?.()}
  >
    <!-- Occupancy ring background -->
    <Circle
      radius={RING_R}
      fill="none"
      stroke={dark ? '#374151' : '#F3F4F6'}
      strokeWidth={3}
    />

    <!-- Occupancy ring arc -->
    <Arc
      innerRadius={37}
      outerRadius={40}
      angle={360 * occupancyPct}
      rotation={-90}
      fill={ringColor}
    />

    <!-- Table circle -->
    <Circle
      radius={TABLE_RADIUS}
      fillRadialGradientStartPoint={{ x: 0, y: 0 }}
      fillRadialGradientEndPoint={{ x: 0, y: 0 }}
      fillRadialGradientStartRadius={0}
      fillRadialGradientEndRadius={TABLE_RADIUS}
      fillRadialGradientColorStops={[0, dark ? '#374151' : '#FFFFFF', 1, dark ? '#1F2937' : '#F3F4F6']}
      stroke={strokeColor}
      strokeWidth={strokeW}
    />

    <!-- Table name -->
    <KText
      text={table.name || table.id}
      fontSize={16}
      fontStyle="bold"
      fill={dark ? '#E5E7EB' : '#1F2937'}
      align="center"
      width={TABLE_RADIUS * 2}
      x={-TABLE_RADIUS}
      y={-8}
    />

    <!-- Occupancy count -->
    <KText
      text="{occupied}/{table.capacity}"
      fontSize={9}
      fill={dark ? '#6B7280' : '#9CA3AF'}
      align="center"
      width={TABLE_RADIUS * 2}
      x={-TABLE_RADIUS}
      y={6}
    />

    <!-- VIP badge -->
    {#if table.isVip}
      <Circle
        x={TABLE_RADIUS - 4}
        y={-TABLE_RADIUS + 4}
        radius={9}
        fill="#D4AF37"
        stroke={dark ? '#1F2937' : '#FFFFFF'}
        strokeWidth={2}
      />
      <KText
        text="★"
        fontSize={8}
        fontStyle="bold"
        fill="#FFFFFF"
        x={TABLE_RADIUS - 4 - 4}
        y={-TABLE_RADIUS + 4 - 4}
      />
    {/if}

    <!-- Seats -->
    {#each Array(table.capacity) as _, i}
      {@const pos = seatPos(i)}
      {@const guest = guests.find(g => g.seatNumber !== null && (i + 1) >= g.seatNumber && (i + 1) < g.seatNumber + g.pax) ?? null}
      {@const sc = seatColor(guest)}
      <Group
        x={pos.x}
        y={pos.y}
        onclick={(e: any) => { e.cancelBubble = true; onSeatClick?.(i + 1, guest ?? null); }}
      >
        <!-- Hit area -->
        <Circle radius={SEAT_RADIUS + 4} fill="transparent" />
        <!-- Seat circle -->
        <Circle
          radius={SEAT_RADIUS}
          fill={sc.fill}
          stroke={sc.stroke}
          strokeWidth={2}
        />
        <!-- Seat label -->
        <KText
          text={guest ? getInitials(guest.name) : String(i + 1)}
          fontSize={7}
          fontStyle={guest ? 'bold' : 'normal'}
          fill={seatTextColor(guest)}
          align="center"
          width={SEAT_RADIUS * 2}
          x={-SEAT_RADIUS}
          y={-4}
        />
      </Group>
    {/each}
  </Group>
{/if}
