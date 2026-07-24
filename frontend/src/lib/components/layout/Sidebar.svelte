<script lang="ts">
  import { cn } from '$lib/utils';
  import { sidebarCollapsed, getAuth, clearAuth } from '$lib/stores';
  import { setWeddingId } from '$lib/stores/weddingId';
  import { goto } from '$app/navigation';
  import {
    LayoutDashboard, Users, MapPin, Search, Monitor, Calendar,
    Settings, BarChart3, Utensils, Menu, X, LogOut, Shield
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
        ...(isAdmin ? [{ href: `/${wid}/admins`, label: 'Users', icon: Shield }] : []),
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

  async function handleLogout() {
    const { refreshToken } = getAuth();
    if (refreshToken) {
      try {
        await fetch('/api/auth/logout', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refreshToken })
        });
      } catch { /* ignore */ }
    }
    clearAuth();
    setWeddingId('');
    goto('/login', { replaceState: true });
  }
</script>

<!-- Mobile hamburger button -->
<button
  onclick={() => $sidebarCollapsed = !$sidebarCollapsed}
  class="fixed top-4 left-4 z-50 lg:hidden p-2 bg-white rounded-xl shadow-lg border border-gray-200 hover:bg-gray-50 transition-colors"
  aria-label="Toggle menu"
>
  {#if $sidebarCollapsed}
    <Menu class="w-5 h-5 text-gray-700" />
  {:else}
    <X class="w-5 h-5 text-gray-700" />
  {/if}
</button>

<aside class={cn(
  "h-screen bg-white border-r border-gray-200 flex flex-col flex-shrink-0 z-40 transition-all duration-300 ease-in-out",
  "fixed lg:relative",
  $sidebarCollapsed ? "-translate-x-full lg:translate-x-0 lg:w-[72px]" : "translate-x-0 w-[260px]"
)}>
  <!-- Brand -->
  <div class={cn(
    "border-b border-gray-100 flex items-center gap-3",
    $sidebarCollapsed ? "px-3 py-5 justify-center" : "px-6 py-5"
  )}>
    <div class="w-9 h-9 bg-red rounded-lg flex items-center justify-center text-gold font-bold font-serif text-lg flex-shrink-0">
      囍
    </div>
    {#if !$sidebarCollapsed}
      <h1 class="text-lg font-bold text-gray-900 tracking-tight whitespace-nowrap">
        Wedding<span class="text-gold">DB</span>
      </h1>
    {/if}
  </div>

  <!-- Navigation -->
  <nav class="flex-1 overflow-y-auto p-3">
    {#each navSections as section}
      {#if !$sidebarCollapsed}
        <div class="px-3 pt-4 pb-1 text-[11px] font-semibold uppercase tracking-wider text-gray-400">
          {section.label}
        </div>
      {/if}
      {#each section.items as item}
        <a
          href={item.href}
          onclick={closeOnMobile}
          title={$sidebarCollapsed ? item.label : undefined}
          class={cn(
            'flex items-center gap-2.5 rounded-lg text-sm font-medium transition-all duration-150 relative',
            $sidebarCollapsed ? 'px-0 py-2.5 justify-center' : 'px-3 py-2.5',
            currentPath === item.href
              ? 'bg-red-50 text-red'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-800'
          )}
        >
          {#if currentPath === item.href}
            <div class="absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-5 bg-red rounded-r"></div>
          {/if}
          <item.icon class="w-5 h-5 flex-shrink-0" />
          {#if !$sidebarCollapsed}
            {item.label}
            {#if item.href === '/guests' && guestCount > 0}
              <span class="ml-auto bg-red text-white text-[11px] font-semibold px-2 py-0.5 rounded-full">
                {guestCount}
              </span>
            {/if}
          {/if}
        </a>
      {/each}
    {/each}
  </nav>

  <!-- Footer -->
  <div class={cn("border-t border-gray-100", $sidebarCollapsed ? "p-2" : "p-4")}>
    <div class={cn(
      "flex items-center gap-3 rounded-lg hover:bg-gray-50 cursor-pointer transition-colors",
      $sidebarCollapsed ? "justify-center py-2" : "px-2 py-2"
    )}>
      <div class="w-9 h-9 rounded-full bg-gold flex items-center justify-center text-white text-sm font-bold flex-shrink-0">
        {initials}
      </div>
      {#if !$sidebarCollapsed}
        <div class="flex-1 min-w-0">
          <div class="text-[13px] font-semibold text-gray-800">{displayName}</div>
          <div class="text-[11px] text-gray-400">{auth.role || 'Administrator'}</div>
        </div>
        <button onclick={handleLogout} class="p-1.5 rounded-lg hover:bg-gray-100 text-gray-400 hover:text-red transition-colors" aria-label="Logout" title="Logout">
          <LogOut class="w-4 h-4" />
        </button>
      {:else}
        <button onclick={handleLogout} class="p-1.5 rounded-lg hover:bg-gray-100 text-gray-400 hover:text-red transition-colors mt-1" aria-label="Logout" title="Logout">
          <LogOut class="w-4 h-4" />
        </button>
      {/if}
    </div>
  </div>
</aside>
