<script lang="ts">
  import { goto } from '$app/navigation';
  import { addToast } from '$lib/stores';
  import { Eye, EyeOff, UserPlus } from 'lucide-svelte';

  let name = $state('');
  let email = $state('');
  let password = $state('');
  let showPassword = $state(false);
  let loading = $state(false);

  async function handleRegister(e: Event) {
    e.preventDefault();
    loading = true;
    try {
      const res = await fetch('/api/auth/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, email, password })
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({ title: 'Registration failed' }));
        addToast(err.title || 'Registration failed', 'error');
        return;
      }
      addToast('Account created! Please sign in.', 'success');
      goto('/login', { replaceState: true });
    } catch {
      addToast('Network error', 'error');
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head><title>Register – WeddingDB</title></svelte:head>

<div class="auth-page">
  <div class="auth-card">
    <div class="auth-hero">
      <div class="auth-logo">囍</div>
      <h1 class="auth-title">WeddingDB</h1>
      <p class="auth-subtitle">Create your admin account</p>
    </div>

    <form onsubmit={handleRegister} class="auth-form">
      <div class="form-field">
        <label for="name" class="form-label">Full Name</label>
        <input id="name" type="text" bind:value={name} required class="form-input" placeholder="Your name" />
      </div>

      <div class="form-field">
        <label for="email" class="form-label">Email</label>
        <input id="email" type="email" bind:value={email} required class="form-input" placeholder="you@example.com" />
      </div>

      <div class="form-field">
        <label for="password" class="form-label">Password</label>
        <div class="password-wrap">
          <input
            id="password"
            type={showPassword ? 'text' : 'password'}
            bind:value={password}
            required
            minlength="8"
            class="form-input password-input"
            placeholder="Min 8 characters"
          />
          <button type="button" onclick={() => showPassword = !showPassword} class="password-toggle">
            {#if showPassword}<EyeOff size={18} />{:else}<Eye size={18} />{/if}
          </button>
        </div>
      </div>

      <button type="submit" disabled={loading} class="auth-submit">
        {#if loading}<div class="btn-spinner"></div>{:else}<UserPlus size={18} />{/if}
        {loading ? 'Creating account...' : 'Create account'}
      </button>

      <p class="auth-footer">
        Already have an account? <a href="/login" class="auth-link">Sign in</a>
      </p>
    </form>
  </div>
</div>

<style>
  .auth-page {
    min-height: 100dvh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 1rem;
    background: linear-gradient(180deg, #fef2f2 0%, #faf7f2 50%, white 100%);
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
  .auth-footer { text-align: center; font-size: 0.875rem; color: #6b7280; margin-top: 0.5rem; }
  .auth-link { color: #A11217; font-weight: 600; text-decoration: none; }
  .auth-link:hover { text-decoration: underline; }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
