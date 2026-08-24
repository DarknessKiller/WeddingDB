<script lang="ts">
  import { goto } from '$app/navigation';
  import { addToast } from '$lib/stores';
  import { Eye, EyeOff, UserPlus, Mail } from 'lucide-svelte';
  import PasswordRequirements from '$lib/components/ui/PasswordRequirements.svelte';

  let name = $state('');
  let email = $state('');
  let password = $state('');
  let confirmPassword = $state('');
  let showPassword = $state(false);
  let loading = $state(false);

  let passwordMismatch = $derived(confirmPassword.length > 0 && password !== confirmPassword);
  let canSubmit = $derived(
    name.length > 0 &&
    email.length > 0 &&
    password.length >= 8 &&
    password === confirmPassword &&
    /[a-zA-Z]/.test(password) &&
    /\d/.test(password) &&
    /[^a-zA-Z0-9]/.test(password)
  );

  async function handleRegister(e: Event) {
    e.preventDefault();
    if (!canSubmit) return;
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
      addToast('Account created! Email mrdarknesskiller@protonmail.com to get your wedding set up.', 'success');
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
      <p class="auth-subtitle">Create your account</p>
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
        {#if loading}<div class="btn-spinner"></div>{:else}<UserPlus size={18} />{/if}
        {loading ? 'Creating account...' : 'Create account'}
      </button>

      <p class="auth-footer">
        Already have an account? <a href="/login" class="auth-link">Sign in</a>
      </p>
    </form>

    <div class="contact-note">
      <Mail size={14} />
      <span>After registering, email <strong>mrdarknesskiller@protonmail.com</strong> to get your wedding set up.</span>
    </div>
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
  .contact-note {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    margin-top: 1.25rem;
    padding: 0.875rem 1rem;
    background: rgba(255, 255, 255, 0.72);
    backdrop-filter: blur(12px);
    border: 1px solid rgba(0, 0, 0, 0.06);
    border-radius: 0.875rem;
    font-size: 0.75rem;
    color: #6b7280;
    line-height: 1.5;
  }
  .contact-note :global(svg) {
    flex-shrink: 0;
    margin-top: 1px;
    color: #9ca3af;
  }
  .contact-note strong {
    color: #374151;
    font-weight: 600;
  }
</style>
