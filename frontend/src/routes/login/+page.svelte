<script lang="ts">
  import { goto } from '$app/navigation';
  import { setAuth, addToast } from '$lib/stores';
  import { weddingId, setWeddingId } from '$lib/stores/weddingId';
  import { get } from 'svelte/store';
  import { Eye, EyeOff, LogIn, Calendar, ChevronRight } from 'lucide-svelte';
  import { encodeId } from '$lib/utils/encode';

  let email = $state('');
  let password = $state('');
  let showPassword = $state(false);
  let loading = $state(false);

  let step = $state<'login' | 'select'>('login');
  let availableWeddings = $state<{ id: string; name: string; date: string }[]>([]);
  let loginRole = $state('');
  let loginName = $state('');
  let loginAccessToken = $state('');
  let loginRefreshToken = $state('');

  let showCreate = $state(false);
  let newName = $state('');
  let newDate = $state('');
  let creating = $state(false);

  async function handleCreateWedding() {
    if (!newName || !newDate) return;
    creating = true;
    try {
      const res = await fetch('/api/weddings', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${loginAccessToken}`
        },
        body: JSON.stringify({ name: newName, date: newDate })
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({ title: 'Failed to create wedding' }));
        addToast(err.title || 'Failed to create wedding', 'error');
        return;
      }
      const wedding = await res.json();
      addToast('Wedding created', 'success');
      // Auto-select the new wedding
      await selectWedding(wedding.id);
    } catch {
      addToast('Network error', 'error');
    } finally {
      creating = false;
    }
  }

  async function handleLogin(e: Event) {
    e.preventDefault();
    loading = true;
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password })
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({ title: 'Login failed' }));
        addToast(err.title || 'Login failed', 'error');
        return;
      }
      const data = await res.json();
      loginAccessToken = data.accessToken;
      loginRefreshToken = data.refreshToken;
      loginRole = data.role ?? '';
      loginName = data.name ?? '';

      if (data.forcePasswordChange) {
        setAuth(loginAccessToken, loginRefreshToken, loginRole, loginName);
        goto('/change-password', { replaceState: true });
        return;
      }

      const weddings = data.weddings ?? [];
      availableWeddings = weddings;

      if (weddings.length === 0) {
        if (loginRole === 'admin') {
          // Admin with no weddings — show selector with create option
          step = 'select';
          return;
        }
        addToast('No weddings assigned to your account', 'error');
        return;
      }
      if (weddings.length === 1) {
        // Auto-select single wedding
        await selectWedding(weddings[0].id);
        return;
      }
      // Multiple weddings — show selector
      step = 'select';
    } catch (err) {
      addToast('Network error', 'error');
    } finally {
      loading = false;
    }
  }

  async function selectWedding(weddingIdValue: string) {
    loading = true;
    try {
      const res = await fetch('/api/auth/select-wedding', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${loginAccessToken}`
        },
        body: JSON.stringify({ weddingId: weddingIdValue })
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({ title: 'Failed to select wedding' }));
        addToast(err.title || 'Failed to select wedding', 'error');
        return;
      }
      const data = await res.json();
      setAuth(data.accessToken, loginRefreshToken, loginRole, loginName);
      setWeddingId(weddingIdValue);
      addToast('Login successful', 'success');
      goto(`/${encodeId(weddingIdValue)}/dashboard`, { replaceState: true });
    } catch (err) {
      addToast('Network error', 'error');
    } finally {
      loading = false;
    }
  }
</script>

<div class="min-h-screen flex items-center justify-center bg-warm-50 px-4">
  <div class="w-full max-w-sm">
    <!-- Logo -->
    <div class="text-center mb-8">
      <div class="w-16 h-16 bg-deep-red rounded-2xl flex items-center justify-center mx-auto mb-4 shadow-lg">
        <span class="text-2xl font-bold text-white font-serif">W</span>
      </div>
      <h1 class="text-2xl font-bold text-gray-900 font-serif">WeddingDB</h1>
      <p class="text-sm text-gray-500 mt-1">Sign in to manage your wedding</p>
    </div>

    {#if step === 'login'}
      <!-- Login Form -->
      <form onsubmit={handleLogin} class="bg-white rounded-2xl shadow-sm border border-gray-100 p-6 space-y-4">
        <div>
          <label for="email" class="block text-sm font-medium text-gray-700 mb-1">Email</label>
          <input
            id="email"
            type="email"
            bind:value={email}
            required
            class="w-full px-3 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-deep-red/20 focus:border-deep-red transition-colors"
            placeholder="admin@weddingdb.local"
          />
        </div>

        <div>
          <label for="password" class="block text-sm font-medium text-gray-700 mb-1">Password</label>
          <div class="relative">
            <input
              id="password"
              type={showPassword ? 'text' : 'password'}
              bind:value={password}
              required
              class="w-full px-3 py-2 pr-10 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-deep-red/20 focus:border-deep-red transition-colors"
              placeholder="Enter password"
            />
            <button
              type="button"
              onclick={() => showPassword = !showPassword}
              class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600"
            >
              {#if showPassword}
                <EyeOff size={18} />
              {:else}
                <Eye size={18} />
              {/if}
            </button>
          </div>
        </div>

        <button
          type="submit"
          disabled={loading}
          class="w-full flex items-center justify-center gap-2 bg-deep-red text-white py-2.5 rounded-lg font-medium hover:bg-deep-red/90 disabled:opacity-50 transition-colors"
        >
          {#if loading}
            <div class="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin"></div>
          {:else}
            <LogIn size={18} />
          {/if}
          {loading ? 'Signing in...' : 'Sign in'}
        </button>
      </form>

      <p class="text-center text-sm text-gray-500 mt-4">
        Need an account? <a href="/register" class="text-deep-red font-medium hover:underline">Register</a>
      </p>
    {:else}
      <!-- Wedding Selector -->
      <div class="bg-white rounded-2xl shadow-sm border border-gray-100 p-6">
        {#if availableWeddings.length === 0 && loginRole === 'admin'}
          {#if !showCreate}
            <div class="text-center py-4">
              <Calendar class="w-10 h-10 text-gray-300 mx-auto mb-3" />
              <p class="text-sm text-gray-500 mb-4">No weddings yet. Create your first wedding to get started.</p>
              <button onclick={() => showCreate = true}
                class="w-full px-4 py-2.5 bg-deep-red text-white rounded-xl text-sm font-semibold hover:bg-deep-red/90 transition-colors">
                Create Wedding
              </button>
            </div>
          {:else}
            <h2 class="text-sm font-semibold text-gray-900 mb-4">New Wedding</h2>
            <div class="space-y-3">
              <div>
                <label for="wedding-name" class="block text-xs font-medium text-gray-600 mb-1">Wedding Name</label>
                <input id="wedding-name" type="text" bind:value={newName} placeholder="e.g. John & Jane's Wedding"
                  class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-deep-red/20 focus:border-deep-red" />
              </div>
              <div>
                <label for="wedding-date" class="block text-xs font-medium text-gray-600 mb-1">Date</label>
                <input id="wedding-date" type="date" bind:value={newDate}
                  class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-deep-red/20 focus:border-deep-red" />
              </div>
              <div class="flex gap-2">
                <button onclick={handleCreateWedding} disabled={creating || !newName || !newDate}
                  class="flex-1 py-2.5 bg-deep-red text-white rounded-xl text-sm font-semibold hover:bg-deep-red/90 disabled:opacity-50 transition-colors">
                  {creating ? 'Creating...' : 'Create & Continue'}
                </button>
                <button onclick={() => showCreate = false}
                  class="px-4 py-2.5 border border-gray-200 rounded-xl text-sm font-medium text-gray-600 hover:bg-gray-50 transition-colors">
                  Cancel
                </button>
              </div>
            </div>
          {/if}
        {:else}
          <h2 class="text-sm font-semibold text-gray-900 mb-4">Select a wedding</h2>
          <div class="space-y-2">
            {#each availableWeddings as w}
              <button
                onclick={() => selectWedding(w.id)}
                disabled={loading}
                class="w-full flex items-center gap-3 p-3 rounded-xl border border-gray-200 hover:border-deep-red hover:bg-red-50 transition-all text-left group"
              >
                <div class="w-10 h-10 rounded-lg bg-red-50 border border-red-100 flex items-center justify-center text-red flex-shrink-0">
                  <Calendar class="w-5 h-5" />
                </div>
                <div class="flex-1 min-w-0">
                  <div class="font-semibold text-gray-900 text-sm">{w.name}</div>
                  <div class="text-xs text-gray-500">{w.date ? new Date(w.date).toLocaleDateString() : 'No date'}</div>
                </div>
                <ChevronRight class="w-4 h-4 text-gray-400 group-hover:text-deep-red transition-colors" />
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
