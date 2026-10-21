<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { copyText } from '../../../../../shared/utils/clipboard';
  import { issueText } from '../model';
  import type { ApiSpecFileResult } from '../types';
  import StatusBadge from './StatusBadge.svelte';

  type Row = ApiSpecFileResult & { willOverwrite?: boolean };
  let { results, projectId, linkNames = false }: { results: Row[]; projectId?: string; linkNames?: boolean } = $props();

  let copied = $state<number | null>(null);
  let manual = $state<number | null>(null);

  async function copy(i: number, r: ApiSpecFileResult) {
    if (await copyText(issueText(r))) {
      copied = i;
      manual = null;
      setTimeout(() => (copied = null), 2000);
    } else manual = i;
  }
  const reasonLabel = (r: ApiSpecFileResult) => {
    if (!r.reason_code) return r.reason;
    const key = `apispec.reason.${r.reason_code}`;
    const label = $t(key);
    return label === key ? (r.reason ?? r.reason_code) : label;
  };
</script>

<ul class="flex flex-col divide-y divide-rule rounded-card border border-border bg-surface">
  {#each results as r, i (i)}
    <li class="flex flex-col gap-1.5 px-4 py-3" data-status={r.status}>
      <div class="flex flex-wrap items-center gap-2">
        <StatusBadge status={r.status} />
        <code class="break-all font-mono text-[13px]">{r.source_path ?? r.file}</code>
        {#if r.name && linkNames && projectId && (r.status === 'created' || r.status === 'updated' || r.status === 'unchanged')}
          <a
            class="text-sm text-accent-ink underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
            href="/projects/{encodeURIComponent(projectId)}/api-specs/{encodeURIComponent(r.name)}">{r.name}</a
          >
        {:else if r.name}
          <span class="font-mono text-[13px] text-muted">{r.name}</span>
        {/if}
        {#if r.willOverwrite}<span class="text-xs text-warning">{$t('apispec.upload.willOverwrite')}</span>{/if}
      </div>
      {#if r.title}
        <p class="text-sm text-muted">
          {r.title}{#if r.openapi_version} · OpenAPI {r.openapi_version}{/if}{#if r.api_version} · {r.api_version}{/if}{#if r.endpoint_count !== null}
            · {r.endpoint_count} endpoint{/if}
        </p>
      {/if}
      {#if r.status === 'rejected' || r.status === 'skipped'}
        <p class="text-sm">{reasonLabel(r)}</p>
        {#if r.issues.length > 0}
          <pre class="whitespace-pre-wrap break-words rounded-base bg-raised p-2 font-mono text-xs" data-testid="issues-{i}">{issueText(r)}</pre>
          <div class="flex items-center gap-2">
            <Button size="sm" variant="secondary" onclick={() => copy(i, r)}>{copied === i ? $t('common.copied') : $t('common.copy')}</Button>
            {#if manual === i}<span class="text-xs text-muted">{$t('common.copyManual')}</span>{/if}
          </div>
        {/if}
      {/if}
    </li>
  {/each}
</ul>
