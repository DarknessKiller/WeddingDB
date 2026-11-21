<script lang="ts">
  import '../app.css';
  import ToastContainer from '#lib/components/ui/ToastContainer.svelte';
  import { page } from '$app/state';
  let { children } = $props();
</script>

<!-- Remount on wedding switch: the [wid] layout sets up stores, SSE and the queue in onMount. -->
{#key page.params.wid}
  {@render children()}
{/key}
<ToastContainer />

<style>
</style>
