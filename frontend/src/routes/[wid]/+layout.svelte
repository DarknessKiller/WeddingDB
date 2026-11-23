<script lang="ts">
  import { page } from '$app/state';
  import { isNetworkError } from '#lib/api/client.ts';
  import Sidebar from '#lib/components/layout/Sidebar.svelte';
  import Header from '#lib/components/layout/Header.svelte';
  import Drawer from '#lib/components/ui/Drawer.svelte';
  import { selectedGuest, isDrawerOpen, drawerStartEditing, drawerCreateMode, sidebarCollapsed, showSeatNumbers, setShowSeatNumbers, addToast, weddingTables } from '#lib/stores/index.ts';
  import { setWeddingId } from '#lib/stores/weddingId.ts';
  import { weddingTitle } from '#lib/stores/weddingTitle.ts';
  import { validateToken } from '#lib/utils/auth.ts';
  import { fetchAllGuests } from '#lib/api/guests.ts';
  import { syncQueue } from '#lib/offline/queue.ts';
  import { getWedding } from '#lib/api/weddings.ts';
  import { listTables } from '#lib/api/tables.ts';
  import { guestList, initializeSSE, readCachedGuests, refreshGuests } from '#lib/stores/guestEvents.ts';
  import { toGuest } from '#lib/utils/guestTransform.ts';

  let { children } = $props();

  let currentPath = $derived(page.url.pathname);
  let wid = $derived(page.params.wid ?? '');
  let authChecked = $state(false);
  let guestCount = $derived($guestList.length);
  let offline = $state(typeof navigator !== 'undefined' ? !navigator.onLine : false);
  let queued = $state(0);
  let loadError = $state(false);
  // Queue sync and guest loading fail independently; one banner each.
  let syncError = $state(false);

  function refreshQueued() {
    if (typeof window === 'undefined' || !wid) return;
    try { queued = JSON.parse(localStorage.getItem(`offline_queue_${wid}`) || '[]').length; } catch { queued = 0; }
  }

  // Bound to the wedding the layout opened: a slow response for wedding A
  // must not land under wedding B, and its cache key must match A.
  const loadGuests = async (forWid: string) => {
    try {
      return (await fetchAllGuests(forWid)).map(toGuest);
    } catch (e) {
      // Only a network failure may fall back to cache. A 403 or other 4xx is
      // not offline, and showing another session's cached guests would leak them.
      if (!isNetworkError(e)) throw e;
      const cached = readCachedGuests(forWid);
      if (cached) return cached;
      throw e;
    }
  };

  async function retryGuests() {
    loadError = false;
    try {
      await refreshGuests(() => loadGuests(wid), wid);
    } catch {
      loadError = true;
    }
  }

  // One setup per wedding id: changing wedding tears down SSE and reloads.
  $effect(() => {
    const forWid = wid;
    if (!forWid) return;
    // Do not show the previous wedding's tables, count or seat setting until
    // this wedding's fetches land (or fail).
    weddingTables.set({ wid: '', tables: [] });
    showSeatNumbers.set(false);
    weddingTitle.set('');
    $isDrawerOpen = false;
    $selectedGuest = null;
    $drawerStartEditing = false;
    $drawerCreateMode = false;
    let cancelled = false;
    let localCleanupSSE: (() => void) | undefined;
    let localCleanupListeners: (() => void) | undefined;

    // Queue drains before reseed, on SSE reconnect too.
    const syncThenLoad = async () => {
      try {
        await syncQueue(forWid);
        syncError = false;
      } catch (e) {
        // Reseeding after a failed drain would clobber optimistic queued edits;
        // let the caller end the sync window and replay queued SSE events.
        syncError = true;
        throw e;
      }
      return loadGuests(forWid);
    };

    let syncing = false;
    async function doSync() {
      // One drain at a time: online+visible can fire together, and two drains
      // would replay the same mutations twice.
      if (syncing || cancelled || typeof window === 'undefined' || !navigator.onLine) return;
      syncing = true;
      try {
        // Queue and guest loading fail independently; one banner each.
        let drained = false;
        try {
          await syncQueue(forWid);
          if (!cancelled) syncError = false;
          drained = true;
        } catch {
          if (!cancelled) syncError = true;
        }
        // Reseeding after a failed drain would overwrite the optimistic editing
        // that is still queued; the next drain retries and reseeds.
        if (!drained) return;
        try {
          // Refresh inside the syncing window so SSE events arriving mid-fetch
          // are queued and replayed after the seed instead of being clobbered.
          await refreshGuests(() => loadGuests(forWid), forWid);
          if (cancelled) return;
          loadError = false;
          refreshQueued();
        } catch {
          if (!cancelled) loadError = true;
        }
      } finally {
        syncing = false;
      }
    }

    const onOnline = () => { offline = false; refreshQueued(); doSync(); };
    const onOffline = () => { offline = true; };
    const onVis = () => { if (document.visibilityState === 'visible' && navigator.onLine) doSync(); };

    void (async () => {
      if (!await validateToken() || cancelled) return;
      setWeddingId(forWid);
      authChecked = true;

      window.addEventListener('online', onOnline);
      window.addEventListener('offline', onOffline);
      document.addEventListener('visibilitychange', onVis);
      window.addEventListener('storage', refreshQueued);
      // same-tab queue updates: storage events only fire in other tabs
      window.addEventListener('offline-queue-changed', refreshQueued);
      localCleanupListeners = () => {
        window.removeEventListener('online', onOnline);
        window.removeEventListener('offline', onOffline);
        document.removeEventListener('visibilitychange', onVis);
        window.removeEventListener('storage', refreshQueued);
        window.removeEventListener('offline-queue-changed', refreshQueued);
      };
      refreshQueued();

      // initial sync before seed
      if (navigator.onLine) { try { await syncQueue(forWid); } catch {} }

      if (cancelled) return;
      // Subscribe before the snapshot; guestEvents queues mutations until seeding completes.
      localCleanupSSE = initializeSSE(syncThenLoad);
      try {
        // refreshGuests keeps the syncing window open across fetch+seed even if
        // initializeSSE early-returned, so mid-fetch SSE events can't be clobbered.
        await refreshGuests(() => loadGuests(forWid), forWid);
        if (cancelled) return;
        loadError = false;
        syncError = false;
      } catch {
        if (!cancelled) loadError = true;
      }

      listTables(forWid).then((t) => {
        if (!cancelled) weddingTables.set({ wid: forWid, tables: t });
      }).catch((e: any) => {
        if (!cancelled) addToast(e?.message ?? 'Failed to load tables', 'error');
      });
      getWedding(forWid).then((w) => {
        if (cancelled) return;
        weddingTitle.set(w.name || '');
        setShowSeatNumbers(forWid, w.showSeatNumbers ?? false);
      }).catch((e: any) => {
        if (!cancelled) addToast(e?.message ?? 'Failed to load wedding settings', 'error');
      });
    })();

    return () => {
      cancelled = true;
      localCleanupListeners?.();
      localCleanupSSE?.();
    };
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
        <div class="offline-banner queued">{queued} queued — syncing on reconnect <button onclick={async () => { try { await syncQueue(wid); syncError = false; } catch { addToast('Could not sync queued edits', 'error'); } refreshQueued(); }}>Sync now</button></div>
      {/if}
      {#if loadError}
        <div class="offline-banner">Could not load guests <button onclick={retryGuests}>Retry</button></div>
      {/if}
      {#if syncError}
        <div class="offline-banner">Could not sync changes — edits stay queued</div>
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
    <Drawer guest={$selectedGuest ?? undefined} tables={$weddingTables.tables} showSeatNumbers={$showSeatNumbers} startEditing={$drawerStartEditing} createMode={$drawerCreateMode} onClose={() => { $isDrawerOpen = false; $selectedGuest = null; $drawerStartEditing = false; $drawerCreateMode = false; }} />
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
