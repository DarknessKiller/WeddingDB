<script lang="ts">
  import '../app.css';
  import { page } from '$app/state';
  import Sidebar from '$lib/components/layout/Sidebar.svelte';
  import Header from '$lib/components/layout/Header.svelte';
  import ToastContainer from '$lib/components/ui/ToastContainer.svelte';
  import Drawer from '$lib/components/ui/Drawer.svelte';
  import { selectedGuest, isDrawerOpen, sidebarCollapsed } from '$lib/stores';

  let { children } = $props();

  let currentPath = $derived(page.url.pathname);
  let isKiosk = $derived(currentPath.startsWith('/kiosk'));

  // Close sidebar on mobile when navigating
  $effect(() => {
    currentPath;
    if (typeof window !== 'undefined' && window.innerWidth < 1024) {
      $sidebarCollapsed = true;
    }
  });
</script>

{#if isKiosk}
  <!-- Kiosk: standalone layout, no sidebar/header -->
  {@render children()}
{:else}
  <div class="flex h-screen overflow-hidden bg-warm-50">
    <!-- Mobile overlay -->
    {#if !$sidebarCollapsed}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="fixed inset-0 bg-black/40 z-30 lg:hidden"
        onclick={() => $sidebarCollapsed = true}
      ></div>
    {/if}

    <Sidebar {currentPath} />

    <div class="flex flex-1 flex-col overflow-hidden min-w-0">
      <Header />
      <main class="flex-1 overflow-y-auto">
        {@render children()}
      </main>
    </div>
  </div>
{/if}

<ToastContainer />
{#if $isDrawerOpen && $selectedGuest}
  <Drawer guest={$selectedGuest} onClose={() => { $isDrawerOpen = false; $selectedGuest = null; }} />
{/if}
