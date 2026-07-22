<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';

  onMount(() => {
    goto('/dashboard', { replaceState: true });
  });
</script>

<div class="min-h-screen flex items-center justify-center">
  <div class="w-8 h-8 border-2 border-gold/30 border-t-gold rounded-full animate-spin"></div>
</div>
