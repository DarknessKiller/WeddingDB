<script lang="ts">
  import type { RSVPStatus } from '$lib/types';

  let { status }: { status: RSVPStatus } = $props();

  const styles: Record<string, string> = {
    confirmed: 'bg-emerald-50 text-emerald-700 border-emerald-200',
    pending: 'bg-amber-50 text-amber-700 border-amber-200',
    declined: 'bg-red-50 text-red-700 border-red-200',
    no_response: 'bg-gray-50 text-gray-500 border-gray-200',
  };

  const labels: Record<string, string> = {
    confirmed: 'Confirmed',
    pending: 'Pending',
    declined: 'Declined',
    no_response: 'No Response',
  };
</script>

<span class="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-semibold border {styles[status]}">
  <span class="w-1.5 h-1.5 rounded-full bg-current"></span>
  {labels[status]}
</span>
