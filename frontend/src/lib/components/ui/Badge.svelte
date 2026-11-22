<script lang="ts">
  import type { RSVPStatus } from '#lib/types/index.ts';

  let { status }: { status: RSVPStatus } = $props();

  const styles: Record<string, string> = {
    confirmed: 'badge-confirmed',
    pending: 'badge-pending',
    declined: 'badge-declined',
    no_response: 'badge-none',
  };

  const labels: Record<string, string> = {
    confirmed: 'Confirmed',
    pending: 'Pending',
    declined: 'Declined',
    no_response: 'No Response',
  };
</script>

<span class="badge {styles[status]}">
  <span class="badge-dot"></span>
  {labels[status]}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.25rem 0.625rem;
    border-radius: 9999px;
    font-size: 0.75rem;
    font-weight: 600;
    line-height: 1;
  }

  .badge-dot {
    width: 0.375rem;
    height: 0.375rem;
    border-radius: 50%;
    background: currentColor;
  }

  .badge-confirmed {
    background: #ECFDF5;
    color: var(--color-status-confirmed);
    border: 1px solid #A7F3D0;
  }

  .badge-pending {
    background: #FFFBEB;
    color: var(--color-status-pending);
    border: 1px solid #FDE68A;
  }

  .badge-declined {
    background: #FEF2F2;
    color: var(--color-status-declined);
    border: 1px solid #FECACA;
  }

  .badge-none {
    background: #F9FAFB;
    color: var(--color-status-none);
    border: 1px solid #E5E7EB;
  }
</style>
