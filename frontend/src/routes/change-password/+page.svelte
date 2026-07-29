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

<div class="auth-page">
  <div class="auth-card">
    <div class="auth-hero">
      <div class="auth-logo">囍</div>
      <h1 class="auth-title">Change Password</h1>
      <p class="auth-subtitle">Please set a new password before continuing</p>
    </div>

    <form onsubmit={handleChangePassword} class="auth-form">
      <div class="form-field">
        <label for="password" class="form-label">New Password</label>
        <div class="password-wrap">
          <input
            id="password"
            type={showPassword ? 'text' : 'password'}
            bind:value={password}
            required
            minlength="8"
            class="form-input password-input"
            placeholder="Min 8 chars, letter + number + symbol"
          />
          <button type="button" onclick={() => showPassword = !showPassword} class="password-toggle">
            {#if showPassword}<EyeOff size={18} />{:else}<Eye size={18} />{/if}
          </button>
        </div>
      </div>

      <div class="form-field">
        <label for="confirmPassword" class="form-label">Confirm Password</label>
        <input
          id="confirmPassword"
          type={showPassword ? 'text' : 'password'}
          bind:value={confirmPassword}
          required
          minlength="8"
          class="form-input"
          placeholder="Repeat password"
        />
      </div>

      <button type="submit" disabled={loading || !password || !confirmPassword} class="auth-submit">
        {#if loading}<div class="btn-spinner"></div>{:else}<KeyRound size={18} />{/if}
        {loading ? 'Updating...' : 'Update Password'}
      </button>
    </form>
  </div>
</div>


