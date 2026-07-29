<script lang="ts">
  import { goto } from '$app/navigation';
  import { addToast, getAuth } from '$lib/stores';
  import { Eye, EyeOff, KeyRound } from 'lucide-svelte';
  import PasswordRequirements from '$lib/components/ui/PasswordRequirements.svelte';

  let password = $state('');
  let confirmPassword = $state('');
  let showPassword = $state(false);
  let loading = $state(false);

  let passwordMismatch = $derived(confirmPassword.length > 0 && password !== confirmPassword);
  let canSubmit = $derived(
    password.length >= 8 &&
    password === confirmPassword &&
    /[a-zA-Z]/.test(password) &&
    /\d/.test(password) &&
    /[^a-zA-Z0-9]/.test(password)
  );

  async function handleChangePassword(e: Event) {
    e.preventDefault();
    if (!canSubmit) return;
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
        <PasswordRequirements {password} />
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
          class:input-error={passwordMismatch}
          placeholder="Repeat password"
        />
        {#if passwordMismatch}
          <p class="field-error">Passwords do not match</p>
        {/if}
      </div>

      <button type="submit" disabled={loading || !canSubmit} class="auth-submit">
        {#if loading}<div class="btn-spinner"></div>{:else}<KeyRound size={18} />{/if}
        {loading ? 'Updating...' : 'Update Password'}
      </button>
    </form>
  </div>
</div>

<style>
  .input-error {
    border-color: #ef4444 !important;
    box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.1) !important;
  }
  .field-error {
    font-size: 0.75rem;
    color: #ef4444;
    margin-top: 0.25rem;
  }
</style>
