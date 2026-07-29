<script lang="ts">
  import HallMap from '$lib/components/seating/HallMap.svelte';
  import { publicListGuests as listGuests, mapGuest } from '$lib/api/public';
  import { getPublicLayout } from '$lib/api/layout';
  import { cn, getInitials } from '$lib/utils';
  import { formatSeatRange } from '$lib/utils/seat';
  import { Maximize, Minimize, Monitor, Search, ArrowLeft, MapPin, Users, Star } from 'lucide-svelte';
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/state';
  import { setWeddingId } from '$lib/stores/weddingId';
  import type { Guest, BanquetTable, HallElement } from '$lib/types';

  let query = $state('');
  let results = $state<Guest[]>([]);
  let allGuests = $state<Guest[]>([]);
  let tables = $state<BanquetTable[]>([]);
  let elements = $state<HallElement[]>([]);
  let hallWidth = $state(860);
  let hallHeight = $state(1000);
  let selectedGuest = $state<Guest | null>(null);
  let isFullscreen = $state(false);
  let currentTime = $state(new Date());
  let timer: ReturnType<typeof setInterval>;
  let searching = $state(false);
  let abortController: AbortController | null = null;

  // Kiosk customization
  let kioskTitle = $state('Find Your Seat');
  let kioskDescription = $state('Enter your name to find your table and seat');
  let kioskLogoUrl = $state('');
  let kioskBackgroundUrl = $state('');
  let kioskBackgroundBlur = $state(0);
  let kioskBackgroundSize = $state('cover');
  let kioskBackgroundPosX = $state('center');
  let kioskBackgroundPosY = $state('center');
  let showSeatNumbers = $state(true);
  let weddingDate = $state<string>('');

  // Bottom sheet drag state
  let sheetY = $state(0);
  let sheetDragging = $state(false);
  let sheetStartY = $state(0);
  let sheetVelocity = $state(0);
  let sheetLastY = $state(0);
  let sheetLastTime = $state(0);
  let sheetAnimFrame = $state(0);
  let prefersReducedMotion = $state(false);
  let mqHandler: ((e: MediaQueryListEvent) => void) | null = null;

  // Sheet collapsed state (peek mode)
  let sheetCollapsed = $state(false);

  let tableGuests = $derived.by(() => {
    const obj: Record<string, Guest[]> = {};
    for (const g of allGuests) {
      if (g.tableId === null) continue;
      const key = String(g.tableId);
      if (!obj[key]) obj[key] = [];
      obj[key].push(g);
    }
    return obj;
  });

  let selectedTable = $derived(selectedGuest?.tableId ? tables.find(t => t.id === selectedGuest!.tableId) ?? null : null);
  let selectedTableName = $derived(selectedTable?.name ?? selectedGuest?.tableId ?? '—');
  let hasValidTable = $derived(selectedGuest?.tableId != null && selectedTable !== null);

  $effect(() => {
    const q = query.trim();
    if (!q) { results = []; searching = false; return; }
    searching = true;
    let cancelled = false;
    abortController?.abort();
    const controller = new AbortController();
    abortController = controller;
    const timer = setTimeout(async () => {
      try {
        const wid = page.params.wid ?? '';
        const res = await fetch(`/api/public/weddings/${wid}/guests/search?q=${encodeURIComponent(q)}`, { signal: controller.signal });
        if (!res.ok) throw new Error('Search failed');
        const data = await res.json();
        if (!cancelled && !controller.signal.aborted) { results = data.map(mapGuest); searching = false; }
      } catch (e) {
        if (!cancelled && !controller.signal.aborted && !(e instanceof DOMException && e.name === 'AbortError')) { results = []; searching = false; }
      }
    }, 300);
    return () => { cancelled = true; clearTimeout(timer); controller.abort(); };
  });

  onMount(() => {
    // Check reduced motion preference
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)');
    prefersReducedMotion = mq.matches;
    mqHandler = (e: MediaQueryListEvent) => prefersReducedMotion = e.matches;
    mq.addEventListener('change', mqHandler);

    const wid = page.params.wid ?? '';
    if (wid) setWeddingId(wid);
    timer = setInterval(() => currentTime = new Date(), 1000);
    listGuests().then(g => allGuests = g).catch(() => {});
    getPublicLayout(wid).then(l => { tables = l.tables; elements = l.elements; hallWidth = l.hallWidth; hallHeight = l.hallHeight; }).catch(() => {});
    fetch(`/api/public/weddings/${wid}/kiosk`).then(r => r.ok ? r.json() : null).then(data => {
      if (data) {
        if (data.kioskTitle) kioskTitle = data.kioskTitle;
        if (data.kioskDescription) kioskDescription = data.kioskDescription;
        if (data.kioskLogoUrl) kioskLogoUrl = data.kioskLogoUrl;
        if (data.kioskBackgroundUrl) kioskBackgroundUrl = data.kioskBackgroundUrl;
        if (data.kioskBackgroundBlur) kioskBackgroundBlur = data.kioskBackgroundBlur;
        if (data.kioskBackgroundSize) kioskBackgroundSize = data.kioskBackgroundSize;
        if (data.kioskBackgroundPosX) kioskBackgroundPosX = data.kioskBackgroundPosX;
        if (data.kioskBackgroundPosY) kioskBackgroundPosY = data.kioskBackgroundPosY;
        if (data.showSeatNumbers !== undefined) showSeatNumbers = data.showSeatNumbers;
        if (data.date) weddingDate = data.date;
      }
    }).catch(() => {});
  });

  onDestroy(() => {
    clearInterval(timer);
    if (sheetAnimFrame) cancelAnimationFrame(sheetAnimFrame);
    if (mqHandler) {
      const mq = window.matchMedia('(prefers-reduced-motion: reduce)');
      mq.removeEventListener('change', mqHandler);
    }
  });

  function toggleFullscreen() {
    if (!document.fullscreenElement) {
      document.documentElement.requestFullscreen();
      isFullscreen = true;
    } else {
      document.exitFullscreen();
      isFullscreen = false;
    }
  }

  function selectGuest(guest: Guest) {
    selectedGuest = guest;
    query = '';
    sheetCollapsed = false;
    sheetY = 0;
    sheetVelocity = 0;
  }

  function backToSearch() {
    selectedGuest = null;
    query = '';
    sheetCollapsed = false;
    sheetY = 0;
  }

  function formatTime(d: Date) {
    return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
  }

  function formatDate(d: Date) {
    return d.toLocaleDateString('en-US', { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' });
  }

  function formatWeddingDate(dateStr: string) {
    if (!dateStr) return formatDate(new Date());
    const d = new Date(dateStr);
    return isNaN(d.getTime()) ? formatDate(new Date()) : formatDate(d);
  }

  // Spring-like cubic bezier for fluid motion
  // Apple's default: damping 1.0, response 0.3-0.4
  const SPRING_EASE = 'cubic-bezier(0.2, 0.8, 0.2, 1)';

  // Bottom sheet drag handlers (Apple-style interruptible gesture)
  function onSheetPointerDown(e: PointerEvent) {
    if (prefersReducedMotion) return;
    e.stopPropagation();
    sheetDragging = true;
    sheetStartY = e.clientY - sheetY;
    sheetLastY = e.clientY;
    sheetLastTime = performance.now();
    sheetVelocity = 0;
    if (sheetAnimFrame) cancelAnimationFrame(sheetAnimFrame);
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
  }

  function onSheetPointerMove(e: PointerEvent) {
    if (!sheetDragging) return;
    const now = performance.now();
    const dt = now - sheetLastTime;
    const dy = e.clientY - sheetLastY;
    if (dt > 0) sheetVelocity = dy / dt * 16; // normalize to ~frame
    sheetLastY = e.clientY;
    sheetLastTime = now;

    // Rubber-band past the top: resistance increases as you drag further up
    const raw = e.clientY - sheetStartY;
    if (raw > 0) {
      sheetY = raw * 0.4; // rubber-band drag down
    } else {
      sheetY = raw; // free drag up
    }
  }

  function onSheetPointerUp() {
    if (!sheetDragging) return;
    sheetDragging = false;

    const sheetEl = document.querySelector('.bottom-sheet');
    const sheetH = sheetEl ? sheetEl.getBoundingClientRect().height : 200;
    const collapseThreshold = sheetH * 0.35;

    const projected = sheetY + sheetVelocity * 8;

    if (sheetCollapsed) {
      // Currently collapsed — swipe up to expand
      if (projected < -30 || sheetVelocity < -2) {
        sheetCollapsed = false;
        animateSheetTo(0);
      } else {
        animateSheetTo(0);
      }
    } else {
      // Currently expanded — swipe down to collapse
      if (projected > collapseThreshold || sheetVelocity > 2) {
        sheetCollapsed = true;
        animateSheetTo(0);
      } else {
        animateSheetTo(0);
      }
    }
  }

  function animateSheetTo(target: number) {
    if (sheetAnimFrame) cancelAnimationFrame(sheetAnimFrame);
    const start = sheetY;
    const startTime = performance.now();
    const duration = prefersReducedMotion ? 100 : 350;

    function tick() {
      const elapsed = performance.now() - startTime;
      const t = Math.min(1, elapsed / duration);
      // Ease-out with slight overshoot approximation
      const ease = 1 - Math.pow(1 - t, 3);
      sheetY = start + (target - start) * ease;
      if (t < 1) {
        sheetAnimFrame = requestAnimationFrame(tick);
      } else {
        sheetY = target;
        sheetVelocity = 0;
      }
    }
    sheetAnimFrame = requestAnimationFrame(tick);
  }

  // Swipe back gesture on map view (velocity-based dismissal)
  let swipeStartX = $state(0);
  let swipeStartYMap = $state(0);

  function onMapPointerDown(e: PointerEvent) {
    swipeStartX = e.clientX;
    swipeStartYMap = e.clientY;
  }

  function onMapPointerUp(e: PointerEvent) {
    const dx = e.clientX - swipeStartX;
    const dy = e.clientY - swipeStartYMap;
    // Only trigger on horizontal swipe right from left edge
    if (dx > 80 && Math.abs(dy) < 60 && swipeStartX < 60) {
      backToSearch();
    }
  }
</script>

<svelte:head>
  <title>Kiosk – WeddingDB</title>
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover" />
  <meta name="apple-mobile-web-app-capable" content="yes" />
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />
</svelte:head>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="kiosk-root" class:reduced-motion={prefersReducedMotion}>
  <!-- Top Bar — translucent material, Apple-style -->
  <header class="top-bar">
    <div class="top-bar-inner">
      <div class="top-bar-left">
        {#if selectedGuest}
          <button
            class="back-btn"
            onclick={backToSearch}
            aria-label="Back to search"
          >
            <ArrowLeft class="icon-sm" />
          </button>
        {:else}
          <div class="top-bar-brand">
            <Monitor class="icon-sm text-red" />
            <span class="top-bar-title">Find Your Seat</span>
          </div>
        {/if}
      </div>

      <div class="top-bar-right">
        <button class="icon-btn" onclick={toggleFullscreen} aria-label="Toggle fullscreen">
          {#if isFullscreen}
            <Minimize class="icon-sm" />
          {:else}
            <Maximize class="icon-sm" />
          {/if}
        </button>
      </div>
    </div>
  </header>

  <!-- Content area -->
  {#if selectedGuest}
    <!-- Map View -->
    <div
      class="map-view"
      onpointerdown={onMapPointerDown}
      onpointerup={onMapPointerUp}
    >
      <HallMap
        tables={tables}
        {elements}
        {hallWidth}
        {hallHeight}
        selectedTableId={selectedGuest.tableId}
        tableGuests={tableGuests}
        dark={false}
        legendPosition="top-left"
      />

      <!-- Bottom Sheet — translucent material with drag -->
      {#if hasValidTable}
        <div
          class="bottom-sheet"
          class:sheet-dragging={sheetDragging}
          class:sheet-collapsed={sheetCollapsed}
          style="transform: translateY({sheetY}px); transition: {sheetDragging ? 'none' : `transform 350ms ${SPRING_EASE}`};"
        >
          <!-- Drag handle — tap to dismiss, drag to collapse -->
          <div
            class="sheet-handle"
            onpointerdown={(e) => { onSheetPointerDown(e); }}
            onpointermove={onSheetPointerMove}
            onpointerup={(e) => { const wasDragging = sheetDragging; onSheetPointerUp(); if (!wasDragging && Math.abs(sheetY) < 5) backToSearch(); }}
          >
            <div class="handle-bar"></div>
          </div>

          <!-- Guest info -->
          <div class="sheet-content">
            <div class="guest-header">
              <div class={cn(
                "avatar-circle",
                selectedGuest.isVip ? "avatar-vip" : "avatar-default"
              )}>
                {getInitials(selectedGuest.name)}
              </div>
              <div class="guest-info">
                <div class="guest-name-row">
                  <span class="guest-name">{selectedGuest.name}</span>
                  {#if selectedGuest.isVip}
                    <Star class="icon-xs text-gold fill-gold" />
                  {/if}
                </div>
                <div class="guest-meta-row">
                  <span class="guest-table-badge">Table {selectedTableName}</span>
                  <span class="guest-pax">{selectedGuest.pax} pax</span>
                  {#if selectedGuest.rsvp && selectedGuest.rsvp !== 'no_response'}
                    <span class="guest-rsvp-badge rsvp-{selectedGuest.rsvp}">{selectedGuest.rsvp}</span>
                  {/if}
                </div>
              </div>
            </div>

            {#if !sheetCollapsed}
              <div class="seat-display">
                <div class="seat-block">
                  <div class="seat-label">Table</div>
                  <div class="seat-value seat-value-large">{selectedTableName}</div>
                  {#if selectedTable?.isVip}
                    <span class="vip-tag">★ VIP</span>
                  {/if}
                </div>
                {#if showSeatNumbers}
                  <div class="seat-divider"></div>
                  <div class="seat-block">
                    <div class="seat-label">Seats</div>
                    <div class="seat-value">{formatSeatRange(selectedGuest.seatNumber, selectedGuest.pax)}</div>
                  </div>
                {/if}
              </div>

              <div class="sheet-hint">
                <MapPin class="icon-xs" />
                <span>Look for the highlighted table on the map</span>
              </div>
            {/if}
          </div>
        </div>
      {:else}
        <!-- No table assigned -->
        <div class="bottom-sheet" style="transform: translateY({sheetY}px);">
          <div class="sheet-handle" onpointerdown={onSheetPointerDown} onpointermove={onSheetPointerMove} onpointerup={onSheetPointerUp}>
            <div class="handle-bar"></div>
          </div>
          <div class="sheet-content sheet-empty">
            <MapPin class="icon-lg text-gray-400" />
            <h3 class="font-bold text-gray-900 mb-1">No Seat Assigned</h3>
            <p class="text-sm text-gray-500">Please see the reception desk for seating.</p>
          </div>
        </div>
      {/if}
    </div>

  {:else}
    <!-- Search View -->
    <div class="search-view">
      {#if kioskBackgroundUrl}
        <div class="search-bg" style={`background-image: url(${kioskBackgroundUrl}); background-size: ${kioskBackgroundSize}; background-position: ${kioskBackgroundPosX} ${kioskBackgroundPosY}; filter: blur(${kioskBackgroundBlur}px);`}></div>
        <div class="search-bg-overlay"></div>
      {/if}

      <div class="search-content">
        <div class="search-hero">
          {#if kioskLogoUrl}
            <img src={kioskLogoUrl} alt="Logo" class="hero-logo" />
          {:else}
            <div class="hero-icon">囍</div>
          {/if}
          {#if weddingDate}
            <p class="hero-date">{formatWeddingDate(weddingDate)}</p>
          {/if}
          <h1 class="hero-title">{kioskTitle}</h1>
          {#if kioskDescription}
            <p class="hero-subtitle">{kioskDescription}</p>
          {/if}
        </div>

        <!-- Search Input — large touch target -->
        <div class="search-input-wrap">
          <Search class="search-icon" />
          <input
            type="text"
            placeholder="Type your name..."
            bind:value={query}
            class="search-input"
            autofocus
          />
        </div>

        <!-- Results -->
        {#if results.length > 0}
          <div class="results-list">
            {#each results.slice(0, 8) as guest, i (guest.id)}
              <button
                class="result-card"
                style="animation-delay: {prefersReducedMotion ? '0ms' : `${i * 40}ms`}"
                onclick={() => selectGuest(guest)}
              >
                <div class={cn(
                  "result-avatar",
                  guest.isVip ? "avatar-vip" : "avatar-default"
                )}>
                  {getInitials(guest.name)}
                </div>
                <div class="result-info">
                  <div class="result-name-row">
                    <span class="result-name">{guest.name}</span>
                    {#if guest.isVip}
                      <Star class="icon-xs text-gold fill-gold" />
                    {/if}
                  </div>
                  <div class="result-meta">
                    {#if guest.tableId}
                      <span class="meta-item"><MapPin class="icon-xs" />Table {tables.find(t => t.id === guest.tableId)?.name ?? guest.tableId}</span>
                      {#if showSeatNumbers}<span class="meta-item">{formatSeatRange(guest.seatNumber, guest.pax)}</span>{/if}
                    {:else}
                      <span class="meta-item text-gray-400">No seat assigned</span>
                    {/if}
                    <span class="meta-item">{guest.pax} pax</span>
                  </div>
                </div>
                <div class="result-chevron">
                  <svg class="icon-sm" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>
                </div>
              </button>
            {/each}
          </div>
        {:else if query.trim().length > 0 && !searching}
          <div class="empty-state">
            <Search class="icon-xl opacity-30" />
            <p class="font-medium">No guests found</p>
            <p class="text-sm mt-1 text-gray-400">Try a different spelling</p>
          </div>
        {:else if searching}
          <div class="empty-state">
            <div class="spinner"></div>
            <p class="font-medium text-gray-500">Searching...</p>
          </div>
        {:else}
          <div class="empty-state">
            <Users class="icon-lg opacity-30" />
            <p class="text-sm text-gray-400">Start typing to search</p>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  /* ---------- Root ---------- */
  .kiosk-root {
    height: 100vh;
    height: 100dvh;
    overflow: hidden;
    background: linear-gradient(180deg, #fef2f2 0%, #faf7f2 50%, white 100%);
    background-attachment: fixed;
    color: #111827;
    display: flex;
    flex-direction: column;
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, system-ui, sans-serif;
    -webkit-font-smoothing: antialiased;
    -webkit-tap-highlight-color: transparent;
  }

  /* ---------- Reduced Motion ---------- */
  .kiosk-root.reduced-motion * {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.15s !important;
  }

  /* ---------- Top Bar ---------- */
  .top-bar {
    position: relative;
    z-index: 20;
    background: white;
    border-bottom: 1px solid #e5e7eb;
    flex-shrink: 0;
    padding-top: env(safe-area-inset-top, 0px);
  }

  .top-bar-inner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 1rem;
    gap: 0.75rem;
  }

  @media (min-width: 640px) {
    .top-bar-inner {
      padding: 0.875rem 2rem;
    }
  }

  .top-bar-left {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
  }

  .top-bar-center {
    flex-shrink: 0;
    text-align: center;
  }

  .top-bar-right {
    flex: 1;
    display: flex;
    justify-content: flex-end;
  }

  .top-bar-brand {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .top-bar-title {
    font-weight: 600;
    font-size: 0.875rem;
    color: #111827;
    letter-spacing: -0.01em;
  }

  /* ---------- Buttons — Press Feedback ---------- */
  .back-btn, .icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2.5rem;
    height: 2.5rem;
    border-radius: 0.75rem;
    background: rgba(0, 0, 0, 0.04);
    color: #4b5563;
    transition: background 100ms ease, transform 100ms ease, color 100ms ease;
    flex-shrink: 0;
    /* Large touch target */
    min-width: 44px;
    min-height: 44px;
  }

  .back-btn:active, .icon-btn:active {
    transform: scale(0.92);
    background: rgba(0, 0, 0, 0.08);
  }

  .back-btn:hover, .icon-btn:hover {
    background: rgba(0, 0, 0, 0.06);
    color: #111827;
  }



  /* ---------- Map View ---------- */
  .map-view {
    flex: 1;
    display: flex;
    flex-direction: column;
    position: relative;
    overflow: hidden;
  }

  /* ---------- Bottom Sheet — Draggable Translucent Material ---------- */
  .bottom-sheet {
    position: absolute;
    bottom: 0;
    left: 0.5rem;
    right: 0.5rem;
    z-index: 30;
    /* Translucent material */
    background: rgba(255, 255, 255, 0.88);
    backdrop-filter: blur(16px) saturate(200%);
    -webkit-backdrop-filter: blur(16px) saturate(200%);
    border: 1px solid rgba(255, 255, 255, 0.6);
    padding-bottom: env(safe-area-inset-bottom, 0px);
    border-radius: 1.25rem 1.25rem 1rem 1rem;
    box-shadow:
      0 -4px 24px rgba(0, 0, 0, 0.08),
      0 -1px 4px rgba(0, 0, 0, 0.04),
      inset 0 1px 0 rgba(255, 255, 255, 0.9);
    overflow: hidden;
    /* Apple: max height for bottom sheet */
    max-height: 45vh;
    touch-action: none;
  }

  @media (min-width: 640px) {
    .bottom-sheet {
      left: auto;
      right: 1rem;
      bottom: 1rem;
      width: 22rem;
      max-height: none;
      border-radius: 1.25rem;
    }
  }

  .sheet-dragging {
    transition: none !important;
  }

  .sheet-collapsed {
    max-height: 5.5rem;
    overflow: hidden;
    transition: max-height 350ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }

  .sheet-collapsed .sheet-content {
    padding-bottom: 1rem;
  }

  .sheet-collapsed .sheet-content > :not(.guest-header) {
    opacity: 0;
    pointer-events: none;
  }

  .sheet-collapsed .guest-header {
    padding-bottom: 0;
  }



  .sheet-handle {
    display: flex;
    justify-content: center;
    padding: 0.5rem 0 0.25rem;
    cursor: grab;
    touch-action: none;
  }

  .sheet-handle:active {
    cursor: grabbing;
  }

  .handle-bar {
    width: 2.5rem;
    height: 0.25rem;
    border-radius: 9999px;
    background: rgba(0, 0, 0, 0.12);
  }

  .sheet-content {
    padding: 0.25rem 1.25rem calc(1.25rem + env(safe-area-inset-bottom));
  }

  .sheet-empty {
    text-align: center;
    padding: 2rem 1.25rem;
  }

  /* ---------- Guest Header in Sheet ---------- */
  .guest-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding-bottom: 0.75rem;
    border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  }

  .avatar-circle {
    width: 2.75rem;
    height: 2.75rem;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: 0.875rem;
    flex-shrink: 0;
  }

  .avatar-default {
    background: #FDEAEA;
    color: #A11217;
    border: 2px solid #FAC5C5;
  }

  .avatar-vip {
    background: #FDF8E8;
    color: #B8941F;
    border: 2px solid #E8CC6E;
  }

  .guest-info {
    flex: 1;
    min-width: 0;
  }

  .guest-name-row {
    display: flex;
    align-items: center;
    gap: 0.375rem;
  }

  .guest-name {
    font-weight: 600;
    color: #111827;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .guest-meta-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.125rem;
  }

  .guest-table-badge {
    font-size: 0.6875rem;
    font-weight: 700;
    color: #A11217;
    background: #FDEAEA;
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
    line-height: 1.4;
  }

  @media (min-width: 640px) {
    .guest-table-badge {
      display: none;
    }
  }

  .guest-pax {
    font-size: 0.75rem;
    color: #6b7280;
  }

  .guest-rsvp-badge {
    font-size: 0.625rem;
    font-weight: 600;
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
    text-transform: capitalize;
  }
  .guest-rsvp-badge:where(:global(.rsvp-confirmed)) { background: #ECFDF5; color: #059669; border: 1px solid #A7F3D0; }
  .guest-rsvp-badge:where(:global(.rsvp-pending)) { background: #FFFBEB; color: #D97706; border: 1px solid #FDE68A; }
  .guest-rsvp-badge:where(:global(.rsvp-declined)) { background: #FEF2F2; color: #DC2626; border: 1px solid #FECACA; }
  .guest-rsvp-badge:where(:global(.rsvp-no_response)) { background: #F9FAFB; color: #6B7280; border: 1px solid #E5E7EB; }

  /* ---------- Seat Display ---------- */
  .seat-display {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 1.5rem;
    padding: 1rem 0;
  }

  @media (min-width: 640px) {
    .seat-display {
      gap: 2rem;
    }
  }

  .seat-block {
    text-align: center;
  }

  .seat-label {
    font-size: 0.625rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: #9ca3af;
    font-weight: 600;
    margin-bottom: 0.25rem;
  }

  .seat-value {
    font-size: 1.25rem;
    font-weight: 800;
    color: #111827;
    letter-spacing: -0.02em;
  }

  .seat-value-large {
    font-size: 2rem;
    color: #A11217;
  }

  @media (min-width: 640px) {
    .seat-value-large {
      font-size: 2.5rem;
    }
  }

  .vip-tag {
    font-size: 0.625rem;
    color: #B8941F;
    font-weight: 600;
  }

  .seat-divider {
    width: 1px;
    height: 2.5rem;
    background: rgba(0, 0, 0, 0.08);
  }

  .sheet-hint {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.25rem;
    font-size: 0.6875rem;
    color: #9ca3af;
    padding-top: 0.25rem;
  }

  /* Hide hint on desktop — map is always visible */
  @media (min-width: 640px) {
    .sheet-hint {
      display: none;
    }
  }

  /* ---------- Search View ---------- */
  .search-view {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 1.5rem;
    position: relative;
    overflow-y: auto;
    overflow-x: hidden;
    -webkit-overflow-scrolling: touch;
  }

  @media (min-width: 640px) {
    .search-view {
      padding: 2rem;
    }
  }

  .search-bg {
    position: absolute;
    inset: 0;
    transform: scale(1.05);
    pointer-events: none;
  }

  .search-content {
    width: 100%;
    max-width: 32rem;
    text-align: center;
    position: relative;
    z-index: 10;
    /* Center vertically when content is short, scroll when tall */
    margin: auto 0;
    padding: 2rem 0;
  }

  /* ---------- Hero ---------- */
  .search-hero {
    margin-bottom: 2rem;
  }

  @media (min-width: 640px) {
    .search-hero {
      margin-bottom: 2.5rem;
    }
  }

  .hero-logo {
    width: 7rem;
    height: 7rem;
    margin: 0 auto 1rem;
    border-radius: 1.25rem;
    object-fit: cover;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.12);
  }

  @media (min-width: 640px) {
    .hero-logo {
      width: 8rem;
      height: 8rem;
      margin-bottom: 1.5rem;
    }
  }

  .hero-icon {
    width: 7rem;
    height: 7rem;
    margin: 0 auto 1rem;
    border-radius: 1.25rem;
    background: #A11217;
    display: flex;
    align-items: center;
    justify-content: center;
    color: #D4AF37;
    font-size: 2.5rem;
    font-weight: 700;
    font-family: 'Noto Serif SC', 'Songti SC', serif;
    box-shadow: 0 8px 32px rgba(161, 18, 23, 0.3);
  }

  @media (min-width: 640px) {
    .hero-icon {
      width: 8rem;
      height: 8rem;
      font-size: 3rem;
      margin-bottom: 1.5rem;
    }
  }

  .hero-title {
    font-size: 1.875rem;
    font-weight: 800;
    color: #111827;
    letter-spacing: -0.025em;
    margin-bottom: 0.5rem;
  }

  @media (min-width: 640px) {
    .hero-title {
      font-size: 2.5rem;
    }
  }

  .hero-date {
    font-size: 0.875rem;
    font-weight: 500;
    color: #9ca3af;
    letter-spacing: 0.04em;
    margin-bottom: 0.5rem;
    text-transform: uppercase;
  }

  @media (min-width: 640px) {
    .hero-date {
      font-size: 1rem;
    }
  }

  .hero-subtitle {
    color: #6b7280;
    font-size: 0.9375rem;
  }

  .search-bg-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.3);
    pointer-events: none;
    z-index: 1;
  }

  /* ---------- Search Input ---------- */
  .search-input-wrap {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 2rem;
    background: rgba(255, 255, 255, 0.9);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1.5px solid rgba(0, 0, 0, 0.08);
    border-radius: 1.25rem;
    padding: 0 1.25rem;
    transition: border-color 200ms ease, box-shadow 200ms ease;
  }

  .search-input-wrap:focus-within {
    border-color: #A11217;
    box-shadow: 0 0 0 3px rgba(161, 18, 23, 0.1), 0 4px 16px rgba(0, 0, 0, 0.06);
  }

  .search-input-wrap:active {
    transform: scale(0.98);
    transition: transform 100ms ease;
  }

  .search-icon {
    width: 1.25rem;
    height: 1.25rem;
    color: #9ca3af;
    flex-shrink: 0;
  }

  .search-input {
    flex: 1;
    border: none;
    background: transparent;
    padding: 1rem 0;
    font-size: 1.125rem;
    line-height: 1.5;
    color: #111827;
    outline: none;
    min-width: 0;
  }

  @media (min-width: 640px) {
    .search-input {
      font-size: 1.25rem;
    }
  }

  .search-input::placeholder {
    color: #9ca3af;
  }

  /* ---------- Results ---------- */
  .results-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    text-align: left;
  }

  .result-card {
    display: flex;
    align-items: center;
    gap: 1rem;
    background: rgba(255, 255, 255, 0.9);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid rgba(0, 0, 0, 0.06);
    border-radius: 1rem;
    padding: 1rem;
    text-align: left;
    width: 100%;
    transition: transform 150ms ease, border-color 150ms ease, box-shadow 150ms ease;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
    /* Staggered entrance animation */
    animation: resultEnter 350ms cubic-bezier(0.2, 0.8, 0.2, 1) both;
  }

  .result-card:active {
    transform: scale(0.96);
  }

  .result-card:hover {
    border-color: rgba(161, 18, 23, 0.2);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.06);
  }

  @keyframes resultEnter {
    from {
      opacity: 0;
      transform: translateY(8px) scale(0.98);
    }
    to {
      opacity: 1;
      transform: translateY(0) scale(1);
    }
  }

  .result-avatar {
    width: 3rem;
    height: 3rem;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: 1rem;
    flex-shrink: 0;
  }

  .result-info {
    flex: 1;
    min-width: 0;
  }

  .result-name-row {
    display: flex;
    align-items: center;
    gap: 0.375rem;
  }

  .result-name {
    font-weight: 600;
    color: #111827;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    transition: color 150ms ease;
  }

  .result-card:hover .result-name {
    color: #A11217;
  }

  .result-meta {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-top: 0.125rem;
    flex-wrap: wrap;
  }

  .meta-item {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    font-size: 0.8125rem;
    color: #6b7280;
  }

  .result-chevron {
    color: #d1d5db;
    flex-shrink: 0;
    transition: color 150ms ease, transform 150ms ease;
  }

  .result-card:hover .result-chevron {
    color: #A11217;
    transform: translateX(2px);
  }

  /* ---------- Empty State ---------- */
  .empty-state {
    text-align: center;
    padding: 2rem 0;
    color: #9ca3af;
  }

  /* ---------- Spinner ---------- */
  .spinner {
    width: 2rem;
    height: 2rem;
    border: 2.5px solid rgba(161, 18, 23, 0.2);
    border-top-color: #A11217;
    border-radius: 50%;
    margin: 0 auto 0.75rem;
    animation: spin 600ms linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  /* ---------- Icon Sizes ---------- */
  :global(.icon-xs) {
    width: 0.875rem;
    height: 0.875rem;
    flex-shrink: 0;
  }

  :global(.icon-sm) {
    width: 1.125rem;
    height: 1.125rem;
    flex-shrink: 0;
  }

  :global(.icon-lg) {
    width: 2rem;
    height: 2rem;
    flex-shrink: 0;
    display: block;
    margin: 0 auto 0.75rem;
  }

  :global(.icon-xl) {
    width: 3rem;
    height: 3rem;
    flex-shrink: 0;
    display: block;
    margin: 0 auto 0.75rem;
  }

  /* ---------- Utility ---------- */
  .text-red { color: #A11217; }
  .text-gold { color: #D4AF37; }
  .fill-gold { fill: #D4AF37; }
  .text-gray-400 { color: #9ca3af; }
  .font-bold { font-weight: 700; }
  .font-medium { font-weight: 500; }
  .mx-auto { margin-left: auto; margin-right: auto; }
  .mb-1 { margin-bottom: 0.25rem; }
  .mb-3 { margin-bottom: 0.75rem; }
  .mt-1 { margin-top: 0.25rem; }
  .opacity-30 { opacity: 0.3; }

  /* ---------- Reduced Motion ---------- */
  @media (prefers-reduced-motion: reduce) {
    .result-card {
      animation: none;
    }

    .search-input-wrap,
    .result-card,
    .back-btn,
    .icon-btn {
      transition: none;
    }

    .sheet-collapsed {
      transition: none;
    }

    .spinner {
      animation: spin 1200ms linear infinite;
    }
  }
</style>
