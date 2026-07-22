<script lang="ts">
  import { getDashboardStats, getTableOccupancy, getRecentActivity } from '$lib/mock/data';
  import {
    Users, UserCheck, Clock, XCircle, CheckCircle, TrendingUp,
    ArrowUpRight, ArrowDownRight, LayoutGrid
  } from 'lucide-svelte';
  import dayjs from 'dayjs';
  import relativeTime from 'dayjs/plugin/relativeTime';

  dayjs.extend(relativeTime);

  const stats = getDashboardStats();
  const occupancy = getTableOccupancy();
  const activity = getRecentActivity();

  const statCards = [
    { label: 'Confirmed Guests', value: stats.confirmedGuests, icon: UserCheck, color: 'bg-red-50 text-red', change: '+12 this week', up: true },
    { label: 'Pending RSVP', value: stats.pendingRsvp, icon: Clock, color: 'bg-gold-50 text-gold-dark', change: '-3 this week', up: false },
    { label: 'Declined', value: stats.declined, icon: XCircle, color: 'bg-gray-100 text-gray-600', change: '', up: false },
    { label: 'Checked In', value: stats.checkedIn, icon: CheckCircle, color: 'bg-emerald-50 text-emerald-600', change: '+8 today', up: true },
    { label: 'Total Pax', value: stats.totalPax, icon: Users, color: 'bg-blue-50 text-blue-600', change: '', up: false },
  ];

  const activityIcons: Record<string, typeof CheckCircle> = {
    'check-circle': CheckCircle,
    'user-check': UserCheck,
    'mail': Clock,
    'user-x': XCircle,
    'arrow-right-circle': TrendingUp,
  };
</script>

<svelte:head><title>Dashboard – WeddingDB</title></svelte:head>

<div class="p-4 sm:p-7 space-y-4 sm:space-y-6 max-w-[1400px]">
  <!-- Stats -->
  <div class="grid grid-cols-2 lg:grid-cols-5 gap-4">
    {#each statCards as card}
      <div class="bg-white border border-gray-200 rounded-2xl p-3 sm:p-5 flex items-start gap-3 sm:gap-4 hover:shadow-md transition-shadow duration-200">
        <div class="w-10 h-10 sm:w-12 sm:h-12 rounded-xl {card.color} flex items-center justify-center flex-shrink-0">
          <card.icon class="w-5 h-5 sm:w-6 sm:h-6" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-[13px] text-gray-500 font-medium">{card.label}</div>
          <div class="text-[28px] font-extrabold text-gray-900 tracking-tight leading-tight">{card.value}</div>
          {#if card.change}
            <div class="text-xs font-semibold mt-1 {card.up ? 'text-emerald-600' : 'text-red'}">
              {card.up ? '↑' : '↓'} {card.change.replace(/[+-]/g, '')}
            </div>
          {/if}
        </div>
      </div>
    {/each}
  </div>

  <div class="grid grid-cols-1 lg:grid-cols-3 gap-5">
    <!-- RSVP Donut -->
    <div class="bg-white border border-gray-200 rounded-2xl p-6">
      <h3 class="text-base font-bold text-gray-800 mb-5">RSVP Status</h3>
      <div class="relative w-40 h-40 mx-auto mb-5">
        <svg viewBox="0 0 100 100" class="w-full h-full -rotate-90">
          <circle cx="50" cy="50" r="40" fill="none" stroke="#E5E7EB" stroke-width="10" />
          <circle cx="50" cy="50" r="40" fill="none" stroke="#059669" stroke-width="10"
            stroke-dasharray="{2 * Math.PI * 40}" stroke-dashoffset="{2 * Math.PI * 40 * (1 - stats.confirmedGuests / stats.totalGuests)}"
            stroke-linecap="round" />
          <circle cx="50" cy="50" r="40" fill="none" stroke="#D97706" stroke-width="10"
            stroke-dasharray="{2 * Math.PI * 40}" stroke-dashoffset="{2 * Math.PI * 40 * (1 - stats.pendingRsvp / stats.totalGuests)}"
            stroke-linecap="round"
            style="transform: rotate({360 * stats.confirmedGuests / stats.totalGuests}deg); transform-origin: center;" />
        </svg>
      </div>
      <div class="flex justify-center gap-5 flex-wrap text-sm">
        <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 rounded-full bg-emerald-500"></span> Confirmed ({stats.confirmedGuests})</div>
        <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 rounded-full bg-amber-500"></span> Pending ({stats.pendingRsvp})</div>
        <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 rounded-full bg-red"></span> Declined ({stats.declined})</div>
      </div>
    </div>

    <!-- Table Occupancy -->
    <div class="bg-white border border-gray-200 rounded-2xl p-6">
      <h3 class="text-base font-bold text-gray-800 mb-5">Table Occupancy</h3>
      <div class="space-y-3">
        {#each occupancy.slice(0, 10) as t}
          <div class="flex items-center gap-3">
            <span class="w-10 text-right text-xs font-semibold text-gray-700">T{t.tableId}</span>
            <div class="flex-1 h-2.5 bg-gray-100 rounded-full overflow-hidden">
              <div
                class="h-full rounded-full transition-all duration-500"
                style="width: {t.percentage}%; background: {t.percentage >= 90 ? '#A11217' : t.percentage >= 60 ? '#D4AF37' : '#059669'}"
              ></div>
            </div>
            <span class="w-10 text-xs font-semibold text-gray-600">{t.percentage}%</span>
          </div>
        {/each}
      </div>
    </div>

    <!-- Recent Activity -->
    <div class="bg-white border border-gray-200 rounded-2xl p-6">
      <h3 class="text-base font-bold text-gray-800 mb-5">Recent Activity</h3>
      <div class="space-y-4">
        {#each activity as item}
          {@const Icon = activityIcons[item.icon] || CheckCircle}
          <div class="flex items-start gap-3">
            <div class="w-8 h-8 rounded-lg bg-gray-50 flex items-center justify-center flex-shrink-0 mt-0.5">
              <Icon class="w-4 h-4 text-gray-500" />
            </div>
            <div class="flex-1 min-w-0">
              <p class="text-sm text-gray-700">
                <span class="font-semibold">{item.guestName}</span>
                {item.message}
              </p>
              <time class="text-xs text-gray-400">{dayjs(item.timestamp).fromNow()}</time>
            </div>
          </div>
        {/each}
      </div>
    </div>
  </div>
</div>
