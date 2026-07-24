<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { addToast, getAuth } from '$lib/stores';
  import { weddingId } from '$lib/stores/weddingId';
  import { listAdmins, createAdmin, deleteAdmin, assignWedding, type AdminUser } from '$lib/api/admins';
  import { listWeddings, type Wedding } from '$lib/api/weddings';
  import { Plus, Trash2, X, Users, Building2 } from 'lucide-svelte';

  let admins = $state<AdminUser[]>([]);
  let weddings = $state<Wedding[]>([]);
  let loading = $state(true);

  // Guard: service_admin only
  const auth = getAuth();
  if (auth.role !== 'service_admin') {
    goto(`/${$weddingId}/dashboard`, { replaceState: true });
  }

  // Create modal
  let showCreate = $state(false);
  let formName = $state('');
  let formEmail = $state('');
  let formPassword = $state('');
  let formRole = $state('wedding_admin');
  let saving = $state(false);

  // Assign wedding modal
  let assignTarget = $state<AdminUser | null>(null);
  let assignWeddingId = $state<string>('');

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    loading = true;
    try {
      const [a, w] = await Promise.all([listAdmins(), listWeddings()]);
      admins = a;
      weddings = w;
    } catch (e: any) {
      addToast(e.message ?? 'Failed to load data', 'error');
    } finally {
      loading = false;
    }
  }

  async function handleCreate() {
    if (!formName || !formEmail || !formPassword) return;
    saving = true;
    try {
      await createAdmin({ name: formName, email: formEmail, password: formPassword, role: formRole });
      addToast('Admin created', 'success');
      showCreate = false;
      formName = ''; formEmail = ''; formPassword = ''; formRole = 'wedding_admin';
      await loadData();
    } catch (e: any) {
      addToast(e.message ?? 'Failed to create admin', 'error');
    } finally {
      saving = false;
    }
  }

  async function handleDelete(admin: AdminUser) {
    if (!confirm(`Delete admin "${admin.name}"?`)) return;
    try {
      await deleteAdmin(admin.id);
      admins = admins.filter(a => a.id !== admin.id);
      addToast(`${admin.name} deleted`, 'info');
    } catch (e: any) {
      addToast(e.message ?? 'Delete failed', 'error');
    }
  }

  function openAssign(admin: AdminUser) {
    assignTarget = admin;
    assignWeddingId = admin.weddingId != null ? String(admin.weddingId) : '';
  }

  async function handleAssign() {
    if (!assignTarget) return;
    try {
      const wid = assignWeddingId ? Number(assignWeddingId) : null;
      const updated = await assignWedding(assignTarget.id, wid);
      admins = admins.map(a => a.id === updated.id ? { ...a, weddingId: updated.weddingId } : a);
      addToast(`Wedding ${wid ? 'assigned' : 'unassigned'}`, 'success');
      assignTarget = null;
    } catch (e: any) {
      addToast(e.message ?? 'Failed to assign wedding', 'error');
    }
  }

  function weddingName(id: number | null): string {
    if (id == null) return '—';
    return weddings.find(w => w.id === id)?.name ?? `Wedding #${id}`;
  }
</script>

<svelte:head><title>Admins – WeddingDB</title></svelte:head>

<div class="p-4 sm:p-7 max-w-[1200px]">
  <div class="flex items-center justify-between mb-6">
    <div>
      <h1 class="text-xl font-bold text-gray-900">Admin Management</h1>
      <p class="text-sm text-gray-500 mt-0.5">{admins.length} admin(s)</p>
    </div>
    <button onclick={() => showCreate = true} class="px-4 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center gap-2">
      <Plus class="w-4 h-4" /> Add Admin
    </button>
  </div>

  {#if loading}
    <div class="text-center py-20 text-gray-400">Loading admins...</div>
  {:else if admins.length === 0}
    <div class="text-center py-20 text-gray-400">No admins found</div>
  {:else}
    <div class="bg-white border border-gray-200 rounded-2xl overflow-hidden">
      <table class="w-full text-sm">
        <thead>
          <tr class="bg-gray-50 border-b border-gray-200">
            <th class="px-5 py-3 text-left text-[12px] font-semibold uppercase tracking-wider text-gray-500">Name</th>
            <th class="px-5 py-3 text-left text-[12px] font-semibold uppercase tracking-wider text-gray-500">Email</th>
            <th class="px-5 py-3 text-left text-[12px] font-semibold uppercase tracking-wider text-gray-500">Role</th>
            <th class="px-5 py-3 text-left text-[12px] font-semibold uppercase tracking-wider text-gray-500">Wedding</th>
            <th class="px-5 py-3 w-10"></th>
          </tr>
        </thead>
        <tbody>
          {#each admins as admin (admin.id)}
            <tr class="border-b border-gray-100 hover:bg-gray-50">
              <td class="px-5 py-3.5 font-semibold text-gray-900">{admin.name}</td>
              <td class="px-5 py-3.5 text-gray-600">{admin.email}</td>
              <td class="px-5 py-3.5">
                <span class="inline-flex px-2 py-0.5 rounded-full text-xs font-semibold {admin.role === 'service_admin' ? 'bg-gold-50 text-gold border border-gold-200' : 'bg-gray-100 text-gray-600 border border-gray-200'}">
                  {admin.role === 'service_admin' ? 'Service Admin' : 'Wedding Admin'}
                </span>
              </td>
              <td class="px-5 py-3.5 text-gray-600">
                {#if admin.role !== 'service_admin'}
                  <button onclick={() => openAssign(admin)} class="text-left hover:text-deep-red transition-colors flex items-center gap-1">
                    <Building2 class="w-3.5 h-3.5" />
                    {weddingName(admin.weddingId)}
                  </button>
                {:else}
                  <span class="text-gray-400">All</span>
                {/if}
              </td>
              <td class="px-5 py-3.5">
                {#if admin.role !== 'service_admin'}
                  <button onclick={() => handleDelete(admin)} class="p-1.5 rounded-lg hover:bg-red-50 text-gray-400 hover:text-red transition-colors" aria-label="Delete">
                    <Trash2 class="w-4 h-4" />
                  </button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<!-- Create Admin Modal -->
{#if showCreate}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="absolute inset-0 bg-black/40 backdrop-blur-sm" onclick={() => showCreate = false} role="presentation"></div>
    <div class="relative bg-white rounded-2xl shadow-2xl w-full max-w-md overflow-hidden">
      <div class="flex items-center justify-between p-5 border-b border-gray-100">
        <h3 class="font-bold text-gray-900">Add Admin</h3>
        <button onclick={() => showCreate = false} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
          <X class="w-4 h-4 text-gray-400" />
        </button>
      </div>
      <div class="p-5 space-y-4">
        <div>
          <label class="text-sm font-semibold text-gray-700 mb-1.5 block">Name</label>
          <input bind:value={formName} class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all" placeholder="Full name" />
        </div>
        <div>
          <label class="text-sm font-semibold text-gray-700 mb-1.5 block">Email</label>
          <input type="email" bind:value={formEmail} class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all" placeholder="email@example.com" />
        </div>
        <div>
          <label class="text-sm font-semibold text-gray-700 mb-1.5 block">Password</label>
          <input type="password" bind:value={formPassword} minlength="6" class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold focus:ring-2 focus:ring-gold/15 outline-none transition-all" placeholder="Min 6 characters" />
        </div>
        <div>
          <label class="text-sm font-semibold text-gray-700 mb-1.5 block">Role</label>
          <select bind:value={formRole} class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold outline-none">
            <option value="wedding_admin">Wedding Admin</option>
            <option value="service_admin">Service Admin</option>
          </select>
        </div>
      </div>
      <div class="flex gap-3 p-5 pt-0">
        <button onclick={handleCreate} disabled={saving || !formName || !formEmail || !formPassword}
          class="flex-1 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors disabled:opacity-50">
          {saving ? 'Creating...' : 'Create Admin'}
        </button>
        <button onclick={() => showCreate = false} class="px-5 py-2.5 border border-gray-200 bg-white rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
          Cancel
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Assign Wedding Modal -->
{#if assignTarget}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="absolute inset-0 bg-black/40 backdrop-blur-sm" onclick={() => assignTarget = null} role="presentation"></div>
    <div class="relative bg-white rounded-2xl shadow-2xl w-full max-w-sm overflow-hidden">
      <div class="flex items-center justify-between p-5 border-b border-gray-100">
        <h3 class="font-bold text-gray-900">Assign Wedding</h3>
        <button onclick={() => assignTarget = null} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
          <X class="w-4 h-4 text-gray-400" />
        </button>
      </div>
      <div class="p-5 space-y-4">
        <p class="text-sm text-gray-600">Assign a wedding to <strong>{assignTarget.name}</strong></p>
        <select bind:value={assignWeddingId} class="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:border-gold outline-none">
          <option value="">No wedding (unassigned)</option>
          {#each weddings as w}
            <option value={String(w.id)}>{w.name} ({w.date?.slice(0, 10) ?? 'No date'})</option>
          {/each}
        </select>
      </div>
      <div class="flex gap-3 p-5 pt-0">
        <button onclick={handleAssign} class="flex-1 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors">
          Save
        </button>
        <button onclick={() => assignTarget = null} class="px-5 py-2.5 border border-gray-200 bg-white rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
          Cancel
        </button>
      </div>
    </div>
  </div>
{/if}
