<script lang="ts">
  import '../app.css';
  import ToastContainer from '$lib/components/ui/ToastContainer.svelte';
  import { browser } from '$app/environment';
  import { track } from '$lib/analytics';
  let { children } = $props();

  if (browser) track('app_opened');
</script>

{@render children()}
<ToastContainer />

<style>
</style>
