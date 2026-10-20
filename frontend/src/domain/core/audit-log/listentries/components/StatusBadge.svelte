<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { AuditStatus } from '../types';
  let { status }: { status: AuditStatus } = $props();
  const cls = { ok: 'border-border text-fg', error: 'border-danger text-danger', denied: 'border-danger text-danger' };
</script>

<span class="inline-block rounded-base border px-2 py-0.5 text-xs font-medium {cls[status]}">
  {$t(`auditLog.status.${status}`)}
</span>
