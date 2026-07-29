<script lang="ts">
  import { cn } from '$lib/utils';
  import { sidebarCollapsed, getAuth } from '$lib/stores';
  import {
    LayoutDashboard, Users, MapPin, Search, Monitor, Calendar,
    Settings, BarChart3, Utensils, Shield
  } from 'lucide-svelte';

  let { currentPath, guestCount = 0, wid = '' }: { currentPath: string; guestCount?: number; wid?: string } = $props();

  const auth = $derived(getAuth());
  const displayName = $derived(auth.name || 'Admin');
  const isAdmin = $derived(auth.role === 'admin');
  const initials = $derived(
    displayName.split(' ').map((w: string) => w[0]).join('').toUpperCase().slice(0, 2)
  );

  const navSections = $derived([
    {
      label: 'Main',
      items: [
        { href: `/${wid}/dashboard`, label: 'Dashboard', icon: LayoutDashboard },
        { href: `/${wid}/guests`, label: 'Guests', icon: Users },
        { href: `/${wid}/tables`, label: 'Tables', icon: Utensils },
      ]
    },
    {
      label: 'Operations',
      items: [
        { href: `/${wid}/seating`, label: 'Seating Map', icon: MapPin },
        { href: `/${wid}/search`, label: 'Check In', icon: Search },
      ]
    },
    {
      label: 'Management',
      items: [
        { href: '/weddings', label: 'Weddings', icon: Calendar },
        ...(isAdmin ? [{ href: `/${wid}/users`, label: 'Users', icon: Shield }] : []),
      ]
    },
    {
      label: 'Other',
      items: [
        { href: `/${wid}/settings`, label: 'Settings', icon: Settings },
      ]
    }
  ]);

  function closeOnMobile() {
    if (window.innerWidth < 1024) $sidebarCollapsed = true;
  }
</script>

<aside class={cn(
  "sidebar-root",
  $sidebarCollapsed ? "sidebar-collapsed" : "sidebar-expanded"
)}>
  <!-- Navigation -->
  <nav class="sidebar-nav">
    {#each navSections as section}
      {#if !$sidebarCollapsed}
        <div class="nav-section-label">{section.label}</div>
      {/if}
      {#each section.items as item}
        <a
          href={item.href}
          onclick={closeOnMobile}
          title={$sidebarCollapsed ? item.label : undefined}
          class={cn(
            'nav-item',
            $sidebarCollapsed ? 'nav-item-collapsed' : 'nav-item-expanded',
            currentPath === item.href ? 'nav-item-active' : ''
          )}
        >
          {#if currentPath === item.href}
            <div class="nav-active-indicator"></div>
          {/if}
          <item.icon class="nav-icon" />
          {#if !$sidebarCollapsed}
            {item.label}
            {#if item.href === '/guests' && guestCount > 0}
              <span class="nav-badge">{guestCount}</span>
            {/if}
          {/if}
        </a>
      {/each}
    {/each}
  </nav>


</aside>

<style>
  .sidebar-root {
    height: 100svh;
    background: white;
    border-right: 1px solid rgba(0, 0, 0, 0.08);
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    z-index: 60;
    transition: width 300ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }

  .sidebar-expanded {
    position: fixed;
    width: 260px;
    transform: translateX(0);
  }

  .sidebar-collapsed {
    position: fixed;
    width: 72px;
    transform: translateX(-100%);
  }

  @media (min-width: 1024px) {
    .sidebar-root {
      position: relative;
      z-index: 40; /* Below header on desktop */
    }
    .sidebar-collapsed {
      width: 72px;
      transform: translateX(0);
    }
  }

  /* Navigation */
  .sidebar-nav {
    flex: 1;
    overflow-y: auto;
    padding: 1.5rem 0.75rem 1rem;
  }

  .nav-section-label {
    padding: 1rem 0.75rem 0.25rem;
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #9ca3af;
  }

  .nav-item {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    border-radius: 0.5rem;
    font-size: 0.875rem;
    font-weight: 500;
    position: relative;
    transition: background 150ms ease, color 150ms ease, transform 100ms ease;
  }

  .nav-item:active {
    transform: scale(0.97);
  }

  .nav-item-expanded {
    padding: 0.625rem 0.75rem;
    color: #4b5563;
  }

  .nav-item-expanded:hover {
    background: rgba(0, 0, 0, 0.03);
    color: #1f2937;
  }

  .nav-item-collapsed {
    padding: 0.625rem;
    justify-content: center;
    color: #4b5563;
  }

  .nav-item-collapsed:hover {
    background: rgba(0, 0, 0, 0.03);
  }

  .nav-item-active {
    background: #FDEAEA !important;
    color: #A11217 !important;
  }

  .nav-active-indicator {
    position: absolute;
    left: 0;
    top: 50%;
    transform: translateY(-50%);
    width: 3px;
    height: 1.25rem;
    background: #A11217;
    border-radius: 0 3px 3px 0;
  }

  .nav-icon {
    width: 1.25rem;
    height: 1.25rem;
    flex-shrink: 0;
  }

  .nav-badge {
    margin-left: auto;
    background: #A11217;
    color: white;
    font-size: 0.6875rem;
    font-weight: 600;
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
  }

  /* Footer */
  .sidebar-footer {
    border-top: 1px solid rgba(0, 0, 0, 0.04);
    padding: 1rem;
    padding-bottom: calc(1rem + env(safe-area-inset-bottom));
  }

  .sidebar-collapsed .sidebar-footer {
    padding: 0.5rem;
  }

  .user-card {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem;
    border-radius: 0.5rem;
    cursor: pointer;
    transition: background 150ms ease;
  }

  .user-card:hover {
    background: rgba(0, 0, 0, 0.03);
  }

  .user-card-collapsed {
    justify-content: center;
  }

  .user-avatar {
    width: 2.25rem;
    height: 2.25rem;
    border-radius: 9999px;
    background: #D4AF37;
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: 0.8125rem;
    color: white;
    flex-shrink: 0;
  }

  .user-info {
    flex: 1;
    min-width: 0;
  }

  .user-name {
    font-size: 0.8125rem;
    font-weight: 600;
    color: #1f2937;
  }

  .user-role {
    font-size: 0.6875rem;
    color: #9ca3af;
  }
</style>
