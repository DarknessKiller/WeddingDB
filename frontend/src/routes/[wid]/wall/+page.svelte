<script lang="ts">
  // Read-only live status wall: one row per table, updates in the same second
  // on every screen via the SSE-backed guestList (offline-queue aware).
  import { onMount } from 'svelte';
  import { listTables } from '$lib/api/tables';
  import { guestList } from '$lib/stores/guestEvents';
  import { addToast } from '$lib/stores';
  import { get } from 'svelte/store';
  import { weddingId } from '$lib/stores/weddingId';
  import { t } from '$lib/i18n';
  import { page } from '$app/state';
  import type { BanquetTable } from '$lib/types';

  let tables = $state<BanquetTable[]>([]);
  let loading = $state(true);

  const wid = $derived(page.params.wid ?? get(weddingId));

  const rows = $derived.by(() => {
    const guests = $guestList;
    return tables.map(tb => {
      const seated = guests.filter(g => g.tableId === tb.id);
      const confirmedPax = seated.filter(g => g.rsvp === 'confirmed').reduce((s, g) => s + g.pax, 0);
      const arrivedPax = seated.filter(g => g.checkedIn).reduce((s, g) => s + g.pax, 0);
      const pct = confirmedPax > 0 ? Math.round((arrivedPax / confirmedPax) * 100) : 0;
      const full = confirmedPax > 0 && arrivedPax >= confirmedPax;
      return { tb, confirmedPax, arrivedPax, pct, full };
    });
  });

  function barColor(pct: number): string {
    return pct >= 90 ? '#A11217' : pct >= 60 ? '#D4AF37' : '#059669';
  }

  onMount(async () => {
    try {
      tables = await listTables(wid);
    } catch (e: any) {
      addToast(e.message ?? t('toast.loadTablesFailed'), 'error');
    } finally {
      loading = false;
    }
  });
</script>

<svelte:head>
  <title>{t('nav.wall')} – WeddingDB</title>
</svelte:head>

<div class="p-4 sm:p-8 max-w-3xl mx-auto">
  <h1 class="text-xl font-bold text-gray-900 mb-1" style="letter-spacing: -0.02em;">{t('nav.wall')}</h1>
  <p class="text-xs text-gray-500 mb-5">{t('wall.subtitle')}</p>

  {#if loading}
    <div class="text-center text-gray-400 py-16 text-sm">{t('common.loading')}</div>
  {:else if tables.length === 0}
    <p class="text-sm text-gray-400">{t('dash.noTables')}</p>
  {:else}
    <div class="space-y-2">
      {#each rows as r (r.tb.id)}
        <div class="wall-row bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl px-4 py-3 shadow-sm flex items-center gap-4">
          <span class="wall-name text-sm font-bold text-gray-900 truncate min-w-16" style="letter-spacing: -0.01em;">{r.tb.name || r.tb.id}{r.tb.isVip ? ' ★' : ''}</span>
          <div class="flex-1 h-2.5 bg-gray-100 rounded-full overflow-hidden">
            <div class="h-full rounded-full transition-all duration-300" style="width: {r.pct}%; background: {barColor(r.pct)};"></div>
          </div>
          <span class="wall-nums text-sm font-bold {r.full ? 'text-green-700' : 'text-gray-700'}">
            {r.arrivedPax}/{r.confirmedPax || 0}
          </span>
          {#if r.full}
            <span class="wall-state text-xs font-bold text-green-700" aria-hidden="true">✅</span>
          {:else}
            <span class="wall-state text-xs font-bold" aria-hidden="true">⏳</span>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .wall-nums { min-width: 4.5rem; text-align: right; font-variant-numeric: tabular-nums; }
  .wall-state { min-width: 2rem; text-align: center; }
</style>
