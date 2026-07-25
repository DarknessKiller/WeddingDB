<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { addToast, getAuth } from '$lib/stores';
  import { weddingId, setWeddingId } from '$lib/stores/weddingId';
  import { listWeddings, createWedding, updateWedding, deleteWedding, type Wedding } from '$lib/api/weddings';
  import { Plus, Pencil, Trash2, X, Calendar, Check, ChevronRight } from 'lucide-svelte';
  import dayjs from 'dayjs';
  import { encodeId } from '$lib/utils/encode';

  let weddings = $state<Wedding[]>([]);
  let loading = $state(true);
  let selecting = $state<string | null>(null);

  const auth = getAuth();
  const isAdmin = auth.role === 'admin';
  let currentWeddingId = $state('');

  weddingId.subscribe(v => { currentWeddingId = v; });

  let showModal = $state(false);
  let editingWedding = $state<Wedding | null>(null);
  let formName = $state('');
  let formDate = $state('');
  let saving = $state(false);

  onMount(async () => { await loadWeddings(); });

  async function loadWeddings() {
    loading = true;
    try { weddings = await listWeddings(); }
    catch (e: any) { addToast(e.message ?? 'Failed to load weddings', 'error'); }
    finally { loading = false; }
  }

  async function selectWedding(w: Wedding) {
    if (selecting !== null) return;
    selecting = w.id;
    try {
      const res = await fetch('/api/auth/select-wedding', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${auth.accessToken}` },
        body: JSON.stringify({ weddingId: w.id })
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({ title: 'Failed to select wedding' }));
        addToast(err.title || 'Failed to select wedding', 'error');
        return;
      }
      const data = await res.json();
      localStorage.setItem('weddingdb_access_token', data.accessToken);
      setWeddingId(w.id);
      addToast(`Switched to ${w.name}`, 'success');
      goto(`/${encodeId(w.id)}/dashboard`, { replaceState: true });
    } catch { addToast('Network error', 'error'); }
    finally { selecting = null; }
  }

  function openCreate() {
    editingWedding = null;
    formName = '';
    formDate = dayjs().format('YYYY-MM-DD');
    showModal = true;
  }

  function openEdit(w: Wedding, e: Event) {
    e.stopPropagation();
    editingWedding = w;
    formName = w.name;
    formDate = w.date?.slice(0, 10) ?? '';
    showModal = true;
  }

  function closeModal() { showModal = false; editingWedding = null; }

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
    } catch (e: any) { addToast(e.message ?? 'Save failed', 'error'); }
    finally { saving = false; }
  }

  async function handleDelete(w: Wedding, e: Event) {
    e.stopPropagation();
    if (!confirm(`Delete wedding "${w.name}"? This cannot be undone.`)) return;
    try {
      await deleteWedding(w.id);
      weddings = weddings.filter(x => x.id !== w.id);
      addToast(`${w.name} deleted`, 'info');
    } catch (e: any) { addToast(e.message ?? 'Delete failed', 'error'); }
  }
</script>

<svelte:head><title>Weddings – WeddingDB</title></svelte:head>

<div class="min-h-screen flex items-center justify-center bg-white px-4">
  <div class="w-full max-w-sm">
    <!-- Logo -->
    <div class="text-center mb-8">
      <div class="w-16 h-16 bg-deep-red rounded-2xl flex items-center justify-center mx-auto mb-4 shadow-lg">
        <span class="text-2xl font-bold text-white font-serif">W</span>
      </div>
      <h1 class="text-2xl font-bold text-gray-900 font-serif">WeddingDB</h1>
      <p class="text-sm text-gray-500 mt-1">Select a wedding to manage</p>
    </div>

    <div class="bg-white rounded-2xl shadow-sm border border-gray-100 p-6">
      {#if loading}
        <div class="space-y-3">
          {#each Array(3) as _}
            <div class="h-14 bg-gray-50 rounded-xl animate-pulse"></div>
          {/each}
        </div>
      {:else if weddings.length === 0 && isAdmin}
        <div class="text-center py-4">
          <Calendar class="w-10 h-10 text-gray-300 mx-auto mb-3" />
          <p class="text-sm text-gray-500 mb-4">No weddings yet. Create your first wedding to get started.</p>
          <button onclick={openCreate}
            class="w-full px-4 py-2.5 bg-deep-red text-white rounded-xl text-sm font-semibold hover:bg-deep-red/90 transition-colors">
            Create Wedding
          </button>
        </div>
      {:else}
        <div class="space-y-2">
          {#each weddings as w (w.id)}
            {@const isSelected = w.id === currentWeddingId}
            <div class="group">
              <button
                onclick={() => selectWedding(w)}
                disabled={selecting !== null}
                class="w-full flex items-center gap-3 p-3 rounded-xl border transition-all text-left {isSelected ? 'border-gold bg-gold-50/50' : 'border-gray-200 hover:border-deep-red hover:bg-red-50'}"
              >
                <div class="w-10 h-10 rounded-lg {isSelected ? 'bg-gold-50 border border-gold-200 text-gold' : 'bg-red-50 border border-red-100 text-red'} flex items-center justify-center flex-shrink-0">
                  {#if isSelected}
                    <Check class="w-5 h-5" />
                  {:else}
                    <Calendar class="w-5 h-5" />
                  {/if}
                </div>
                <div class="flex-1 min-w-0">
                  <div class="font-semibold text-gray-900 text-sm">{w.name}</div>
                  <div class="text-xs text-gray-500">{w.date ? new Date(w.date).toLocaleDateString() : 'No date'}</div>
                </div>
                {#if selecting === w.id}
                  <div class="w-4 h-4 border-2 border-deep-red/30 border-t-deep-red rounded-full animate-spin"></div>
                {:else if isSelected}
                  <span class="text-xs font-medium text-gold bg-gold-50 px-2 py-0.5 rounded-full border border-gold-200">Active</span>
                {:else}
                  <ChevronRight class="w-4 h-4 text-gray-400 group-hover:text-deep-red transition-colors" />
                {/if}
                {#if isAdmin}
                  <div class="flex items-center gap-1 ml-2 {isSelected ? 'opacity-0 group-hover:opacity-100' : 'opacity-0 group-hover:opacity-100'} transition-opacity">
                    <button onclick={(e) => openEdit(w, e)} class="p-1.5 rounded-lg hover:bg-white/80 text-gray-400 hover:text-gray-600 transition-colors" aria-label="Edit">
                      <Pencil class="w-3.5 h-3.5" />
                    </button>
                    <button onclick={(e) => handleDelete(w, e)} class="p-1.5 rounded-lg hover:bg-red-50 text-gray-400 hover:text-red transition-colors" aria-label="Delete">
                      <Trash2 class="w-3.5 h-3.5" />
                    </button>
                  </div>
                {/if}
              </button>
            </div>
          {/each}
        </div>

        {#if isAdmin}
          <button onclick={openCreate}
            class="w-full mt-3 px-4 py-2.5 border border-dashed border-gray-300 rounded-xl text-sm font-medium text-gray-500 hover:border-deep-red hover:text-deep-red hover:bg-red-50 transition-colors flex items-center justify-center gap-2">
            <Plus class="w-4 h-4" /> New Wedding
          </button>
        {/if}
      {/if}
    </div>
  </div>
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
