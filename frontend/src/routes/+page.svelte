<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { weddingId } from '$lib/stores/weddingId';
  import { get } from 'svelte/store';
  import { encodeId } from '$lib/utils/encode';
  import { validateToken } from '$lib/utils/auth';

  onMount(async () => {
    if (!await validateToken()) return;
    const wid = get(weddingId);
    if (!wid) {
      goto('/login', { replaceState: true });
    } else {
      goto(`/${encodeId(wid)}/dashboard`, { replaceState: true });
    }
  });
</script>

<div class="min-h-screen flex items-center justify-center">
  <div class="w-8 h-8 border-2 border-gold/30 border-t-gold rounded-full animate-spin"></div>
</div>
