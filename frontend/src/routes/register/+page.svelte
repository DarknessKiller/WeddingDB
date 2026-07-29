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


