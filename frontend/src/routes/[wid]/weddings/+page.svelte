<script lang="ts">
  import { onMount } from 'svelte';
  import { addToast, getAuth } from '$lib/stores';
  import { listWeddings, createWedding, updateWedding, deleteWedding, type Wedding } from '$lib/api/weddings';
  import { Plus, Pencil, Trash2, X, Calendar, Users } from 'lucide-svelte';
  import dayjs from 'dayjs';

  let weddings = $state<Wedding[]>([]);
  let loading = $state(true);

  const auth = getAuth();
  const isServiceAdmin = auth.role === 'service_admin';

  // Modal state
  let showModal = $state(false);
  let editingWedding = $state<Wedding | null>(null);
  let formName = $state('');
  let formDate = $state('');
  let saving = $state(false);

  onMount(async () => {
    await loadWeddings();
  });

  async function loadWeddings() {
    loading = true;
    try {
      weddings = await listWeddings();
    } catch (e: any) {
      addToast(e.message ?? 'Failed to load weddings', 'error');
    } finally {
      loading = false;
    }
  }

  function openCreate() {
    editingWedding = null;
    formName = '';
    formDate = dayjs().format('YYYY-MM-DD');
    showModal = true;
  }

  function openEdit(w: Wedding) {
    editingWedding = w;
    formName = w.name;
    formDate = w.date?.slice(0, 10) ?? '';
    showModal = true;
  }

  function closeModal() {
    showModal = false;
    editingWedding = null;
  }

  async function handleSave() {
    if (!formName || !formDate) return;
    saving = true;
    try {
      if (editingWedding) {
        await updateWedding(editingWedding.id, { name: formName, date: formDate });
        addToast('Wedding updated', 'success');
      } else {
        await createWedding({ name: formName, date: formDate });
        addToast('Wedding created', 'success');
      }
      closeModal();
      await loadWeddings();
    } catch (e: any) {
      addToast(e.message ?? 'Save failed', 'error');
    } finally {
      saving = false;
    }
  }

  async function handleDelete(w: Wedding) {
    if (!confirm(`Delete wedding "${w.name}"? This cannot be undone.`)) return;
    try {
      await deleteWedding(w.id);
      weddings = weddings.filter(x => x.id !== w.id);
      addToast(`${w.name} deleted`, 'info');
    } catch (e: any) {
      addToast(e.message ?? 'Delete failed', 'error');
    }
  }
</script>

<svelte:head><title>Weddings – WeddingDB</title></svelte:head>

<div class="p-4 sm:p-7 max-w-[1200px]">
  <div class="flex items-center justify-between mb-6">
    <div>
      <h1 class="text-xl font-bold text-gray-900">Weddings</h1>
      <p class="text-sm text-gray-500 mt-0.5">{weddings.length} wedding(s)</p>
    </div>
    {#if isServiceAdmin}
      <button onclick={openCreate} class="px-4 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center gap-2">
        <Plus class="w-4 h-4" /> New Wedding
      </button>
    {/if}
  </div>

  {#if loading}
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {#each Array(3) as _}
        <div class="bg-white border border-gray-200 rounded-2xl p-6 animate-pulse">
          <div class="h-5 bg-gray-100 rounded w-32 mb-3"></div>
          <div class="h-4 bg-gray-100 rounded w-24 mb-2"></div>
          <div class="h-3 bg-gray-100 rounded w-20"></div>
        </div>
      {/each}
    </div>
  {:else if weddings.length === 0}
    <div class="flex flex-col items-center justify-center py-20 text-center">
      <div class="w-16 h-16 bg-gray-100 rounded-full flex items-center justify-center mb-4">
        <Calendar class="w-8 h-8 text-gray-400" />
      </div>
      <p class="text-gray-500 font-medium">No weddings yet</p>
      <p class="text-sm text-gray-400 mt-1 mb-4">Create your first wedding to get started.</p>
      {#if isServiceAdmin}
        <button onclick={openCreate} class="px-4 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center gap-2">
          <Plus class="w-4 h-4" /> New Wedding
        </button>
      {/if}
    </div>
  {:else}
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {#each weddings as w (w.id)}
        <div class="bg-white border border-gray-200 rounded-2xl p-6 hover:shadow-md transition-shadow duration-200 group relative">
          {#if isServiceAdmin}
            <div class="absolute top-3 right-3 flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
              <button onclick={() => openEdit(w)} class="p-1.5 rounded-lg hover:bg-gray-100 text-gray-400 hover:text-gray-600 transition-colors" aria-label="Edit">
                <Pencil class="w-4 h-4" />
              </button>
              <button onclick={() => handleDelete(w)} class="p-1.5 rounded-lg hover:bg-red-50 text-gray-400 hover:text-red transition-colors" aria-label="Delete">
                <Trash2 class="w-4 h-4" />
              </button>
            </div>
          {:else}
            <button onclick={() => openEdit(w)} class="absolute top-3 right-3 p-1.5 rounded-lg hover:bg-gray-100 text-gray-400 hover:text-gray-600 transition-colors opacity-0 group-hover:opacity-100 transition-opacity" aria-label="Edit">
              <Pencil class="w-4 h-4" />
            </button>
          {/if}

          <div class="w-12 h-12 rounded-xl bg-red-50 border border-red-100 flex items-center justify-center text-red font-bold text-lg mb-4">
            <Calendar class="w-6 h-6" />
          </div>
          <h3 class="font-bold text-gray-900 text-lg mb-1">{w.name}</h3>
          <p class="text-sm text-gray-500 flex items-center gap-1.5">
            <Calendar class="w-3.5 h-3.5" />
            {w.date ? dayjs(w.date).format('MMMM D, YYYY') : 'No date set'}
          </p>
        </div>
      {/each}
    </div>
  {/if}
</div>

<!-- Create/Edit Modal -->
{#if showModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="absolute inset-0 bg-black/40 backdrop-blur-sm" onclick={closeModal} role="presentation"></div>
    <div class="relative bg-white rounded-2xl shadow-2xl w-full max-w-md overflow-hidden">
      <div class="flex items-center justify-between p-5 border-b border-gray-100">
        <h3 class="font-bold text-gray-900">{editingWedding ? 'Edit Wedding' : 'New Wedding'}</h3>
        <button onclick={closeModal} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
          <X class="w-4 h-4 text-gray-400" />
        </button>
      </div>
      <div class="p-5 space-y-4">
        <div>
          <label for="wedding-name" class="text-sm font-semibold text-gray-700 mb-1.5 block">Wedding Name</label>
          <input id="wedding-name" type="text" bind:value={formName} placeholder="e.g. John & Jane's Wedding"
            class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all" />
        </div>
        <div>
          <label for="wedding-date" class="text-sm font-semibold text-gray-700 mb-1.5 block">Date</label>
          <input id="wedding-date" type="date" bind:value={formDate}
            class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all" />
        </div>
      </div>
      <div class="flex gap-3 p-5 pt-0">
        <button onclick={handleSave} disabled={saving || !formName || !formDate}
          class="flex-1 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors disabled:opacity-50">
          {saving ? 'Saving...' : editingWedding ? 'Save Changes' : 'Create Wedding'}
        </button>
        <button onclick={closeModal} class="px-5 py-2.5 border border-gray-200 bg-white rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
          Cancel
        </button>
      </div>
    </div>
  </div>
{/if}
