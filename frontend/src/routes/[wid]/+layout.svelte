<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/state';
  import Sidebar from '$lib/components/layout/Sidebar.svelte';
  import Header from '$lib/components/layout/Header.svelte';
  import Drawer from '$lib/components/ui/Drawer.svelte';
  import { selectedGuest, isDrawerOpen, drawerStartEditing, drawerCreateMode, sidebarCollapsed } from '$lib/stores';
  import { weddingId, setWeddingId } from '$lib/stores/weddingId';
  import { weddingTitle } from '$lib/stores/weddingTitle';
  import { validateToken } from '$lib/utils/auth';
  import { fetchAllGuests } from '$lib/api/guests';
  import { getWedding } from '$lib/api/weddings';
  import { listTables } from '$lib/api/tables';
  import { initializeSSE, seedGuests } from '$lib/stores/guestEvents';
  import type { BanquetTable } from '$lib/types';

  let { children } = $props();

  let currentPath = $derived(page.url.pathname);
  let wid = $derived(page.params.wid ?? '');
  let authChecked = $state(false);
  let guestCount = $state(0);
  let tables = $state<BanquetTable[]>([]);
  let showSeatNumbers = $state(true);
  let cleanupSSE: (() => void) | undefined;
  let offline = $state(typeof navigator !== 'undefined' ? !navigator.onLine : false);
  let queued = $state(0);

  function refreshQueued() {
    if (typeof window === 'undefined' || !wid) return;
    try { queued = JSON.parse(localStorage.getItem(`offline_queue_${wid}`) || '[]').length; } catch { queued = 0; }
  }

  onMount(async () => {
    if (!await validateToken()) return;
    if (wid) {
      setWeddingId(wid);
    }
    authChecked = true;

    const toGuest = (g: any) => ({
      id: g.id, name: g.name, phone: g.phone, email: g.email,
      rsvp: g.rsvp as any, pax: g.pax, tableId: g.tableId, seatNumber: g.seatNum,
      checkedIn: !!g.checkedInAt, checkedInAt: g.checkedInAt ? new Date(g.checkedInAt) : undefined,
      notes: g.notes, dietaryRequirements: g.dietary ?? [], isVip: g.isVip,
      angbaoAmount: g.angbaoAmt ?? undefined, giftItem: g.giftItem ?? undefined, createdAt: new Date(),
    });
    const loadGuests = async () => {
      try {
        const guests = (await fetchAllGuests($weddingId)).map(toGuest);
        return guests;
      } catch {
        try {
          const cached = JSON.parse(localStorage.getItem(`offline_cache_${$weddingId}`) || 'null');
          if (cached) return (cached as any[]).map(toGuest);
        } catch {}
        throw new Error('offline');
      }
    };

    async function doSync() {
      if (!wid || typeof window === 'undefined' || !navigator.onLine) return;
      try {
        const { syncQueue } = await import('$lib/offline/queue');
        await syncQueue(wid);
        const guests = await loadGuests();
        guestCount = guests.length;
        seedGuests(guests);
        refreshQueued();
      } catch {}
    }

    const onOnline = () => { offline = false; refreshQueued(); doSync(); };
    const onOffline = () => { offline = true; };
    const onVis = () => { if (document.visibilityState === 'visible' && navigator.onLine) doSync(); };
    window.addEventListener('online', onOnline);
    window.addEventListener('offline', onOffline);
    document.addEventListener('visibilitychange', onVis);
    window.addEventListener('storage', refreshQueued);
    // same-tab queue updates: storage events only fire in other tabs
    window.addEventListener('offline-queue-changed', refreshQueued);
    refreshQueued();

    // initial sync before seed
    if (navigator.onLine) { try { const { syncQueue } = await import('$lib/offline/queue'); await syncQueue(wid); } catch {} }

    // Subscribe before the snapshot; guestEvents queues mutations until seeding completes.
    cleanupSSE = initializeSSE(loadGuests);
    try {
      const guests = await loadGuests();
      guestCount = guests.length;
      seedGuests(guests);
    } catch {}

    listTables($weddingId).then((t) => {
      tables = t;
    }).catch(() => {});
    getWedding(wid).then((w) => {
      weddingTitle.set(w.name || '');
      showSeatNumbers = w.showSeatNumbers ?? true;
    }).catch(() => {});
  });

  onDestroy(() => {
    cleanupSSE?.();
  });

  $effect(() => {
    currentPath;
    if (typeof window !== 'undefined' && window.innerWidth < 1024) {
      $sidebarCollapsed = true;
    }
  });
</script>

{#if !authChecked}
  <div class="loading-screen">
    <div class="loading-spinner"></div>
  </div>
{:else}
  <div class="app-layout">
    <!-- Mobile backdrop -->
    {#if !$sidebarCollapsed}
      <div
        class="sidebar-backdrop lg:hidden"
        onclick={() => $sidebarCollapsed = true}
        onkeydown={(e) => { if (e.key === 'Escape') $sidebarCollapsed = true; }}
        role="button"
        tabindex="-1"
        aria-label="Close sidebar"
      ></div>
    {/if}

    <Sidebar {currentPath} {guestCount} {wid} />

    <div class="main-area">
      {#if offline}
        <div class="offline-banner">Offline — changes queued{queued ? ` (${queued})` : ''}</div>
      {:else if queued}
        <div class="offline-banner queued">{queued} queued — syncing on reconnect <button onclick={async () => { const { syncQueue } = await import('$lib/offline/queue'); await syncQueue(wid); refreshQueued(); }}>Sync now</button></div>
      {/if}
      <Header />
      <main class="main-content">
        {@render children()}
      </main>
    </div>
  </div>
{/if}

{#if $isDrawerOpen && ($selectedGuest || $drawerCreateMode)}
  {#key `${$drawerStartEditing}-${$drawerCreateMode}`}
    <Drawer guest={$selectedGuest ?? undefined} tables={tables} {showSeatNumbers} startEditing={$drawerStartEditing} createMode={$drawerCreateMode} onClose={() => { $isDrawerOpen = false; $selectedGuest = null; $drawerStartEditing = false; $drawerCreateMode = false; }} />
  {/key}
{/if}

<style>
  .loading-screen {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100dvh;
    background: linear-gradient(180deg, #fef2f2 0%, #faf7f2 50%, white 100%);
    background-attachment: fixed;
  }

  .loading-spinner {
    width: 2rem;
    height: 2rem;
    border: 2.5px solid rgba(161, 18, 23, 0.2);
    border-top-color: #A11217;
    border-radius: 50%;
    animation: spin 600ms linear infinite;
  }

  .app-layout {
    display: flex;
    height: 100dvh;
    overflow: hidden;
    background: linear-gradient(180deg, #fef2f2 0%, #faf7f2 50%, white 100%);
    background-attachment: fixed;
  }

  .sidebar-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 55;
    animation: fadeIn 200ms ease;
  }

  .main-area {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    position: relative;
    overflow: hidden;
  }

  .main-content {
    flex: 1;
    overflow-y: auto;
    /* ponytail: no bg, inherit gradient from body */
    padding-top: 3.5rem;
    padding-bottom: env(safe-area-inset-bottom);
  }

  @media (min-width: 640px) {
    .main-content {
      padding-top: 4rem;
    }
  }



  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }
  .offline-banner {
    background: #A11217; color: white; text-align: center; padding: 0.35rem; font-size: 0.85rem; z-index: 40;
  }
  .offline-banner.queued { background: #7a2b00; }
  .offline-banner button { margin-left: 0.5rem; background: white; color: #A11217; border: 0; border-radius: 999px; padding: 0.15rem 0.6rem; font-weight: 600; cursor: pointer; }
</style>
