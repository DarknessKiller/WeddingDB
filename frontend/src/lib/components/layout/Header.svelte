<script lang="ts">
  import { page } from '$app/state';
  import { sidebarCollapsed, getAuth, clearAuth } from '$lib/stores';
  import { setWeddingId } from '$lib/stores/weddingId';
  import { goto } from '$app/navigation';
  import { Menu, X, LogOut } from 'lucide-svelte';

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
  let auth = $derived(getAuth());
  let initials = $derived(
    (auth.name || 'A').split(' ').map((w: string) => w[0]).join('').toUpperCase().slice(0, 2)
  );

  async function handleLogout() {
    const { refreshToken, accessToken } = getAuth();
    if (refreshToken) {
      try {
        await fetch('/api/auth/logout', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {})
          },
          body: JSON.stringify({ refreshToken })
        });
      } catch { /* ignore */ }
    }
    clearAuth();
    setWeddingId('');
    goto('/login', { replaceState: true });
  }
</script>

<header class="app-header material-glass" class:sidebar-open={!$sidebarCollapsed}>
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

  <div class="header-spacer"></div>

  <div class="header-user">
    <div class="header-user-avatar">{initials}</div>
    <span class="header-user-name">{auth.name || 'Admin'}</span>
    <button onclick={handleLogout} class="header-logout-btn" aria-label="Logout" title="Logout">
      <LogOut class="w-4 h-4" />
      <span class="header-logout-text">Logout</span>
    </button>
  </div>
</header>

<style>
  .app-header {
    height: 3.5rem;
    display: flex;
    align-items: center;
    padding: 0 1rem;
    padding-top: env(safe-area-inset-top);
    gap: 0.75rem;
    flex-shrink: 0;
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    z-index: 50;
  }

  @media (min-width: 640px) {
    .app-header {
      height: 4rem;
      padding: 0 1.75rem;
    }
  }

  @media (min-width: 1024px) {
    .app-header {
      left: 72px;
    }
    .app-header.sidebar-open {
      left: 260px;
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

  .header-spacer { flex: 1; }

  .header-user {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.25rem 0.5rem;
    border-radius: 0.75rem;
    background: rgba(0, 0, 0, 0.03);
    min-height: 44px;
  }

  .header-user-avatar {
    width: 2rem;
    height: 2rem;
    border-radius: 9999px;
    background: #D4AF37;
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: 0.75rem;
    color: white;
    flex-shrink: 0;
  }

  .header-user-name {
    font-size: 0.8125rem;
    font-weight: 600;
    color: #374151;
    white-space: nowrap;
  }

  @media (max-width: 640px) {
    .header-user-name { display: none; }
  }

  .header-logout-btn {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.5rem;
    border-radius: 0.5rem;
    color: #9ca3af;
    min-width: 44px;
    min-height: 44px;
    justify-content: center;
    transition: background 100ms ease, color 100ms ease, transform 100ms ease;
  }

  .header-logout-btn:hover {
    color: #A11217;
    background: rgba(161, 18, 23, 0.06);
  }

  .header-logout-btn:active {
    transform: scale(0.9);
  }

  @media (min-width: 640px) {
    .header-logout-text {
      font-size: 0.8125rem;
      font-weight: 500;
    }
  }

  @media (max-width: 640px) {
    .header-logout-text { display: none; }
  }
</style>
