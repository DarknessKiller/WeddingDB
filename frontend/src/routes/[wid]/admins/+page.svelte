<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { addToast, getAuth } from '$lib/stores';
  import { weddingId } from '$lib/stores/weddingId';
  import { listAdmins, createAdmin, deleteAdmin, assignWeddings, type AdminUser } from '$lib/api/admins';
  import { listWeddings, type Wedding } from '$lib/api/weddings';
  import { Plus, Trash2, X, Users, Building2 } from 'lucide-svelte';

  let admins = $state<AdminUser[]>([]);
  let weddings = $state<Wedding[]>([]);
  let loading = $state(true);

  // Guard: admin only
  const auth = getAuth();
  if (auth.role !== 'admin') {
    goto(`/${$weddingId}/dashboard`, { replaceState: true });
  }

  // Create modal
  let showCreate = $state(false);
  let formName = $state('');
  let formEmail = $state('');
  let formPassword = $state('');
  let formRole = $state('user');
  let saving = $state(false);

  // Assign weddings modal
  let assignTarget = $state<AdminUser | null>(null);
  let assignWeddingIds = $state<string[]>([]);

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
      formName = ''; formEmail = ''; formPassword = ''; formRole = 'user';
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
    assignWeddingIds = [];
  }

  async function handleAssign() {
    if (!assignTarget) return;
    try {
      const updated = await assignWeddings(assignTarget.id, assignWeddingIds);
      admins = admins.map(a => a.id === updated.id ? updated : a);
      addToast('Weddings updated', 'success');
      assignTarget = null;
    } catch (e: any) {
      addToast(e.message ?? 'Failed to assign weddings', 'error');
    }
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
            <th class="px-5 py-3 w-10"></th>
          </tr>
        </thead>
        <tbody>
          {#each admins as admin (admin.id)}
            <tr class="border-b border-gray-100 hover:bg-gray-50">
              <td class="px-5 py-3.5 font-semibold text-gray-900">{admin.name}</td>
              <td class="px-5 py-3.5 text-gray-600">{admin.email}</td>
              <td class="px-5 py-3.5">
                <span class="inline-flex px-2 py-0.5 rounded-full text-xs font-semibold {admin.role === 'admin' ? 'bg-gold-50 text-gold border border-gold-200' : 'bg-gray-100 text-gray-600 border border-gray-200'}">
                  {admin.role === 'admin' ? 'Admin' : 'User'}
                </span>
              </td>
              <td class="px-5 py-3.5">
                {#if admin.role !== 'admin'}
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
            <option value="user">User</option>
            <option value="admin">Admin</option>
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

<!-- Assign Weddings Modal -->
{#if assignTarget}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="absolute inset-0 bg-black/40 backdrop-blur-sm" onclick={() => assignTarget = null} role="presentation"></div>
    <div class="relative bg-white rounded-2xl shadow-2xl w-full max-w-sm overflow-hidden">
      <div class="flex items-center justify-between p-5 border-b border-gray-100">
        <h3 class="font-bold text-gray-900">Assign Weddings</h3>
        <button onclick={() => assignTarget = null} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
          <X class="w-4 h-4 text-gray-400" />
        </button>
      </div>
      <div class="p-5 space-y-4">
        <p class="text-sm text-gray-600">Assign weddings to <strong>{assignTarget.name}</strong></p>
        <div class="space-y-2 max-h-60 overflow-y-auto">
          {#each weddings as w}
            <label class="flex items-center gap-2 p-2 rounded-lg hover:bg-gray-50 cursor-pointer">
              <input type="checkbox" value={w.id} bind:group={assignWeddingIds} class="rounded border-gray-300 text-deep-red focus:ring-deep-red/20" />
              <span class="text-sm text-gray-700">{w.name}</span>
            </label>
          {/each}
        </div>
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
