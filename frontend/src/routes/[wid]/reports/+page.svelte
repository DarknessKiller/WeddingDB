<script lang="ts">
  import { page } from '$app/state';
  import { weddingTitle } from '$lib/stores/weddingTitle';
  import { weddingId } from '$lib/stores/weddingId';
  import { exportAngpaoReport } from '$lib/api/reports';
  import { track } from '$lib/analytics';
  import { Download, FileSpreadsheet, FileText, Loader2 } from 'lucide-svelte';

  let exporting = $state<'csv' | 'xlsx' | null>(null);
  let error = $state('');

  async function handleExport(format: 'csv' | 'xlsx') {
    exporting = format;
    error = '';
    try {
      await exportAngpaoReport($weddingId, format);
      track('report_exported', { wedding_id: $weddingId, format });
    } catch (e: any) {
      error = e.message ?? 'Export failed';
    } finally {
      exporting = null;
    }
  }
</script>

<svelte:head>
  <title>{$weddingTitle ? `${$weddingTitle} – Reports` : 'Reports – WeddingDB'}</title>
</svelte:head>

<div class="p-4 sm:p-7 max-w-2xl mx-auto">
  <h1 class="text-xl font-bold text-gray-900 mb-6" style="letter-spacing: -0.02em;">Reports</h1>

  <div class="space-y-4">
    <!-- Angpao Report Card -->
    <div class="bg-white/90 backdrop-blur-xl border border-black/[0.06] rounded-2xl p-5 sm:p-6 shadow-sm">
      <div class="flex items-center gap-3 mb-4">
        <div class="w-10 h-10 rounded-xl bg-red flex items-center justify-center text-white">
          <Download class="w-5 h-5" />
        </div>
        <div>
          <h3 class="font-semibold text-gray-900 text-sm">Angpao Report</h3>
          <p class="text-xs text-gray-500">Guest contributions, table totals, and gift items</p>
        </div>
      </div>

      {#if error}
        <div class="mb-4 px-3 py-2 bg-red-50 border border-red-200 rounded-xl text-xs text-red-600">
          {error}
        </div>
      {/if}

      <p class="text-xs text-gray-500 mb-4">
        Includes per-guest angpao amounts, gift items, check-in status, and summary totals per table.
      </p>

      <div class="flex gap-3">
        <button
          onclick={() => handleExport('csv')}
          disabled={exporting !== null}
          class="flex-1 px-4 py-3 bg-gray-50 hover:bg-gray-100 border border-gray-200 rounded-xl text-sm font-semibold text-gray-700 transition-colors disabled:opacity-50 flex items-center justify-center gap-2"
        >
          {#if exporting === 'csv'}
            <Loader2 class="w-4 h-4 animate-spin" /> Generating...
          {:else}
            <FileText class="w-4 h-4" /> CSV
          {/if}
        </button>
        <button
          onclick={() => handleExport('xlsx')}
          disabled={exporting !== null}
          class="flex-1 px-4 py-3 bg-red text-white rounded-xl text-sm font-semibold hover:bg-red-light transition-colors disabled:opacity-50 flex items-center justify-center gap-2"
        >
          {#if exporting === 'xlsx'}
            <Loader2 class="w-4 h-4 animate-spin" /> Generating...
          {:else}
            <FileSpreadsheet class="w-4 h-4" /> Excel
          {/if}
        </button>
      </div>
    </div>
  </div>
</div>
