<script lang="ts">
  import { Plus, Trash2, Save, X, RotateCw, Maximize2 } from 'lucide-svelte';
  import type { HallElement, ElementType } from '$lib/types';

  let {
    hallWidth,
    hallHeight,
    selectedId = null,
    isTableSelected = false,
    selectedItem = null,
    onSave,
    onCancel,
    onDelete,
    onAddElement,
    onUpdateSelected,
    onWidthChange,
    onHeightChange,
  }: {
    hallWidth: number;
    hallHeight: number;
    selectedId?: string | null;
    isTableSelected?: boolean;
    selectedItem?: any;
    onSave: () => void;
    onCancel: () => void;
    onDelete: (id: string) => void;
    onAddElement: (el: HallElement) => void;
    onUpdateSelected: (props: Record<string, any>) => void;
    onWidthChange: (w: number) => void;
    onHeightChange: (h: number) => void;
  } = $props();

  const defaults: Record<ElementType, { w: number; h: number; label: string }> = {
    stage: { w: 55, h: 6, label: 'Stage' },
    dj_counter: { w: 12, h: 5, label: 'DJ' },
    entrance: { w: 14, h: 4, label: 'Entrance' },
    tv: { w: 5, h: 3, label: 'TV' },
    walkway: { w: 3, h: 40, label: '' },
    box: { w: 25, h: 30, label: '' },
  };

  function addElement(type: ElementType) {
    const d = defaults[type];
    onAddElement({
      id: '',
      type,
      x: 50,
      y: 50,
      degree: 0,
      width: d.w,
      height: d.h,
      label: d.label,
      zIndex: 10,
    });
  }

  let saving = $state(false);
  async function handleSave() {
    saving = true;
    try {
      await onSave();
    } finally {
      saving = false;
    }
  }
</script>

<div class="flex flex-wrap items-center gap-2 px-3 py-2 bg-white border-b border-gray-200 text-xs">
  <span class="font-semibold text-gray-500 mr-1">Add:</span>
  {#each (['stage', 'dj_counter', 'entrance', 'tv', 'walkway', 'box'] as ElementType[]) as type}
    <button
      onclick={() => addElement(type)}
      class="px-2 py-1 border border-gray-200 rounded-lg hover:bg-gray-50 text-gray-700 font-medium transition-colors"
    >
      {type === 'dj_counter' ? 'DJ' : type.charAt(0).toUpperCase() + type.slice(1)}
    </button>
  {/each}

  {#if selectedId && !isTableSelected}
    <button
      onclick={() => onDelete(selectedId!)}
      class="px-2 py-1 border border-red-200 text-red rounded-lg hover:bg-red-50 font-medium transition-colors flex items-center gap-1"
    >
      <Trash2 class="w-3 h-3" /> Delete
    </button>
  {/if}

  <!-- Hall size -->
  <div class="flex items-center gap-1 ml-auto">
    <label for="hall-w" class="text-gray-500">W</label>
    <input
      id="hall-w"
      type="number"
      value={hallWidth}
      onchange={(e) => onWidthChange(Number(e.currentTarget.value))}
      class="w-16 px-1.5 py-1 border border-gray-200 rounded-lg text-center"
      min="200"
      max="3000"
    />
    <label for="hall-h" class="text-gray-500 ml-1">H</label>
    <input
      id="hall-h"
      type="number"
      value={hallHeight}
      onchange={(e) => onHeightChange(Number(e.currentTarget.value))}
      class="w-16 px-1.5 py-1 border border-gray-200 rounded-lg text-center"
      min="200"
      max="3000"
    />
  </div>

  <button
    onclick={handleSave}
    disabled={saving}
    class="px-3 py-1.5 bg-red text-white rounded-lg font-semibold hover:bg-red-light transition-colors disabled:opacity-50 flex items-center gap-1"
  >
    <Save class="w-3 h-3" /> {saving ? 'Saving...' : 'Save'}
  </button>
  <button
    onclick={onCancel}
    class="px-3 py-1.5 border border-gray-200 rounded-lg font-medium text-gray-700 hover:bg-gray-50 transition-colors flex items-center gap-1"
  >
    <X class="w-3 h-3" /> Cancel
  </button>
</div>

<!-- Selected item property editor -->
{#if selectedItem}
  <div class="flex flex-wrap items-center gap-3 px-3 py-2 bg-gray-50 border-b border-gray-200 text-xs">
    <span class="font-semibold text-gray-500">
      {selectedItem.kind === 'table' ? `Table: ${selectedItem.name}` : `${selectedItem.type}`}
    </span>

    <!-- Degree (both tables and elements) -->
    <div class="flex items-center gap-1">
      <RotateCw class="w-3 h-3 text-gray-400" />
      <label for="sel-degree" class="text-gray-500">°</label>
      <input
        id="sel-degree"
        type="number"
        value={selectedItem.degree ?? 0}
        onchange={(e) => onUpdateSelected({ degree: Number(e.currentTarget.value) })}
        class="w-14 px-1.5 py-1 border border-gray-200 rounded-lg text-center"
        min="0"
        max="360"
        step="15"
      />
    </div>

    <!-- Width + Height (elements only) -->
    {#if selectedItem.kind === 'element'}
      <div class="flex items-center gap-1">
        <Maximize2 class="w-3 h-3 text-gray-400" />
        <label for="sel-w" class="text-gray-500">W</label>
        <input
          id="sel-w"
          type="number"
          value={selectedItem.width ?? 10}
          onchange={(e) => onUpdateSelected({ width: Number(e.currentTarget.value) })}
          class="w-14 px-1.5 py-1 border border-gray-200 rounded-lg text-center"
          min="1"
          max="100"
        />
        <label for="sel-h" class="text-gray-500 ml-1">H</label>
        <input
          id="sel-h"
          type="number"
          value={selectedItem.height ?? 10}
          onchange={(e) => onUpdateSelected({ height: Number(e.currentTarget.value) })}
          class="w-14 px-1.5 py-1 border border-gray-200 rounded-lg text-center"
          min="1"
          max="100"
        />
      </div>
      <!-- Label -->
      <div class="flex items-center gap-1">
        <label for="sel-label" class="text-gray-500">Label</label>
        <input
          id="sel-label"
          type="text"
          value={selectedItem.label ?? ''}
          oninput={(e) => onUpdateSelected({ label: e.currentTarget.value })}
          class="w-24 px-1.5 py-1 border border-gray-200 rounded-lg"
        />
      </div>
    {/if}

    <!-- Table-specific: capacity, VIP -->
    {#if selectedItem.kind === 'table'}
      <div class="flex items-center gap-1">
        <label for="sel-cap" class="text-gray-500">Seats</label>
        <input
          id="sel-cap"
          type="number"
          value={selectedItem.capacity ?? 10}
          onchange={(e) => onUpdateSelected({ capacity: Number(e.currentTarget.value) })}
          class="w-14 px-1.5 py-1 border border-gray-200 rounded-lg text-center"
          min="1"
          max="50"
        />
      </div>
      <div class="flex items-center gap-1">
        <label for="sel-vip" class="text-gray-500">VIP</label>
        <input
          id="sel-vip"
          type="checkbox"
          checked={selectedItem.isVip ?? false}
          onchange={(e) => onUpdateSelected({ isVip: e.currentTarget.checked })}
          class="rounded"
        />
      </div>
    {/if}
  </div>
{/if}
