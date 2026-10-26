<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { AuditStatus } from '../types';
  let { status }: { status: AuditStatus } = $props();
  const cls = {
    ok: 'bg-pill-neutral-bg text-pill-neutral-ink',
    error: 'bg-pill-negative-bg text-pill-negative-ink',
    denied: 'bg-pill-warning-bg text-pill-warning-ink',
  };
</script>

<span class="inline-flex h-6 items-center rounded-full px-2.5 text-xs font-medium {cls[status]}">
  {$t(`auditLog.status.${status}`)}
</span>
