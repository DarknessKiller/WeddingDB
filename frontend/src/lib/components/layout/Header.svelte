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

<header class="h-14 sm:h-16 bg-white border-b border-gray-200 flex items-center px-4 sm:px-7 flex-shrink-0 gap-3 relative z-50">
  <button
    onclick={() => $sidebarCollapsed = !$sidebarCollapsed}
    class="lg:hidden p-2 -ml-2 rounded-xl hover:bg-gray-100 transition-colors flex-shrink-0"
    aria-label="Toggle menu"
  >
    {#if $sidebarCollapsed}
      <Menu class="w-5 h-5 text-gray-700" />
    {:else}
      <X class="w-5 h-5 text-gray-700" />
    {/if}
  </button>
  <span class="text-lg font-bold text-gray-900 tracking-tight">{pageTitle}</span>
</header>
