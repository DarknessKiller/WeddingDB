<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/state';
  import Sidebar from '#lib/components/layout/Sidebar.svelte';
  import Header from '#lib/components/layout/Header.svelte';
  import Drawer from '#lib/components/ui/Drawer.svelte';
  import { selectedGuest, isDrawerOpen, drawerStartEditing, drawerCreateMode, sidebarCollapsed, showSeatNumbers, setShowSeatNumbers } from '#lib/stores/index.ts';
  import { weddingId, setWeddingId } from '#lib/stores/weddingId.ts';
  import { weddingTitle } from '#lib/stores/weddingTitle.ts';
  import { validateToken } from '#lib/utils/auth.ts';
  import { fetchAllGuests, isNetworkError } from '#lib/api/guests.ts';
  import { syncQueue, loadCachedGuests, queuedCount } from '#lib/offline/queue.ts';
  import { getWedding } from '#lib/api/weddings.ts';
  import { listTables } from '#lib/api/tables.ts';
  import { initializeSSE, refreshGuests, clearGuests, guestList, toGuest, reviveCachedGuest } from '#lib/stores/guestEvents.ts';
  import type { BanquetTable, Guest } from '#lib/types/index.ts';

  let { children } = $props();

  let currentPath = $derived(page.url.pathname);
  let wid = $derived(page.params.wid ?? '');
  let authChecked = $state(false);
  // Follows the live list, so SSE creates, deletes and imports update the badge.
  let guestCount = $derived($guestList.length);
  let tables = $state<BanquetTable[]>([]);
  let cleanupSSE: (() => void) | undefined;
  let offline = $state(typeof navigator !== 'undefined' ? !navigator.onLine : false);
  let queued = $state(0);

  function refreshQueued() {
    if (typeof window === 'undefined' || !wid) return;
    queued = queuedCount(wid);
  }

  const loadGuests = async (w: string) => {
    try {
      const guests = (await fetchAllGuests(w)).map((g) => toGuest(g));
      return guests;
    } catch (e) {
      // Only an unreachable server may fall back to cached data. An HTTP
      // rejection (401/403/404) means the server denied access; serving a
      // cached roster would leak a wedding the user can no longer open. Venue
      // wifi without internet still reports onLine, so a failed or timed-out
      // fetch counts as offline too.
      if (isNetworkError(e)) {
        // The cache holds Guest objects, not server records: only revive the dates.
        const cached = loadCachedGuests<Guest>(w);
        if (cached) return cached.map(reviveCachedGuest);
      }
      throw new Error('offline');
    }
  };

  // Guest stores are module state shared across routes. Clear them before a new
  // wedding renders, so the previous wedding's or user's roster never shows.
  function resetWeddingState() {
    clearGuests();
    selectedGuest.set(null);
    isDrawerOpen.set(false);
    drawerCreateMode.set(false);
    tables = [];
  }

  async function startWedding(w: string) {
    cleanupSSE?.();
    setWeddingId(w);
    resetWeddingState();
    // Subscribe before the snapshot; guestEvents queues mutations until seeding completes.
    cleanupSSE = initializeSSE(() => loadGuests(w));
    try {
      // refreshGuests keeps the syncing window open across fetch+seed even if
      // initializeSSE early-returned, so mid-fetch SSE events can't be clobbered.
      await refreshGuests(() => loadGuests(w));
    } catch {}
    // Another startWedding took over while this one loaded; its own calls own the UI.
    if (destroyed || w !== $weddingId) return;
    listTables(w).then((t) => {
      tables = t;
    }).catch(() => {});
    getWedding(w).then((wedding) => {
      weddingTitle.set(wedding.name || '');
      setShowSeatNumbers(w, wedding.showSeatNumbers ?? false);
    }).catch(() => {});
  }

  let destroyed = false;
  let activeWid = '';
  // Set in onMount; the banner's "Sync now" runs the same sync-and-reseed path.
  let syncNow: () => Promise<void> = async () => {};
  let cleanupListeners: (() => void) | undefined;
  onMount(async () => {
    if (!await validateToken() || destroyed) return;

    async function doSync() {
      if (!wid || typeof window === 'undefined' || !navigator.onLine) return;
      try {
        await syncQueue(wid);
        // Refresh inside the syncing window so SSE events arriving mid-fetch
        // are queued and replayed after the seed instead of being clobbered.
        const w = wid;
        await refreshGuests(() => loadGuests(w));
        refreshQueued();
      } catch {}
    }

    syncNow = doSync;
    const onOnline = () => { offline = false; refreshQueued(); doSync(); };
    const onOffline = () => { offline = true; };
    const onVis = () => { if (document.visibilityState === 'visible' && navigator.onLine) doSync(); };
    window.addEventListener('online', onOnline);
    window.addEventListener('offline', onOffline);
    document.addEventListener('visibilitychange', onVis);
    window.addEventListener('storage', refreshQueued);
    // same-tab queue updates: storage events only fire in other tabs
    window.addEventListener('offline-queue-changed', refreshQueued);
    cleanupListeners = () => {
      window.removeEventListener('online', onOnline);
      window.removeEventListener('offline', onOffline);
      document.removeEventListener('visibilitychange', onVis);
      window.removeEventListener('storage', refreshQueued);
      window.removeEventListener('offline-queue-changed', refreshQueued);
    };
    refreshQueued();

    // initial sync before seed
    if (wid && navigator.onLine) { try { await syncQueue(wid); } catch {} }

    if (destroyed) return;
    if (wid) {
      activeWid = wid;
      await startWedding(wid);
    }
    if (destroyed) return;
    authChecked = true;
  });

  // A wedding switch reuses this layout without a remount; never keep the
  // previous wedding's roster on screen.
  $effect(() => {
    const next = wid;
    if (!next || !authChecked || next === activeWid) return;
    activeWid = next;
    void startWedding(next);
  });

  onDestroy(() => {
    destroyed = true;
    cleanupListeners?.();
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
        <div class="offline-banner queued">{queued} queued — syncing on reconnect <button onclick={() => syncNow()}>Sync now</button></div>
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
    <Drawer guest={$selectedGuest ?? undefined} tables={tables} showSeatNumbers={$showSeatNumbers} startEditing={$drawerStartEditing} createMode={$drawerCreateMode} onClose={() => { $isDrawerOpen = false; $selectedGuest = null; $drawerStartEditing = false; $drawerCreateMode = false; }} />
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
    /* State indication: banner slides down instead of teleporting in */
    transform: translateY(0);
    animation: bannerIn 200ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }

  @keyframes bannerIn {
    from { transform: translateY(-100%); }
    to { transform: translateY(0); }
  }

  @media (prefers-reduced-motion: reduce) {
    .offline-banner { animation: none; }
  }
  .offline-banner.queued { background: #7a2b00; }
  .offline-banner button { margin-left: 0.5rem; background: white; color: #A11217; border: 0; border-radius: 999px; padding: 0.15rem 0.6rem; font-weight: 600; cursor: pointer; }
</style>
