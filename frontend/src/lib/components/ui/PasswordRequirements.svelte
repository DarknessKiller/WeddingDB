<script lang="ts">
  let { password = '' } = $props();

  const rules = [
    { label: '8+ characters', test: (p: string) => p.length >= 8 },
    { label: 'One letter', test: (p: string) => /[a-zA-Z]/.test(p) },
    { label: 'One number', test: (p: string) => /\d/.test(p) },
    { label: 'One symbol', test: (p: string) => /[^a-zA-Z0-9]/.test(p) },
  ];

  let passed = $derived(rules.filter(r => r.test(password)).length);
  let allValid = $derived(passed === rules.length);
</script>

{#if password.length > 0}
  <div class="req-list" class:all-valid={allValid}>
    {#each rules as rule}
      {@const valid = rule.test(password)}
      <div class="req-item" class:valid>
        <span class="req-dot">{valid ? '✓' : '○'}</span>
        <span class="req-label">{rule.label}</span>
      </div>
    {/each}
  </div>
{/if}

<style>
  .req-list {
    display: flex;
    flex-wrap: wrap;
    gap: 0.375rem 0.75rem;
    margin-top: 0.375rem;
  }
  .req-item {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    font-size: 0.6875rem;
    color: #9ca3af;
    transition: color 150ms ease;
  }
  .req-item.valid {
    color: #059669;
  }
  .req-dot {
    font-size: 0.625rem;
    line-height: 1;
  }
  .req-label {
    white-space: nowrap;
  }
</style>
