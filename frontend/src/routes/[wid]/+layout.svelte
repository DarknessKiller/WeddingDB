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

  onMount(async () => {
    if (!await validateToken()) return;
    if (wid) {
      setWeddingId(wid);
    }
    authChecked = true;

    // Fetch all guests and seed the store FIRST, then start SSE.
    // This prevents SSE events from appending guests that seedGuests
    // would then overwrite, which causes duplicates.
    try {
      const guests = await fetchAllGuests($weddingId);
      guestCount = guests.length;
      seedGuests(guests.map((g) => ({
        id: g.id,
        name: g.name,
        phone: g.phone,
        email: g.email,
        rsvp: g.rsvp as any,
        pax: g.pax,
        tableId: g.tableId,
        seatNumber: g.seatNum,
        checkedIn: !!g.checkedInAt,
        checkedInAt: g.checkedInAt ? new Date(g.checkedInAt) : undefined,
        notes: g.notes,
        dietaryRequirements: g.dietary ?? [],
        isVip: g.isVip,
        angbaoAmount: g.angbaoAmt ?? undefined,
        giftItem: g.giftItem ?? undefined,
        createdAt: new Date(),
      })));
    } catch {}

    // Now start SSE — any events arriving after this point will
    // upsert into the already-seeded list, no duplicates.
    cleanupSSE = initializeSSE();

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
</style>
