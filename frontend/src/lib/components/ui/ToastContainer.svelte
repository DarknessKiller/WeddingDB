<script lang="ts">
  import { toasts, type Toast } from '$lib/stores';
  import { CheckCircle, XCircle, Info, X } from 'lucide-svelte';

  const icons: Record<string, typeof CheckCircle> = {
    success: CheckCircle,
    error: XCircle,
    info: Info,
  };

  const colors: Record<string, string> = {
    success: 'border-l-emerald-500',
    error: 'border-l-red-500',
    info: 'border-l-blue-500',
  };

  function getIcon(type: string) {
    return icons[type] ?? Info;
  }

  function getColor(type: string) {
    return colors[type] ?? 'border-l-blue-500';
  }
</script>

<div class="fixed top-5 right-5 z-[1000] flex flex-col gap-2.5">
  {#each $toasts as toast (toast.id)}
    <div
      class="bg-white border border-gray-200 rounded-lg shadow-lg px-5 py-3.5 flex items-center gap-3 min-w-[300px] animate-slide-in border-l-4 {getColor(toast.type)}"
    >
      {#if toast.type === 'success'}
        <CheckCircle class="w-5 h-5 flex-shrink-0 text-emerald-500" />
      {:else if toast.type === 'error'}
        <XCircle class="w-5 h-5 flex-shrink-0 text-red" />
      {:else}
        <Info class="w-5 h-5 flex-shrink-0 text-blue-500" />
      {/if}
      <span class="text-sm font-medium text-gray-700">{toast.message}</span>
    </div>
  {/each}
</div>

<style>
  @keyframes slide-in {
    from { opacity: 0; transform: translateX(40px); }
    to { opacity: 1; transform: translateX(0); }
  }
  .animate-slide-in {
    animation: slide-in 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  }
</style>
