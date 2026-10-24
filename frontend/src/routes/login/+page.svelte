<script lang="ts">
  import { goto } from '$app/navigation';
  import { setAuth, addToast } from '$lib/stores';
  import { weddingId, setWeddingId } from '$lib/stores/weddingId';
  import { get } from 'svelte/store';
  import { t, locale } from '$lib/i18n';
  import Eye from 'lucide-svelte/icons/eye';
import EyeOff from 'lucide-svelte/icons/eye-off';
import LogIn from 'lucide-svelte/icons/log-in';
import Calendar from 'lucide-svelte/icons/calendar';
import ChevronRight from 'lucide-svelte/icons/chevron-right';

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
        const err = await res.json().catch(() => ({ title: t('toast.createFailed') }));
        addToast(err.title || t('toast.createFailed'), 'error');
        return;
      }
      const wedding = await res.json();
      addToast(t('toast.weddingCreated'), 'success');
      await selectWedding(wedding.id);
    } catch {
      addToast(t('toast.netError'), 'error');
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
        const err = await res.json().catch(() => ({ title: t('toast.loginFailed') }));
        addToast(err.title || t('toast.loginFailed'), 'error');
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
        addToast(t('toast.noWeddings'), 'error');
        return;
      }
      if (weddings.length === 1) {
        await selectWedding(weddings[0].id);
        return;
      }
      step = 'select';
    } catch (err) {
      addToast(t('toast.netError'), 'error');
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
        const err = await res.json().catch(() => ({ title: t('toast.selectFailed') }));
        addToast(err.title || t('toast.selectFailed'), 'error');
        return;
      }
      const data = await res.json();
      setAuth(data.accessToken, loginRefreshToken, loginRole, loginName);
      setWeddingId(weddingIdValue);
      addToast(t('toast.loginOk'), 'success');
      goto(`/${weddingIdValue}/dashboard`, { replaceState: true });
    } catch (err) {
      addToast(t('toast.netError'), 'error');
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head><title>{t('auth.title')} – WeddingDB</title></svelte:head>

<div class="auth-page">
  <div class="auth-card">
    <!-- Logo -->
    <div class="auth-hero">
      <div class="auth-logo">囍</div>
      <h1 class="auth-title">WeddingDB</h1>
      <p class="auth-subtitle">{t('auth.subtitle')}</p>
    </div>

    {#if step === 'login'}
      <form onsubmit={handleLogin} class="auth-form">
        <div class="form-field">
          <label for="email" class="form-label">{t('common.email')}</label>
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
          <label for="password" class="form-label">{t('auth.password')}</label>
          <div class="password-wrap">
            <input
              id="password"
              type={showPassword ? 'text' : 'password'}
              bind:value={password}
              required
              class="form-input password-input"
              placeholder={t("auth.passwordPlaceholder")}
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
          {loading ? t('auth.signingIn') : t('auth.signin')}
        </button>
      </form>

      <p class="auth-footer">
        {t('auth.noAccount')} <a href="/register" class="auth-link">{t('auth.register')}</a>
      </p>
    {:else}
      <div class="auth-form">
        {#if availableWeddings.length === 0 && loginRole === 'admin'}
          {#if !showCreate}
            <div class="empty-state">
              <Calendar class="empty-icon" />
              <p class="empty-text">{t('auth.emptyWeddings')}</p>
              <button onclick={() => showCreate = true} class="auth-submit">
                {t('auth.createWedding')}
              </button>
            </div>
          {:else}
            <h2 class="section-title">{t('auth.newWedding')}</h2>
            <div class="form-grid">
              <div class="form-field">
                <label for="wedding-name" class="form-label">{t('weddings.name')}</label>
                <input id="wedding-name" type="text" bind:value={newName} placeholder={t("auth.namePlaceholder")} class="form-input" />
              </div>
              <div class="form-field">
                <label for="wedding-date" class="form-label">{t('common.date')}</label>
                <input id="wedding-date" type="date" bind:value={newDate} class="form-input" />
              </div>
              <div class="form-row">
                <button onclick={handleCreateWedding} disabled={creating || !newName || !newDate} class="auth-submit">
                  {creating ? t('auth.creating') : t('auth.createContinue')}
                </button>
                <button onclick={() => showCreate = false} class="auth-cancel">{t('common.cancel')}</button>
              </div>
            </div>
          {/if}
        {:else}
          <h2 class="section-title">{t('auth.selectWedding')}</h2>
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
                  <div class="wedding-date">{w.date ? new Date(w.date).toLocaleDateString($locale === 'zh' ? 'zh-CN' : 'en-US') : t('weddings.noDate')}</div>
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


</style>
