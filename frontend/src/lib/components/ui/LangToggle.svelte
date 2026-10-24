<script lang="ts">
  import { locale, setLocale } from '$lib/i18n';
  function toggle() {
    setLocale($locale === 'zh' ? 'en' : 'zh');
  }
</script>

<button class="lang-toggle" onclick={toggle} aria-label="Toggle language 中文 / English">
  {$locale === 'zh' ? 'EN' : '中文'}
</button>

<style>
  .lang-toggle {
    flex-shrink: 0;
    min-height: 44px;
    padding: 0 0.9rem;
    border: 1px solid rgba(0, 0, 0, 0.1);
    border-radius: 999px;
    background: white;
    color: #374151;
    font-size: 0.85rem;
    font-weight: 600;
    cursor: pointer;
  }
  .lang-toggle:hover { background: #f9fafb; }
  .lang-toggle:focus-visible { outline: 2px solid #A11217; outline-offset: 2px; }
</style>
