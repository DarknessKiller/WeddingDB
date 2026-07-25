<script lang="ts">
  import { Upload, X, Move, Maximize2 } from 'lucide-svelte';
  import { uploadFile } from '$lib/api/client';
  import { addToast } from '$lib/stores';

  let {
    value = $bindable(''),
    size = $bindable('cover'),
    posX = $bindable('50%'),
    posY = $bindable('50%'),
    blur = $bindable(0),
    label = 'Image',
    accept = 'image/*',
    showBlur = false,
    aspect = 'video',
  }: {
    value: string;
    size: string;
    posX: string;
    posY: string;
    blur: number;
    label: string;
    accept?: string;
    showBlur?: boolean;
    aspect?: string;
  } = $props();

  let uploading = $state(false);

  async function handleFile(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    uploading = true;
    try {
      const result = await uploadFile(file);
      value = window.location.origin + result.url;
      addToast(`${label} uploaded`, 'success');
    } catch (err: any) {
      addToast(err.message || 'Upload failed', 'error');
    } finally {
      uploading = false;
      input.value = '';
    }
  }

  function clear() {
    value = '';
    size = 'cover';
    posX = '50%';
    posY = '50%';
    blur = 0;
  }
</script>

<div class="space-y-3">
  <!-- Current image + controls -->
  {#if value}
    <div class="relative rounded-xl overflow-hidden border border-gray-200">
      <!-- Preview -->
      <div class="relative {aspect === 'square' ? 'aspect-square' : aspect === 'portrait' ? 'aspect-[9/16]' : 'aspect-video'} overflow-hidden bg-gray-100">
        {#if aspect === 'square'}
          <img src={value} alt={label} class="absolute inset-0 w-full h-full object-cover" style={`object-position: ${posX} ${posY};`} />
        {:else}
          <div
            class="absolute inset-0 bg-cover bg-no-repeat transition-all duration-300"
            style={`background-image: url(${value}); background-size: ${size}; background-position: ${posX} ${posY}; filter: blur(${blur}px); transform: scale(${blur > 0 ? 1.1 : 1});`}
          ></div>
        {/if}
        <!-- Remove button -->
        <button onclick={clear} class="absolute top-2 right-2 p-1.5 bg-white/90 rounded-lg hover:bg-white transition-colors shadow-sm z-10" aria-label="Remove image">
          <X class="w-4 h-4 text-gray-600" />
        </button>
      </div>

      <!-- Controls -->
      <div class="p-3 bg-gray-50 space-y-3">
        <!-- Size -->
        <div>
          <label class="flex items-center gap-1.5 text-xs font-medium text-gray-600 mb-1">
            <Maximize2 class="w-3 h-3" /> Size
          </label>
          <div class="flex gap-1">
            {#each ['cover', 'contain', 'auto'] as opt}
              <button
                onclick={() => size = opt}
                class="flex-1 px-2 py-1.5 text-xs font-medium rounded-lg border transition-colors {size === opt ? 'bg-red text-white border-red' : 'bg-white text-gray-600 border-gray-200 hover:bg-gray-50'}"
              >
                {opt}
              </button>
            {/each}
          </div>
        </div>

        <!-- Position -->
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="flex items-center justify-between text-xs font-medium text-gray-600 mb-1">
              <span class="flex items-center gap-1"><Move class="w-3 h-3" /> Position X</span>
              <span class="text-gray-400">{posX}</span>
            </label>
            <input type="range" min="0" max="100" step="1" value={parseInt(posX) || 50}
              oninput={(e) => posX = `${(e.target as HTMLInputElement).value}%`}
              class="w-full h-1.5 bg-gray-200 rounded-lg appearance-none cursor-pointer accent-red" />
          </div>
          <div>
            <label class="flex items-center justify-between text-xs font-medium text-gray-600 mb-1">
              <span class="flex items-center gap-1"><Move class="w-3 h-3" /> Position Y</span>
              <span class="text-gray-400">{posY}</span>
            </label>
            <input type="range" min="0" max="100" step="1" value={parseInt(posY) || 50}
              oninput={(e) => posY = `${(e.target as HTMLInputElement).value}%`}
              class="w-full h-1.5 bg-gray-200 rounded-lg appearance-none cursor-pointer accent-red" />
          </div>
        </div>

        <!-- Blur -->
        {#if showBlur}
          <div>
            <label class="flex items-center justify-between text-xs font-medium text-gray-600 mb-1">
              <span>Blur</span>
              <span class="text-gray-400">{blur}px</span>
            </label>
            <input type="range" min="0" max="20" step="1" bind:value={blur}
              class="w-full h-1.5 bg-gray-200 rounded-lg appearance-none cursor-pointer accent-red" />
          </div>
        {/if}
      </div>
    </div>
  {:else}
    <!-- Upload area -->
    <label class="flex flex-col items-center justify-center h-32 border-2 border-dashed border-gray-200 rounded-xl cursor-pointer hover:border-red/50 hover:bg-red-50/30 transition-colors">
      {#if uploading}
        <div class="w-6 h-6 border-2 border-red/30 border-t-red rounded-full animate-spin mb-2"></div>
        <span class="text-xs text-gray-500">Uploading...</span>
      {:else}
        <Upload class="w-6 h-6 text-gray-400 mb-2" />
        <span class="text-xs text-gray-500">Click or drag to upload {label}</span>
        <span class="text-[10px] text-gray-400 mt-0.5">or paste a URL below</span>
      {/if}
      <input type="file" {accept} class="hidden" onchange={handleFile} />
    </label>

    <!-- URL input fallback -->
    <div class="relative">
      <input
        type="url"
        {value}
        oninput={(e) => value = (e.target as HTMLInputElement).value}
        placeholder="Or paste image URL..."
        class="w-full px-3 py-2 border border-gray-200 rounded-lg text-xs bg-white focus:border-red focus:ring-1 focus:ring-red/15 outline-none transition-all"
      />
    </div>
  {/if}
</div>
