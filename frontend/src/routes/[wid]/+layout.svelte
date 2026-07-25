<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import Sidebar from '$lib/components/layout/Sidebar.svelte';
  import Header from '$lib/components/layout/Header.svelte';
  import Drawer from '$lib/components/ui/Drawer.svelte';
  import { selectedGuest, isDrawerOpen, drawerStartEditing, sidebarCollapsed, getAuth } from '$lib/stores';
  import { weddingId, setWeddingId } from '$lib/stores/weddingId';
  import { decodeId } from '$lib/utils/encode';
  import { listGuests } from '$lib/api/guests';
  import { listTables } from '$lib/api/tables';
  import type { BanquetTable } from '$lib/types';

  let { children } = $props();

  let currentPath = $derived(page.url.pathname);
  let wid = $derived(page.params.wid ? decodeId(page.params.wid) : '');
  let authChecked = $state(false);
  let guestCount = $state(0);
  let tables = $state<BanquetTable[]>([]);

  onMount(() => {
    const { accessToken } = getAuth();
    if (!accessToken) {
      goto('/login', { replaceState: true });
      authChecked = true;
      return;
    }
    // Sync URL param to store (wid is already decoded)
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

  // Close sidebar on mobile when navigating
  $effect(() => {
    currentPath;
    if (typeof window !== 'undefined' && window.innerWidth < 1024) {
      $sidebarCollapsed = true;
    }
  });
</script>

{#if !authChecked}
  <div class="flex items-center justify-center h-screen bg-warm-50">
    <div class="w-8 h-8 border-2 border-red border-t-transparent rounded-full animate-spin"></div>
  </div>
{:else}
  <div class="flex h-screen overflow-hidden bg-warm-50">
    {#if !$sidebarCollapsed}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="fixed inset-0 bg-black/40 z-30 lg:hidden"
        onclick={() => $sidebarCollapsed = true}
      ></div>
    {/if}

    <Sidebar {currentPath} {guestCount} {wid} />

    <div class="flex flex-1 flex-col overflow-hidden min-w-0">
      <Header />
      <main class="flex-1 overflow-y-auto bg-white">
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
