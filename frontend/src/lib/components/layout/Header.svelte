<script lang="ts">
  import { page } from '$app/state';
  import { sidebarCollapsed } from '$lib/stores';
  import { Menu, X } from 'lucide-svelte';

  const TITLES: Record<string, string> = {
    '/dashboard': 'Dashboard',
    '/guests': 'Guest Management',
    '/seating': 'Seating Map',
    '/search': 'Reception Check-In',
    '/kiosk': 'Kiosk Mode',
    '/reservation': 'Reservations',
    '/tables': 'Table Management',
    '/settings': 'Settings',
    '/users': 'User Management',
  };

  let pageTitle = $derived(TITLES[page.url.pathname] || 'WeddingDB');
</script>

<header class="app-header">
  <button
    onclick={() => $sidebarCollapsed = !$sidebarCollapsed}
    class="header-menu-btn"
    aria-label="Toggle menu"
  >
    {#if $sidebarCollapsed}
      <Menu class="w-5 h-5" />
    {:else}
      <X class="w-5 h-5" />
    {/if}
  </button>
  <span class="header-title">{pageTitle}</span>
</header>

<style>
  .app-header {
    height: 3.5rem;
    display: flex;
    align-items: center;
    padding: 0 1rem;
    gap: 0.75rem;
    flex-shrink: 0;
    position: relative;
    z-index: 50;
    /* Apple translucent material */
    background: rgba(255, 255, 255, 0.72);
    backdrop-filter: blur(20px) saturate(180%);
    -webkit-backdrop-filter: blur(20px) saturate(180%);
    border-bottom: 1px solid rgba(0, 0, 0, 0.06);
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.8);
  }

  @media (min-width: 640px) {
    .app-header {
      height: 4rem;
      padding: 0 1.75rem;
    }
  }

  .header-menu-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2.5rem;
    height: 2.5rem;
    border-radius: 0.75rem;
    color: #374151;
    flex-shrink: 0;
    min-width: 44px;
    min-height: 44px;
    margin-left: -0.5rem;
    transition: background 100ms ease, transform 100ms ease;
  }

  .header-menu-btn:active {
    transform: scale(0.92);
    background: rgba(0, 0, 0, 0.06);
  }

  .header-title {
    font-size: 1.125rem;
    font-weight: 700;
    color: #111827;
    letter-spacing: -0.02em;
  }
</style>
