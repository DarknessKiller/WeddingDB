<script lang="ts">
  import { page } from '$app/state';

  const TITLES: Record<string, string> = {
    '/dashboard': 'Dashboard',
    '/guests': 'Guest Management',
    '/seating': 'Seating Map',
    '/search': 'Reception Check-In',
    '/kiosk': 'Kiosk Mode',
    '/reservation': 'Reservations',
    '/tables': 'Table Management',
    '/settings': 'Settings',
  };

  let pageTitle = $derived(TITLES[page.url.pathname] || 'WeddingDB');
</script>

<header class="h-14 sm:h-16 bg-white border-b border-gray-200 flex items-center px-4 sm:px-7 flex-shrink-0 gap-4">
  <span class="text-lg font-bold text-gray-900 tracking-tight pl-10 lg:pl-0">{pageTitle}</span>
</header>
