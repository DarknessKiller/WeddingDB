<script lang="ts">
  import { cn } from '#lib/utils/index.ts';
  import { sidebarCollapsed, getAuth } from '#lib/stores/index.ts';
  import LayoutDashboard from 'lucide-svelte/icons/layout-dashboard';
import Users from 'lucide-svelte/icons/users';
import MapPin from 'lucide-svelte/icons/map-pin';
import Search from 'lucide-svelte/icons/search';
import Calendar from 'lucide-svelte/icons/calendar';
import Settings from 'lucide-svelte/icons/settings';
import Utensils from 'lucide-svelte/icons/utensils';
import Shield from 'lucide-svelte/icons/shield';
import FileDown from 'lucide-svelte/icons/file-down';

  let { currentPath, guestCount = 0, wid = '' }: { currentPath: string; guestCount?: number; wid?: string } = $props();

  const auth = $derived(getAuth());
  const isAdmin = $derived(auth.role === 'admin');

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
        { href: `/${wid}/reports`, label: 'Reports', icon: FileDown },
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
            {#if item.href === `/${wid}/guests` && guestCount > 0}
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
    color: #6b7280;
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
    /* Indicator grows in on route change: state indication, not decoration */
    animation: indicatorIn 200ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }

  @keyframes indicatorIn {
    from { transform: translateY(-50%) scaleY(0); opacity: 0; }
    to { transform: translateY(-50%) scaleY(1); opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .nav-active-indicator { animation: none; }
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

</style>
