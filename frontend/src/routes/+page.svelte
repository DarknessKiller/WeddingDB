<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { getAuth } from '$lib/stores';
  import { weddingId } from '$lib/stores/weddingId';
  import { get } from 'svelte/store';
  import { encodeId } from '$lib/utils/encode';

  onMount(() => {
    const { accessToken } = getAuth();
    if (!accessToken) {
      goto('/login', { replaceState: true });
    } else {
      const wid = get(weddingId);
      if (!wid) {
        // No wedding selected — go to login to pick one
        goto('/login', { replaceState: true });
      } else {
        goto(`/${encodeId(wid)}/dashboard`, { replaceState: true });
      }
    }
  });
</script>

<div class="min-h-screen flex items-center justify-center">
  <div class="w-8 h-8 border-2 border-gold/30 border-t-gold rounded-full animate-spin"></div>
</div>
