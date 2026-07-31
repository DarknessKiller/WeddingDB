<script lang="ts">
  import { getDashboardData } from '$lib/api/dashboard';
  import { weddingTitle } from '$lib/stores/weddingTitle';
  import { addToast } from '$lib/stores';
  import { get } from 'svelte/store';
  import { weddingId } from '$lib/stores/weddingId';
  import {
    Users, UserCheck, Clock, XCircle, CheckCircle, TrendingUp,
    ArrowUpRight, ArrowDownRight, LayoutGrid, Monitor, Copy, ExternalLink
  } from 'lucide-svelte';
  import dayjs from 'dayjs';
  import relativeTime from 'dayjs/plugin/relativeTime';
  import { onMount } from 'svelte';
  import type { DashboardStats, TableOccupancy, ActivityItem } from '$lib/types';

  dayjs.extend(relativeTime);

  let stats: DashboardStats = $state({ totalGuests: 0, confirmedGuests: 0, pendingRsvp: 0, declined: 0, checkedIn: 0, totalPax: 0, totalTables: 0, occupiedTables: 0, averageOccupancy: 0 });
  let occupancy: TableOccupancy[] = $state([]);
  let activity: ActivityItem[] = $state([]);
  let loading = $state(true);
  let loaded = $state(false);

  let totalGuests = $derived(stats.totalGuests || 1);
  let confirmedPct = $derived(stats.confirmedGuests / totalGuests);
  let pendingPct = $derived(stats.pendingRsvp / totalGuests);
  const DONUT_R = 40;
  const DONUT_CIRCUM = 2 * Math.PI * DONUT_R;

  onMount(async () => {
    try {
      const wid = get(weddingId);
      const data = await getDashboardData(wid);
      stats = data.stats;
      occupancy = data.occupancy;
      activity = data.activity;
      loaded = true;
    } catch (e) {
      addToast('Failed to load dashboard data', 'error');
    } finally {
      loading = false;
    }
  });

  const activityIcons: Record<string, typeof CheckCircle> = {
    'check-circle': CheckCircle,
    'user-check': UserCheck,
    'mail': Clock,
    'user-x': XCircle,
    'arrow-right-circle': TrendingUp,
  };
</script>

<svelte:head> <title>{$weddingTitle ? `${$weddingTitle} – Dashboard` : 'Dashboard – WeddingDB'}</title></svelte:head>

<div class="p-4 sm:p-7 space-y-4 sm:space-y-6 max-w-[1400px]">
  {#if loading}
    <div class="text-center text-gray-400 py-16 text-sm">Loading dashboard…</div>
  {:else if loaded}
    <!-- Kiosk Quick Access -->
    <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl p-4 sm:p-5 flex items-center justify-between gap-4 shadow-sm">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 sm:w-12 sm:h-12 rounded-xl bg-red flex items-center justify-center flex-shrink-0">
          <Monitor class="w-5 h-5 sm:w-6 sm:h-6 text-white" />
        </div>
        <div>
          <h3 class="text-sm font-bold text-gray-900">Guest Kiosk</h3>
          <p class="text-xs text-gray-500">Share this with guests to find their seats</p>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <button
          onclick={() => {
            const url = `${window.location.origin}/kiosk/${get(weddingId)}`;
            const ta = document.createElement('textarea');
            ta.value = url;
            ta.style.position = 'fixed';
            ta.style.opacity = '0';
            document.body.appendChild(ta);
            ta.select();
            document.execCommand('copy');
            document.body.removeChild(ta);
            addToast('Kiosk link copied!', 'success');
          }}
          class="px-3 py-2 sm:px-4 sm:py-2.5 bg-white/90 border border-black/[0.06] hover:bg-gray-50 text-gray-700 rounded-xl text-xs sm:text-sm font-semibold transition-colors flex items-center gap-1.5 min-h-[44px]"
        >
          <Copy class="w-3.5 h-3.5" /> Copy Link
        </button>
        <a
          href="/kiosk/{get(weddingId)}"
          target="_blank"
          class="px-3 py-2 sm:px-4 sm:py-2.5 bg-red hover:bg-red-light text-white rounded-xl text-xs sm:text-sm font-semibold transition-colors flex items-center gap-1.5 min-h-[44px]"
        >
          <ExternalLink class="w-3.5 h-3.5" /> Open Kiosk
        </a>
      </div>
    </div>

    <!-- Stats -->
    <div class="grid grid-cols-2 lg:grid-cols-5 gap-4">
      {#each [
        { label: 'Confirmed Guests', value: stats.confirmedGuests, icon: UserCheck, color: 'bg-red-50 text-red' },
        { label: 'Pending RSVP', value: stats.pendingRsvp, icon: Clock, color: 'bg-gold-50 text-gold-dark' },
        { label: 'Declined', value: stats.declined, icon: XCircle, color: 'bg-gray-100 text-gray-600' },
        { label: 'Checked In', value: stats.checkedIn, icon: CheckCircle, color: 'bg-emerald-50 text-emerald-600' },
        { label: 'Total Pax', value: stats.totalPax, icon: Users, color: 'bg-blue-50 text-blue-600' }
      ] as card}
        <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl p-3 sm:p-5 flex items-start gap-3 sm:gap-4 hover:shadow-md transition-all duration-300" style="transition-timing-function: cubic-bezier(0.2, 0.8, 0.2, 1);">
          <div class="w-10 h-10 sm:w-12 sm:h-12 rounded-xl {card.color} flex items-center justify-center flex-shrink-0">
            <card.icon class="w-5 h-5 sm:w-6 sm:h-6" />
          </div>
          <div class="flex-1 min-w-0">
            <div class="text-[13px] text-gray-500 font-medium">{card.label}</div>
            <div class="text-[28px] font-extrabold text-gray-900 leading-tight" style="letter-spacing: -0.02em;">{card.value}</div>
          </div>
        </div>
      {/each}
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-5">
      <!-- RSVP Donut -->
      <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl p-6 shadow-sm">
        <h3 class="text-base font-bold text-gray-800 mb-5" style="letter-spacing: -0.01em;">RSVP Status</h3>
      <div class="relative w-40 h-40 mx-auto mb-5">
        <svg viewBox="0 0 100 100" class="w-full h-full -rotate-90">
          <circle cx="50" cy="50" r={DONUT_R} fill="none" stroke="#E5E7EB" stroke-width="10" />
          {#if confirmedPct > 0}
            <circle cx="50" cy="50" r={DONUT_R} fill="none" stroke="#059669" stroke-width="10"
              stroke-dasharray="{DONUT_CIRCUM * confirmedPct} {DONUT_CIRCUM * (1 - confirmedPct)}"
              stroke-linecap="round" />
          {/if}
          {#if pendingPct > 0}
            <circle cx="50" cy="50" r={DONUT_R} fill="none" stroke="#D97706" stroke-width="10"
              stroke-dasharray="{DONUT_CIRCUM * pendingPct} {DONUT_CIRCUM * (1 - pendingPct)}"
              stroke-dashoffset={-DONUT_CIRCUM * confirmedPct}
              stroke-linecap="round" />
          {/if}
        </svg>
      </div>
        <div class="flex justify-center gap-5 flex-wrap text-sm">
          <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 rounded-full bg-emerald-500"></span> Confirmed ({stats.confirmedGuests})</div>
          <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 rounded-full bg-amber-500"></span> Pending ({stats.pendingRsvp})</div>
          <div class="flex items-center gap-2"><span class="w-2.5 h-2.5 rounded-full bg-red"></span> Declined ({stats.declined})</div>
        </div>
      </div>

      <!-- Table Occupancy -->
      <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl p-6 shadow-sm">
        <h3 class="text-base font-bold text-gray-800 mb-5" style="letter-spacing: -0.01em;">Table Occupancy</h3>
        <div class="space-y-3">
          {#each occupancy.slice(0, 10) as t}
            <div class="flex items-center gap-3">
              <span class="w-20 text-right text-xs font-semibold text-gray-700 truncate">{t.tableName}</span>
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
      <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl p-6 shadow-sm">
        <h3 class="text-base font-bold text-gray-800 mb-5" style="letter-spacing: -0.01em;">Recent Activity</h3>
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
  {:else}
    <div class="text-center text-gray-400 py-16 text-sm">No data available</div>
  {/if}
</div>
