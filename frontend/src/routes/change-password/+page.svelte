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

<style>
  .auth-page {
    min-height: 100dvh; display: flex; align-items: center; justify-content: center;
    padding: 1rem; background: linear-gradient(180deg, #fef2f2 0%, #faf7f2 50%, white 100%);
  }
  .auth-card { width: 100%; max-width: 24rem; }
  .auth-hero { text-align: center; margin-bottom: 2rem; }
  .auth-logo {
    width: 4rem; height: 4rem; background: #A11217; border-radius: 1rem;
    display: flex; align-items: center; justify-content: center;
    margin: 0 auto 1rem; color: #D4AF37; font-size: 1.5rem; font-weight: 700;
    font-family: 'Noto Serif SC', 'Songti SC', serif;
    box-shadow: 0 8px 32px rgba(161, 18, 23, 0.25);
  }
  .auth-title { font-size: 1.5rem; font-weight: 800; color: #111827; letter-spacing: -0.025em; font-family: 'Noto Serif SC', 'Songti SC', serif; }
  .auth-subtitle { font-size: 0.875rem; color: #6b7280; margin-top: 0.25rem; }
  .auth-form {
    background: rgba(255, 255, 255, 0.92); backdrop-filter: blur(20px) saturate(180%);
    -webkit-backdrop-filter: blur(20px) saturate(180%); border: 1px solid rgba(0, 0, 0, 0.06);
    border-radius: 1.25rem; padding: 1.5rem; display: flex; flex-direction: column; gap: 1rem;
    box-shadow: 0 4px 24px rgba(0, 0, 0, 0.06);
  }
  .form-field { display: flex; flex-direction: column; gap: 0.375rem; }
  .form-label { font-size: 0.8125rem; font-weight: 600; color: #374151; }
  .form-input {
    width: 100%; padding: 0.75rem 1rem; border: 1.5px solid rgba(0, 0, 0, 0.08);
    border-radius: 0.75rem; font-size: 0.9375rem; color: #111827; background: white;
    outline: none; transition: border-color 200ms ease, box-shadow 200ms ease, transform 100ms ease;
    min-height: 48px;
  }
  .form-input:focus { border-color: #A11217; box-shadow: 0 0 0 3px rgba(161, 18, 23, 0.1); }
  .form-input:active { transform: scale(0.99); }
  .form-input::placeholder { color: #9ca3af; }
  .password-wrap { position: relative; }
  .password-input { padding-right: 3rem; }
  .password-toggle {
    position: absolute; right: 0.5rem; top: 50%; transform: translateY(-50%);
    padding: 0.5rem; color: #9ca3af; min-width: 44px; min-height: 44px;
    display: flex; align-items: center; justify-content: center; border-radius: 0.5rem;
  }
  .password-toggle:hover { color: #4b5563; }
  .auth-submit {
    display: flex; align-items: center; justify-content: center; gap: 0.5rem;
    width: 100%; padding: 0.875rem; background: #A11217; color: white;
    border-radius: 0.75rem; font-size: 0.9375rem; font-weight: 600;
    transition: background 100ms ease, transform 100ms ease, opacity 100ms ease;
    min-height: 48px;
  }
  .auth-submit:active { transform: scale(0.98); }
  .auth-submit:disabled { opacity: 0.5; }
  .btn-spinner {
    width: 1.125rem; height: 1.125rem; border: 2px solid rgba(255, 255, 255, 0.3);
    border-top-color: white; border-radius: 50%; animation: spin 600ms linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
