<script lang="ts">
  import { goto } from '$app/navigation';
  import { addToast, getAuth, setAuth } from '$lib/stores';
  import { Eye, EyeOff, KeyRound } from 'lucide-svelte';

  let password = $state('');
  let confirmPassword = $state('');
  let showPassword = $state(false);
  let loading = $state(false);

  async function handleChangePassword(e: Event) {
    e.preventDefault();
    if (password !== confirmPassword) {
      addToast('Passwords do not match', 'error');
      return;
    }
    loading = true;
    try {
      const { accessToken } = getAuth();
      const res = await fetch('/api/auth/change-password', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${accessToken}`
        },
        body: JSON.stringify({ password })
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({ title: 'Failed to change password' }));
        addToast(err.title || 'Failed to change password', 'error');
        return;
      }
      addToast('Password changed successfully', 'success');
      goto('/', { replaceState: true });
    } catch {
      addToast('Network error', 'error');
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head><title>Change Password – WeddingDB</title></svelte:head>

<div class="min-h-screen flex items-center justify-center bg-warm-50 px-4">
  <div class="w-full max-w-sm">
    <div class="text-center mb-8">
      <div class="w-16 h-16 bg-deep-red rounded-2xl flex items-center justify-center mx-auto mb-4 shadow-lg">
        <span class="text-2xl font-bold text-white font-serif">W</span>
      </div>
      <h1 class="text-2xl font-bold text-gray-900 font-serif">Change Password</h1>
      <p class="text-sm text-gray-500 mt-1">Please set a new password before continuing</p>
    </div>

    <form onsubmit={handleChangePassword} class="bg-white rounded-2xl shadow-sm border border-gray-100 p-6 space-y-4">
      <div>
        <label for="password" class="block text-sm font-medium text-gray-700 mb-1">New Password</label>
        <div class="relative">
          <input
            id="password"
            type={showPassword ? 'text' : 'password'}
            bind:value={password}
            required
            minlength="8"
            class="w-full px-3 py-2 pr-10 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-deep-red/20 focus:border-deep-red transition-colors"
            placeholder="Min 8 chars, letter + number + symbol"
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

      <div>
        <label for="confirmPassword" class="block text-sm font-medium text-gray-700 mb-1">Confirm Password</label>
        <input
          id="confirmPassword"
          type={showPassword ? 'text' : 'password'}
          bind:value={confirmPassword}
          required
          minlength="8"
          class="w-full px-3 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-deep-red/20 focus:border-deep-red transition-colors"
          placeholder="Repeat password"
        />
      </div>

      <button
        type="submit"
        disabled={loading || !password || !confirmPassword}
        class="w-full flex items-center justify-center gap-2 bg-deep-red text-white py-2.5 rounded-lg font-medium hover:bg-deep-red/90 disabled:opacity-50 transition-colors"
      >
        {#if loading}
          <div class="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin"></div>
        {:else}
          <KeyRound size={18} />
        {/if}
        {loading ? 'Updating...' : 'Update Password'}
      </button>
    </form>
  </div>
</div>
