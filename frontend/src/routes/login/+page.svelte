<script lang="ts">
  import { goto } from '$app/navigation';
  import { setAuth, addToast } from '$lib/stores';
  import { weddingId, setWeddingId } from '$lib/stores/weddingId';
  import { get } from 'svelte/store';
  import { Eye, EyeOff, LogIn, Calendar, ChevronRight } from 'lucide-svelte';

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
          step = 'select';
          return;
        }
        addToast('No weddings assigned to your account', 'error');
        return;
      }
      if (weddings.length === 1) {
        await selectWedding(weddings[0].id);
        return;
      }
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
      goto(`/${weddingIdValue}/dashboard`, { replaceState: true });
    } catch (err) {
      addToast('Network error', 'error');
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head><title>Sign In – WeddingDB</title></svelte:head>

<div class="auth-page">
  <div class="auth-card">
    <!-- Logo -->
    <div class="auth-hero">
      <div class="auth-logo">囍</div>
      <h1 class="auth-title">WeddingDB</h1>
      <p class="auth-subtitle">Sign in to manage your wedding</p>
    </div>

    {#if step === 'login'}
      <form onsubmit={handleLogin} class="auth-form">
        <div class="form-field">
          <label for="email" class="form-label">Email</label>
          <input
            id="email"
            type="email"
            bind:value={email}
            required
            class="form-input"
            placeholder="admin@weddingdb.local"
          />
        </div>

        <div class="form-field">
          <label for="password" class="form-label">Password</label>
          <div class="password-wrap">
            <input
              id="password"
              type={showPassword ? 'text' : 'password'}
              bind:value={password}
              required
              class="form-input password-input"
              placeholder="Enter password"
            />
            <button
              type="button"
              onclick={() => showPassword = !showPassword}
              class="password-toggle"
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
          class="auth-submit"
        >
          {#if loading}
            <div class="btn-spinner"></div>
          {:else}
            <LogIn size={18} />
          {/if}
          {loading ? 'Signing in...' : 'Sign in'}
        </button>
      </form>

      <p class="auth-footer">
        Need an account? <a href="/register" class="auth-link">Register</a>
      </p>
    {:else}
      <div class="auth-form">
        {#if availableWeddings.length === 0 && loginRole === 'admin'}
          {#if !showCreate}
            <div class="empty-state">
              <Calendar class="empty-icon" />
              <p class="empty-text">No weddings yet. Create your first wedding to get started.</p>
              <button onclick={() => showCreate = true} class="auth-submit">
                Create Wedding
              </button>
            </div>
          {:else}
            <h2 class="section-title">New Wedding</h2>
            <div class="form-grid">
              <div class="form-field">
                <label for="wedding-name" class="form-label">Wedding Name</label>
                <input id="wedding-name" type="text" bind:value={newName} placeholder="e.g. John & Jane's Wedding" class="form-input" />
              </div>
              <div class="form-field">
                <label for="wedding-date" class="form-label">Date</label>
                <input id="wedding-date" type="date" bind:value={newDate} class="form-input" />
              </div>
              <div class="form-row">
                <button onclick={handleCreateWedding} disabled={creating || !newName || !newDate} class="auth-submit">
                  {creating ? 'Creating...' : 'Create & Continue'}
                </button>
                <button onclick={() => showCreate = false} class="auth-cancel">Cancel</button>
              </div>
            </div>
          {/if}
        {:else}
          <h2 class="section-title">Select a wedding</h2>
          <div class="wedding-list">
            {#each availableWeddings as w}
              <button
                onclick={() => selectWedding(w.id)}
                disabled={loading}
                class="wedding-item"
              >
                <div class="wedding-icon">
                  <Calendar class="w-5 h-5" />
                </div>
                <div class="wedding-info">
                  <div class="wedding-name">{w.name}</div>
                  <div class="wedding-date">{w.date ? new Date(w.date).toLocaleDateString() : 'No date'}</div>
                </div>
                <ChevronRight class="wedding-chevron" />
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
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

  .auth-card {
    width: 100%;
    max-width: 24rem;
  }

  .auth-hero {
    text-align: center;
    margin-bottom: 2rem;
  }

  .auth-logo {
    width: 4rem;
    height: 4rem;
    background: #A11217;
    border-radius: 1rem;
    display: flex;
    align-items: center;
    justify-content: center;
    margin: 0 auto 1rem;
    color: #D4AF37;
    font-size: 1.5rem;
    font-weight: 700;
    font-family: 'Noto Serif SC', 'Songti SC', serif;
    box-shadow: 0 8px 32px rgba(161, 18, 23, 0.25);
  }

  .auth-title {
    font-size: 1.5rem;
    font-weight: 800;
    color: #111827;
    letter-spacing: -0.025em;
    font-family: 'Noto Serif SC', 'Songti SC', serif;
  }

  .auth-subtitle {
    font-size: 0.875rem;
    color: #6b7280;
    margin-top: 0.25rem;
  }

  .auth-form {
    background: rgba(255, 255, 255, 0.92);
    backdrop-filter: blur(20px) saturate(180%);
    -webkit-backdrop-filter: blur(20px) saturate(180%);
    border: 1px solid rgba(0, 0, 0, 0.06);
    border-radius: 1.25rem;
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
    box-shadow: 0 4px 24px rgba(0, 0, 0, 0.06);
  }

  .form-field {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
  }

  .form-label {
    font-size: 0.8125rem;
    font-weight: 600;
    color: #374151;
  }

  .form-input {
    width: 100%;
    padding: 0.75rem 1rem;
    border: 1.5px solid rgba(0, 0, 0, 0.08);
    border-radius: 0.75rem;
    font-size: 0.9375rem;
    color: #111827;
    background: white;
    outline: none;
    transition: border-color 200ms ease, box-shadow 200ms ease, transform 100ms ease;
    min-height: 48px;
  }

  .form-input:focus {
    border-color: #A11217;
    box-shadow: 0 0 0 3px rgba(161, 18, 23, 0.1);
  }

  .form-input:active {
    transform: scale(0.99);
  }

  .form-input::placeholder {
    color: #9ca3af;
  }

  .password-wrap {
    position: relative;
  }

  .password-input {
    padding-right: 3rem;
  }

  .password-toggle {
    position: absolute;
    right: 0.5rem;
    top: 50%;
    transform: translateY(-50%);
    padding: 0.5rem;
    color: #9ca3af;
    min-width: 44px;
    min-height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 0.5rem;
    transition: color 100ms ease;
  }

  .password-toggle:hover {
    color: #4b5563;
  }

  .auth-submit {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.875rem;
    background: #A11217;
    color: white;
    border-radius: 0.75rem;
    font-size: 0.9375rem;
    font-weight: 600;
    transition: background 100ms ease, transform 100ms ease, opacity 100ms ease;
    min-height: 48px;
  }

  .auth-submit:active {
    transform: scale(0.98);
  }

  .auth-submit:disabled {
    opacity: 0.5;
  }

  .auth-cancel {
    padding: 0.875rem;
    border: 1.5px solid rgba(0, 0, 0, 0.08);
    border-radius: 0.75rem;
    font-size: 0.9375rem;
    font-weight: 500;
    color: #4b5563;
    background: white;
    transition: background 100ms ease, transform 100ms ease;
  }

  .auth-cancel:active {
    transform: scale(0.98);
  }

  .btn-spinner {
    width: 1.125rem;
    height: 1.125rem;
    border: 2px solid rgba(255, 255, 255, 0.3);
    border-top-color: white;
    border-radius: 50%;
    animation: spin 600ms linear infinite;
  }

  .auth-footer {
    text-align: center;
    font-size: 0.875rem;
    color: #6b7280;
    margin-top: 1rem;
  }

  .auth-link {
    color: #A11217;
    font-weight: 600;
    text-decoration: none;
  }

  .auth-link:hover {
    text-decoration: underline;
  }

  .section-title {
    font-size: 0.875rem;
    font-weight: 600;
    color: #111827;
  }

  .form-grid {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .form-row {
    display: flex;
    gap: 0.5rem;
  }

  .form-row .auth-submit {
    flex: 1;
  }

  .form-row .auth-cancel {
    flex: 1;
  }

  .empty-state {
    text-align: center;
    padding: 1rem 0;
  }

  .empty-icon {
    width: 2.5rem;
    height: 2.5rem;
    color: #d1d5db;
    margin: 0 auto 0.75rem;
  }

  .empty-text {
    font-size: 0.875rem;
    color: #6b7280;
    margin-bottom: 1rem;
  }

  .wedding-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .wedding-item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.875rem;
    border: 1.5px solid rgba(0, 0, 0, 0.06);
    border-radius: 0.875rem;
    text-align: left;
    transition: border-color 150ms ease, background 150ms ease, transform 100ms ease;
    width: 100%;
  }

  .wedding-item:active {
    transform: scale(0.98);
  }

  .wedding-item:hover {
    border-color: rgba(161, 18, 23, 0.3);
    background: #fef2f2;
  }

  .wedding-icon {
    width: 2.5rem;
    height: 2.5rem;
    border-radius: 0.625rem;
    background: #FDEAEA;
    border: 1px solid #FAC5C5;
    display: flex;
    align-items: center;
    justify-content: center;
    color: #A11217;
    flex-shrink: 0;
  }

  .wedding-info {
    flex: 1;
    min-width: 0;
  }

  .wedding-name {
    font-weight: 600;
    color: #111827;
    font-size: 0.875rem;
  }

  .wedding-date {
    font-size: 0.75rem;
    color: #6b7280;
  }

  .wedding-chevron {
    width: 1rem;
    height: 1rem;
    color: #d1d5db;
    flex-shrink: 0;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
