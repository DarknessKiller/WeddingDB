<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import Sidebar from '$lib/components/layout/Sidebar.svelte';
  import Header from '$lib/components/layout/Header.svelte';
  import Drawer from '$lib/components/ui/Drawer.svelte';
  import { selectedGuest, isDrawerOpen, drawerStartEditing, sidebarCollapsed } from '$lib/stores';
  import { weddingId, setWeddingId } from '$lib/stores/weddingId';
  import { validateToken } from '$lib/utils/auth';
  import { listGuests } from '$lib/api/guests';
  import { listTables } from '$lib/api/tables';
  import type { BanquetTable } from '$lib/types';

  let { children } = $props();

  let currentPath = $derived(page.url.pathname);
  let wid = $derived(page.params.wid ?? '');
  let authChecked = $state(false);
  let guestCount = $state(0);
  let tables = $state<BanquetTable[]>([]);

  onMount(async () => {
    if (!await validateToken()) return;
    if (wid) {
      setWeddingId(wid);
    }
    authChecked = true;
    listGuests($weddingId).then((res) => {
      guestCount = res.total;
    }).catch(() => {});
    listTables($weddingId).then((t) => {
      tables = t;
    }).catch(() => {});
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
    {#if !$sidebarCollapsed}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="sidebar-backdrop"
        onclick={() => $sidebarCollapsed = true}
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

{#if $isDrawerOpen && $selectedGuest}
  {#key $drawerStartEditing}
    <Drawer guest={$selectedGuest} tables={tables} startEditing={$drawerStartEditing} onClose={() => { $isDrawerOpen = false; $selectedGuest = null; $drawerStartEditing = false; }} />
  {/key}
{/if}

<style>
  .loading-screen {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100dvh;
    background: #faf7f2;
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
    background: #faf7f2;
  }

  .sidebar-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.4);
    backdrop-filter: blur(4px);
    -webkit-backdrop-filter: blur(4px);
    z-index: 30;
    animation: fadeIn 200ms ease;
  }

  .main-area {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    min-width: 0;
  }

  .main-content {
    flex: 1;
    overflow-y: auto;
    background: white;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }
</style>
