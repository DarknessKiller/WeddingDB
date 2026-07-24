<script lang="ts">
  import { goto } from '$app/navigation';
  import { setAuth, addToast } from '$lib/stores';
  import { Eye, EyeOff, LogIn } from 'lucide-svelte';

  let email = $state('admin@weddingdb.local');
  let password = $state('admin123');
  let showPassword = $state(false);
  let loading = $state(false);

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
      setAuth(data.accessToken, data.refreshToken, data.role ?? '', data.name ?? '');
      addToast('Login successful', 'success');
      goto('/dashboard', { replaceState: true });
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

    <!-- Form -->
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
  </div>
</div>
