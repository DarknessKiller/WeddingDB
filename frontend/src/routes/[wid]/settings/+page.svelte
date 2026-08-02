<script lang="ts">
  import { onMount } from 'svelte';
  import { weddingTitle } from '$lib/stores/weddingTitle';
  import { addToast } from '$lib/stores';
  import { weddingId } from '$lib/stores/weddingId';
  import { get } from 'svelte/store';
  import { getWedding, updateKioskSettings, updateWedding, type Wedding, type KioskSettings } from '$lib/api/weddings';
  import { Monitor, Save, Loader2, ExternalLink, Copy, Image, Type, FileText, Upload, MapPin } from 'lucide-svelte';
  import ImageEditor from '$lib/components/ui/ImageEditor.svelte';
  import { uploadFile } from '$lib/api/client';

  let wedding = $state<Wedding | null>(null);
  let loading = $state(true);
  let saving = $state(false);

  let kioskDescription = $state('');
  let kioskLogoUrl = $state('');
  let kioskBackgroundUrl = $state('');
  let kioskBackgroundBlur = $state(0);
  let kioskBackgroundSize = $state('cover');
  let kioskBackgroundPosX = $state('50%');
  let kioskBackgroundPosY = $state('50%');
  let kioskLogoSize = $state('contain');
  let kioskLogoPosX = $state('50%');
  let kioskLogoPosY = $state('50%');
  let kioskLogoBlur = $state(0);
  let weddingNameEdit = $state('');
  let showSeatNumbers = $state(true);
  let venueName = $state('');
  let venueAddress = $state('');

  onMount(async () => {
    try {
      const wid = get(weddingId);
      wedding = await getWedding(wid).catch(() => null);
      if (wedding) {
        weddingNameEdit = wedding?.name ?? '';
        kioskDescription = (wedding as any).kioskDescription ?? '';
        kioskLogoUrl = (wedding as any).kioskLogoUrl ?? '';
        kioskBackgroundUrl = (wedding as any).kioskBackgroundUrl ?? '';
        kioskBackgroundBlur = (wedding as any).kioskBackgroundBlur ?? 0;
        if ((wedding as any).kioskBackgroundSize) kioskBackgroundSize = (wedding as any).kioskBackgroundSize;
        if ((wedding as any).kioskBackgroundPosX) kioskBackgroundPosX = (wedding as any).kioskBackgroundPosX;
        if ((wedding as any).kioskBackgroundPosY) kioskBackgroundPosY = (wedding as any).kioskBackgroundPosY;
        if ((wedding as any).kioskLogoSize) kioskLogoSize = (wedding as any).kioskLogoSize;
        if ((wedding as any).kioskLogoPosX) kioskLogoPosX = (wedding as any).kioskLogoPosX;
        if ((wedding as any).kioskLogoPosY) kioskLogoPosY = (wedding as any).kioskLogoPosY;
        showSeatNumbers = (wedding as any).showSeatNumbers ?? true;
        venueName = (wedding as any).venueName ?? '';
        venueAddress = (wedding as any).venueAddress ?? '';
      }
    } catch {
      addToast('Failed to load settings', 'error');
    } finally {
      loading = false;
    }
  });

  async function handleSave() {
    if (!wedding) return;
    saving = true;
    try {
      await updateKioskSettings(wedding.id, {
        venueName,
        venueAddress,
        kioskDescription,
        kioskLogoUrl,
        kioskBackgroundUrl,
        kioskBackgroundBlur,
        kioskBackgroundSize,
        kioskBackgroundPosX,
        kioskBackgroundPosY,
        kioskLogoSize,
        kioskLogoPosX,
        kioskLogoPosY,
        showSeatNumbers,
      });
      await updateWedding(wedding.id, { name: weddingNameEdit, date: wedding.date.split('T')[0] });
      addToast('Kiosk settings saved', 'success');
    } catch (e: any) {
      addToast(e.message ?? 'Failed to save', 'error');
    } finally {
      saving = false;
    }
  }

  async function copyKioskLink() {
    if (!wedding) return;
    const url = `${window.location.origin}/kiosk/${wedding.id}`;
    try {
      await navigator.clipboard.writeText(url);
      addToast('Kiosk link copied!', 'success');
    } catch {
      addToast('Failed to copy', 'error');
    }
  }
</script>

<svelte:head> <title>{$weddingTitle ? `${$weddingTitle} – Settings` : 'Settings – WeddingDB'}</title></svelte:head>


<div class="p-4 sm:p-7 max-w-2xl mx-auto">
  <h1 class="text-xl font-bold text-gray-900 mb-6" style="letter-spacing: -0.02em;">Settings</h1>

  {#if loading}
    <div class="space-y-4">
      {#each Array(2) as _}
        <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl p-6 animate-pulse">
          <div class="h-5 bg-gray-100 rounded w-40 mb-4"></div>
          <div class="space-y-3">
            <div class="h-10 bg-gray-100 rounded-xl"></div>
            <div class="h-20 bg-gray-100 rounded-xl"></div>
          </div>
        </div>
      {/each}
    </div>
  {:else}
    <div class="space-y-6">
      <!-- Kiosk Quick Access -->
      <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl p-5 shadow-sm">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 rounded-xl bg-red flex items-center justify-center text-white"><Monitor class="w-5 h-5" /></div>
            <div>
              <h3 class="font-semibold text-gray-900 text-sm">Kiosk Mode</h3>
              <p class="text-xs text-gray-500">Guest seat-finding kiosk screen</p>
            </div>
          </div>
          <div class="flex gap-2">
            <button onclick={copyKioskLink} class="px-3 py-2 text-xs font-semibold bg-gray-100 hover:bg-gray-200 text-gray-700 rounded-xl transition-colors flex items-center gap-1.5">
              <Copy class="w-3.5 h-3.5" /> Copy Link
            </button>
            <a href="/kiosk/{wedding ? wedding.id : ''}" target="_blank"
              class="px-3 py-2 text-xs font-semibold bg-red text-white rounded-xl hover:bg-red-light transition-colors flex items-center gap-1.5">
              <ExternalLink class="w-3.5 h-3.5" /> Preview
            </a>
          </div>
        </div>
      </div>

      <!-- Kiosk Customization -->
      <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl p-5 sm:p-6 shadow-sm">
        <h3 class="font-bold text-gray-900 mb-5" style="letter-spacing: -0.01em;">Kiosk Display Settings</h3>

        <div class="space-y-5">
          <!-- Wedding Name -->
          <div>
            <label for="wedding-name" class="flex items-center gap-1.5 text-sm font-semibold text-gray-700 mb-1.5">
              <Type class="w-4 h-4 text-gray-400" /> Wedding Name
            </label>
            <input id="wedding-name" type="text" bind:value={weddingNameEdit}
              placeholder="e.g. Sarah & John's Wedding"
              class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-red focus:ring-2 focus:ring-red/15 outline-none transition-all" />
            <p class="text-xs text-gray-400 mt-1">Name displayed in the kiosk header</p>
          </div>

          <!-- Venue Name -->
          <div>
            <label for="venue-name" class="flex items-center gap-1.5 text-sm font-semibold text-gray-700 mb-1.5">
              <MapPin class="w-4 h-4 text-gray-400" /> Venue Name
            </label>
            <input id="venue-name" type="text" bind:value={venueName}
              placeholder="e.g. Grand Ballroom"
              class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-red focus:ring-2 focus:ring-red/15 outline-none transition-all" />
            <p class="text-xs text-gray-400 mt-1">Name of the venue or hall</p>
          </div>

          <!-- Venue Address -->
          <div>
            <label for="venue-address" class="flex items-center gap-1.5 text-sm font-semibold text-gray-700 mb-1.5">
              <MapPin class="w-4 h-4 text-gray-400" /> Venue Address
            </label>
            <input id="venue-address" type="text" bind:value={venueAddress}
              placeholder="e.g. 123 Main Street, City"
              class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-red focus:ring-2 focus:ring-red/15 outline-none transition-all" />
            <p class="text-xs text-gray-400 mt-1">Address shown on the kiosk screen</p>
          </div>

          <!-- Description -->
          <div>
            <label for="kiosk-desc" class="flex items-center gap-1.5 text-sm font-semibold text-gray-700 mb-1.5">
              <FileText class="w-4 h-4 text-gray-400" /> Description
            </label>
            <textarea id="kiosk-desc" bind:value={kioskDescription} rows="2"
              placeholder="e.g. Enter your name to find your table and seat"
              class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-red focus:ring-2 focus:ring-red/15 outline-none transition-all resize-none"></textarea>
          </div>

          <!-- Logo -->
          <div>
            <label class="text-sm font-semibold text-gray-700 mb-1.5 block">Logo</label>
            <ImageEditor
              bind:value={kioskLogoUrl}
              bind:size={kioskLogoSize}
              bind:posX={kioskLogoPosX}
              bind:posY={kioskLogoPosY}
              bind:blur={kioskLogoBlur}
              label="Logo"
              aspect="square"
            />
          </div>

          <!-- Background -->
          <div>
            <label class="text-sm font-semibold text-gray-700 mb-1.5 block">Background Image</label>
            <ImageEditor
              bind:value={kioskBackgroundUrl}
              bind:size={kioskBackgroundSize}
              bind:posX={kioskBackgroundPosX}
              bind:posY={kioskBackgroundPosY}
              bind:blur={kioskBackgroundBlur}
              label="Background"
            />
          </div>
        </div>

        <!-- Save -->
        <div class="mt-6 flex justify-end">
          <button onclick={handleSave} disabled={saving}
            class="px-5 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors disabled:opacity-50 flex items-center gap-2">
            {#if saving}
              <Loader2 class="w-4 h-4 text-white animate-spin" /> Saving...
            {:else}
              <Save class="w-4 h-4" /> Save Settings
            {/if}
          </button>
        </div>
      </div>

      <!-- Seat Numbers Toggle -->
      <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl p-5 shadow-sm">
        <div class="flex items-center justify-between py-1">
          <div>
            <p class="text-sm font-semibold text-gray-900">Show Seat Numbers</p>
            <p class="text-xs text-gray-500">Display individual seat numbers on the kiosk and check-in screens</p>
          </div>
          <button onclick={() => { showSeatNumbers = !showSeatNumbers; }}
            class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors {showSeatNumbers ? 'bg-deep-red' : 'bg-gray-200'}">
            <span class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform {showSeatNumbers ? 'translate-x-6' : 'translate-x-1'}" />
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>
