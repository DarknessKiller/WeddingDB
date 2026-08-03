<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { addToast, getAuth } from '$lib/stores';
  import { weddingId } from '$lib/stores/weddingId';
  import { listUsers, createUser, deleteUser, assignWeddings, resetPassword, getUserWeddings, updateRole, type User } from '$lib/api/users';
  import { listWeddings, type Wedding } from '$lib/api/weddings';
  import { Plus, Trash2, X, Users, Building2, Key, Shield, Check } from 'lucide-svelte';
  import { weddingTitle } from '$lib/stores/weddingTitle';
  import PasswordRequirements from '$lib/components/ui/PasswordRequirements.svelte';

  let admins = $state<User[]>([]);
  let weddings = $state<Wedding[]>([]);
  let loading = $state(true);

  const auth = getAuth();
  if (auth.role !== 'admin') {
    goto(`/${$weddingId}/dashboard`, { replaceState: true });
  }

  // Create modal
  let showCreate = $state(false);
  let formName = $state('');
  let formEmail = $state('');
  let formPassword = $state('');
  let formConfirmPassword = $state('');
  let formRole = $state('user');
  let saving = $state(false);

  let createPasswordMismatch = $derived(formConfirmPassword.length > 0 && formPassword !== formConfirmPassword);
  let canCreate = $derived(
    formName.length > 0 &&
    formEmail.length > 0 &&
    formPassword.length >= 8 &&
    formPassword === formConfirmPassword &&
    /[a-zA-Z]/.test(formPassword) &&
    /\d/.test(formPassword) &&
    /[^a-zA-Z0-9]/.test(formPassword)
  );

  // Inline role editing
  let editingRoleUserId = $state<string | null>(null);
  let editingRoleValue = $state('');
  let roleSaving = $state(false);

  // Assign weddings modal
  let assignTarget = $state<User | null>(null);
  let assignWeddingIds = $state<string[]>([]);
  let assignSaving = $state(false);

  // Reset password modal
  let resetTarget = $state<User | null>(null);
  let resetPassword_ = $state('');
  let resetConfirmPassword = $state('');
  let resetSaving = $state(false);

  let resetPasswordMismatch = $derived(resetConfirmPassword.length > 0 && resetPassword_ !== resetConfirmPassword);
  let canReset = $derived(
    resetPassword_.length >= 8 &&
    resetPassword_ === resetConfirmPassword &&
    /[a-zA-Z]/.test(resetPassword_) &&
    /\d/.test(resetPassword_) &&
    /[^a-zA-Z0-9]/.test(resetPassword_)
  );

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    loading = true;
    try {
      const [a, w] = await Promise.all([listUsers(), listWeddings()]);
      admins = a;
      weddings = w;
    } catch (e: any) {
      addToast(e.message ?? 'Failed to load data', 'error');
    } finally {
      loading = false;
    }
  }

  async function handleCreate() {
    if (!canCreate) return;
    saving = true;
    try {
      await createUser({ name: formName, email: formEmail, password: formPassword, role: formRole });
      addToast('User created', 'success');
      showCreate = false;
      formName = ''; formEmail = ''; formPassword = ''; formConfirmPassword = ''; formRole = 'user';
      await loadData();
    } catch (e: any) {
      addToast(e.message ?? 'Failed to create user', 'error');
    } finally {
      saving = false;
    }
  }

  async function handleDelete(user: User) {
    if (!confirm(`Delete user "${user.name}"?`)) return;
    try {
      await deleteUser(user.id);
      admins = admins.filter(a => a.id !== user.id);
      addToast(`${user.name} deleted`, 'info');
    } catch (e: any) {
      addToast(e.message ?? 'Delete failed', 'error');
    }
  }

  // Inline role editing
  function startEditRole(user: User) {
    editingRoleUserId = user.id;
    editingRoleValue = user.role;
  }

  function cancelEditRole() {
    editingRoleUserId = null;
    editingRoleValue = '';
  }

  async function saveRole(user: User) {
    if (editingRoleValue === user.role) { cancelEditRole(); return; }
    if (editingRoleValue !== 'admin' && editingRoleValue !== 'user') return;

    // Confirm demotion
    if (user.role === 'admin' && editingRoleValue === 'user') {
      if (!confirm(`Demote "${user.name}" from admin to user?`)) { cancelEditRole(); return; }
    }
    if (editingRoleValue === 'admin' && user.role === 'user') {
      if (!confirm(`Promote "${user.name}" to admin?`)) { cancelEditRole(); return; }
    }

    roleSaving = true;
    try {
      await updateRole(user.id, editingRoleValue);
      addToast(`${user.name} is now ${editingRoleValue}`, 'success');
      editingRoleUserId = null;
      await loadData();
    } catch (e: any) {
      addToast(e.message ?? 'Failed to update role', 'error');
    } finally {
      roleSaving = false;
    }
  }

  function openAssign(user: User) {
    assignTarget = user;
    assignWeddingIds = [];
    getUserWeddings(user.id).then(weds => {
      assignWeddingIds = weds.map(w => w.id);
    }).catch(() => {});
  }

  async function handleAssign() {
    if (!assignTarget) return;
    assignSaving = true;
    try {
      await assignWeddings(assignTarget.id, assignWeddingIds);
      addToast('Weddings updated', 'success');
      assignTarget = null;
    } catch (e: any) {
      addToast(e.message ?? 'Failed to assign weddings', 'error');
    } finally {
      assignSaving = false;
    }
  }

  function openReset(user: User) {
    resetTarget = user;
    resetPassword_ = '';
    resetConfirmPassword = '';
  }

  async function handleReset() {
    if (!resetTarget || !canReset) return;
    resetSaving = true;
    try {
      await resetPassword(resetTarget.id, resetPassword_);
      addToast(`Password updated for ${resetTarget.name}`, 'success');
      resetTarget = null;
    } catch (e: any) {
      addToast(e.message ?? 'Failed to reset password', 'error');
    } finally {
      resetSaving = false;
    }
  }
</script>

<svelte:head> <title>{$weddingTitle ? `${$weddingTitle} – Users` : 'User Management – WeddingDB'}</title></svelte:head>

<div class="p-4 sm:p-7 max-w-[1200px]">
  <div class="flex items-center justify-between mb-6">
    <div>
      <h1 class="text-xl font-bold text-gray-900" style="letter-spacing: -0.02em;">User Management</h1>
      <p class="text-sm text-gray-500 mt-0.5">{admins.length} user(s)</p>
    </div>
    <button onclick={() => showCreate = true} class="px-4 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors flex items-center gap-2">
      <Plus class="w-4 h-4" /> Add User
    </button>
  </div>

  {#if loading}
    <div class="text-center py-20 text-gray-400">Loading users...</div>
  {:else if admins.length === 0}
    <div class="text-center py-20 text-gray-400">No users found</div>
  {:else}
    <!-- Desktop table -->
    <div class="hidden sm:block bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl overflow-hidden">
      <table class="w-full text-sm">
        <thead>
          <tr class="bg-gray-50 border-b border-gray-200">
            <th class="px-5 py-3 text-left text-[12px] font-semibold uppercase tracking-wider text-gray-500">Name</th>
            <th class="px-5 py-3 text-left text-[12px] font-semibold uppercase tracking-wider text-gray-500">Email</th>
            <th class="px-5 py-3 text-left text-[12px] font-semibold uppercase tracking-wider text-gray-500">Role</th>
            <th class="px-5 py-3 text-left text-[12px] font-semibold uppercase tracking-wider text-gray-500">Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each admins as user (user.id)}
            <tr class="border-b border-gray-100 hover:bg-gray-50">
              <td class="px-5 py-3.5 font-semibold text-gray-900">{user.name}</td>
              <td class="px-5 py-3.5 text-gray-600">{user.email}</td>
              <td class="px-5 py-3.5">
                {#if editingRoleUserId === user.id}
                  <div class="flex items-center gap-1.5">
                    <select bind:value={editingRoleValue}
                      class="px-2 py-1 border border-gray-200 rounded-lg text-xs font-semibold bg-white focus:border-red focus:ring-1 focus:ring-red/10 outline-none">
                      <option value="user">User</option>
                      <option value="admin">Admin</option>
                    </select>
                    <button onclick={() => saveRole(user)} disabled={roleSaving}
                      class="p-1 rounded-md bg-emerald-50 text-emerald-600 hover:bg-emerald-100 transition-colors" title="Save">
                      <Check class="w-3.5 h-3.5" />
                    </button>
                    <button onclick={cancelEditRole}
                      class="p-1 rounded-md bg-gray-100 text-gray-500 hover:bg-gray-200 transition-colors" title="Cancel">
                      <X class="w-3.5 h-3.5" />
                    </button>
                  </div>
                {:else}
                  <button onclick={() => startEditRole(user)}
                    class="inline-flex px-2 py-0.5 rounded-full text-xs font-semibold cursor-pointer hover:opacity-80 transition-opacity {user.role === 'admin' ? 'bg-gold-50 text-gold border border-gold-200' : 'bg-gray-100 text-gray-600 border border-gray-200'}">
                    {user.role === 'admin' ? 'Admin' : 'User'}
                  </button>
                {/if}
              </td>
              <td class="px-5 py-3.5">
                <div class="flex items-center gap-1">
                  {#if user.role !== 'admin' || editingRoleUserId === user.id}
                    <button onclick={() => openAssign(user)} class="p-1.5 rounded-lg hover:bg-blue-50 text-gray-400 hover:text-blue-600 transition-colors" aria-label="Assign weddings" title="Assign weddings">
                      <Building2 class="w-4 h-4" />
                    </button>
                    <button onclick={() => openReset(user)} class="p-1.5 rounded-lg hover:bg-amber-50 text-gray-400 hover:text-amber-600 transition-colors" aria-label="Reset password" title="Reset password">
                      <Key class="w-4 h-4" />
                    </button>
                  {/if}
                    <button onclick={() => handleDelete(user)} class="p-1.5 rounded-lg hover:bg-red-50 text-gray-400 hover:text-red transition-colors" aria-label="Delete">
                      <Trash2 class="w-4 h-4" />
                    </button>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <!-- Mobile cards -->
    <div class="sm:hidden space-y-3">
      {#each admins as user (user.id)}
        <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl p-4">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="font-semibold text-gray-900">{user.name}</div>
              <div class="text-sm text-gray-500 truncate">{user.email}</div>
            </div>
            {#if editingRoleUserId === user.id}
              <div class="flex items-center gap-1 flex-shrink-0">
                <select bind:value={editingRoleValue}
                  class="px-2 py-1 border border-gray-200 rounded-lg text-xs font-semibold bg-white focus:border-red outline-none">
                  <option value="user">User</option>
                  <option value="admin">Admin</option>
                </select>
                <button onclick={() => saveRole(user)} disabled={roleSaving}
                  class="p-1 rounded-md bg-emerald-50 text-emerald-600 hover:bg-emerald-100 transition-colors">
                  <Check class="w-3.5 h-3.5" />
                </button>
                <button onclick={cancelEditRole}
                  class="p-1 rounded-md bg-gray-100 text-gray-500 hover:bg-gray-200 transition-colors">
                  <X class="w-3.5 h-3.5" />
                </button>
              </div>
            {:else}
              <button onclick={() => startEditRole(user)}
                class="inline-flex px-2 py-0.5 rounded-full text-xs font-semibold flex-shrink-0 cursor-pointer hover:opacity-80 transition-opacity {user.role === 'admin' ? 'bg-gold-50 text-gold border border-gold-200' : 'bg-gray-100 text-gray-600 border border-gray-200'}">
                {user.role === 'admin' ? 'Admin' : 'User'}
              </button>
            {/if}
          </div>
          {#if user.role !== 'admin' || editingRoleUserId === user.id}
            <div class="flex items-center gap-2 mt-3 pt-3 border-t border-gray-100">
              <button onclick={() => openAssign(user)} class="flex-1 flex items-center justify-center gap-1.5 py-2 rounded-lg bg-blue-50 text-blue-600 text-xs font-semibold hover:bg-blue-100 transition-colors">
                <Building2 class="w-3.5 h-3.5" /> Weddings
              </button>
              <button onclick={() => openReset(user)} class="flex-1 flex items-center justify-center gap-1.5 py-2 rounded-lg bg-amber-50 text-amber-600 text-xs font-semibold hover:bg-amber-100 transition-colors">
                <Key class="w-3.5 h-3.5" /> Reset PW
              </button>
              <button onclick={() => handleDelete(user)} class="flex items-center justify-center p-2 rounded-lg bg-red-50 text-red hover:bg-red-100 transition-colors">
                <Trash2 class="w-3.5 h-3.5" />
              </button>
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<!-- Create User Modal -->
{#if showCreate}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="absolute inset-0 bg-black/30 backdrop-blur-md" onclick={() => showCreate = false} role="presentation"></div>
    <div class="relative bg-white/95 backdrop-blur-xl rounded-2xl shadow-2xl w-full max-w-md overflow-hidden">
      <div class="flex items-center justify-between p-5 border-b border-gray-100">
        <h3 class="font-bold text-gray-900">Add User</h3>
        <button onclick={() => showCreate = false} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
          <X class="w-4 h-4 text-gray-400" />
        </button>
      </div>
      <div class="p-5 space-y-4">
        <div>
          <label for="create-name" class="text-sm font-semibold text-gray-700 mb-1.5 block">Name</label>
          <input id="create-name" bind:value={formName} class="w-full px-4 py-2.5 border border-black/[0.08] rounded-xl text-sm bg-white/80 focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all min-h-[44px]" placeholder="Full name" />
        </div>
        <div>
          <label for="create-email" class="text-sm font-semibold text-gray-700 mb-1.5 block">Email</label>
          <input id="create-email" type="email" bind:value={formEmail} class="w-full px-4 py-2.5 border border-black/[0.08] rounded-xl text-sm bg-white/80 focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all min-h-[44px]" placeholder="email@example.com" />
        </div>
        <div>
          <label for="create-password" class="text-sm font-semibold text-gray-700 mb-1.5 block">Password</label>
          <input id="create-password" type="password" bind:value={formPassword} minlength="8" class="w-full px-4 py-2.5 border border-black/[0.08] rounded-xl text-sm bg-white/80 focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all min-h-[44px]" placeholder="Min 8 characters" />
          <PasswordRequirements password={formPassword} />
        </div>
        <div>
          <label for="create-confirm-password" class="text-sm font-semibold text-gray-700 mb-1.5 block">Confirm Password</label>
          <input id="create-confirm-password" type="password" bind:value={formConfirmPassword} minlength="8"
            class="w-full px-4 py-2.5 border border-black/[0.08] rounded-xl text-sm bg-white/80 focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all min-h-[44px] {createPasswordMismatch ? 'border-red !shadow-[0_0_0_3px_rgba(239,68,68,0.1)]' : ''}"
            placeholder="Repeat password" />
          {#if createPasswordMismatch}
            <p class="text-xs text-red mt-1">Passwords do not match</p>
          {/if}
        </div>
        <div>
          <label for="create-role" class="text-sm font-semibold text-gray-700 mb-1.5 block">Role</label>
          <select id="create-role" bind:value={formRole} class="w-full px-4 py-2.5 border border-black/[0.08] rounded-xl text-sm bg-white/80 focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all min-h-[44px]">
            <option value="user">User</option>
            <option value="admin">Admin</option>
          </select>
        </div>
      </div>
      <div class="flex gap-3 p-5 pt-0">
        <button onclick={handleCreate} disabled={saving || !canCreate}
          class="flex-1 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors disabled:opacity-50">
          {saving ? 'Creating...' : 'Create User'}
        </button>
        <button onclick={() => showCreate = false} class="px-5 py-2.5 border border-black/[0.06] bg-white/90 rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
          Cancel
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Assign Weddings Modal -->
{#if assignTarget}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="absolute inset-0 bg-black/30 backdrop-blur-md" onclick={() => assignTarget = null} role="presentation"></div>
    <div class="relative bg-white/95 backdrop-blur-xl rounded-2xl shadow-2xl w-full max-w-sm overflow-hidden">
      <div class="flex items-center justify-between p-5 border-b border-gray-100">
        <h3 class="font-bold text-gray-900">Assign Weddings</h3>
        <button onclick={() => assignTarget = null} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
          <X class="w-4 h-4 text-gray-400" />
        </button>
      </div>
      <div class="p-5 space-y-4">
        <p class="text-sm text-gray-600">Assign weddings to <strong>{assignTarget.name}</strong></p>
        {#if weddings.length === 0}
          <p class="text-sm text-gray-400 italic">No weddings available</p>
        {:else}
          <div class="space-y-2 max-h-60 overflow-y-auto">
            {#each weddings as w}
              <label class="flex items-center gap-2 p-2 rounded-lg hover:bg-gray-50 cursor-pointer">
                <input type="checkbox" value={w.id} bind:group={assignWeddingIds} class="rounded border-gray-300 text-deep-red focus:ring-deep-red/20" />
                <span class="text-sm text-gray-700">{w.name}</span>
              </label>
            {/each}
          </div>
        {/if}
      </div>
      <div class="flex gap-3 p-5 pt-0">
        <button onclick={handleAssign} disabled={assignSaving}
          class="flex-1 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors disabled:opacity-50">
          {assignSaving ? 'Saving...' : 'Save'}
        </button>
        <button onclick={() => assignTarget = null} class="px-5 py-2.5 border border-black/[0.06] bg-white/90 rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
          Cancel
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Reset Password Modal -->
{#if resetTarget}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="absolute inset-0 bg-black/30 backdrop-blur-md" onclick={() => resetTarget = null} role="presentation"></div>
    <div class="relative bg-white/95 backdrop-blur-xl rounded-2xl shadow-2xl w-full max-w-md overflow-hidden">
      <div class="flex items-center justify-between p-5 border-b border-gray-100">
        <h3 class="font-bold text-gray-900">Reset Password</h3>
        <button onclick={() => resetTarget = null} class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100 transition-colors">
          <X class="w-4 h-4 text-gray-400" />
        </button>
      </div>
      <div class="p-5 space-y-4">
        <p class="text-sm text-gray-600">Set new password for <strong>{resetTarget.name}</strong></p>
        <div>
          <label for="reset-password" class="text-sm font-semibold text-gray-700 mb-1.5 block">New Password</label>
          <input id="reset-password" type="password" bind:value={resetPassword_} minlength="8"
            class="w-full px-4 py-2.5 border border-black/[0.08] rounded-xl text-sm bg-white/80 focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all min-h-[44px]"
            placeholder="Min 8 characters" />
          <PasswordRequirements password={resetPassword_} />
        </div>
        <div>
          <label for="reset-confirm-password" class="text-sm font-semibold text-gray-700 mb-1.5 block">Confirm Password</label>
          <input id="reset-confirm-password" type="password" bind:value={resetConfirmPassword} minlength="8"
            class="w-full px-4 py-2.5 border border-black/[0.08] rounded-xl text-sm bg-white/80 focus:border-red focus:ring-2 focus:ring-red/10 outline-none transition-all min-h-[44px] {resetPasswordMismatch ? 'border-red !shadow-[0_0_0_3px_rgba(239,68,68,0.1)]' : ''}"
            placeholder="Repeat password" />
          {#if resetPasswordMismatch}
            <p class="text-xs text-red mt-1">Passwords do not match</p>
          {/if}
        </div>
      </div>
      <div class="flex gap-3 p-5 pt-0">
        <button onclick={handleReset} disabled={resetSaving || !canReset}
          class="flex-1 py-2.5 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors disabled:opacity-50">
          {resetSaving ? 'Updating...' : 'Update Password'}
        </button>
        <button onclick={() => resetTarget = null} class="px-5 py-2.5 border border-black/[0.06] bg-white/90 rounded-xl text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors">
          Cancel
        </button>
      </div>
    </div>
  </div>
{/if}
