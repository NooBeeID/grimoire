<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatBytes } from '../../../../../shared/utils/bytes';
  import { MAX_DROP_ENTRIES } from '../services/collect';
  import type { UploadPlan, UploadTarget } from '../types';

  let {
    plan,
    targets,
    targetDir,
    truncated = false,
    onstart,
    oncancel,
  }: {
    plan: UploadPlan;
    targets: UploadTarget[];
    targetDir: string;
    truncated?: boolean;
    onstart: () => void;
    oncancel: () => void;
  } = $props();

  const sendable = $derived([...plan.byTarget.values()].reduce((n, l) => n + l.length, 0));
  const COLLAPSE_AT = 10;
</script>

<h3 class="text-sm font-semibold">{$t('knowledge.upload.review')}</h3>
<p class="mt-1 text-sm text-muted">
  {$t('knowledge.upload.targetDir', { dir: targetDir || $t('knowledge.upload.root') })}
</p>
<ul class="mt-2 flex flex-col gap-1 text-sm">
  {#each targets as tg (tg.key)}
    {@const list = plan.byTarget.get(tg.key) ?? []}
    {#if list.length}
      <li>
        {$t('knowledge.upload.toTarget', {
          count: list.length,
          target: $t(tg.labelKey),
          size: formatBytes(list.reduce((n, f) => n + f.file.size, 0), $locale),
        })}
      </li>
    {/if}
  {/each}
  {#if sendable === 0}<li class="text-muted">{$t('knowledge.upload.nothing')}</li>{/if}
</ul>

{#each [{ key: 'skippedList', items: plan.skipped }, { key: 'unsupportedList', items: plan.unsupported }] as g (g.key)}
  {#if g.items.length}
    <!-- Dilipat bila banyak; daftar path ditampilkan sebagai teks mono. -->
    <details class="mt-2 text-sm text-muted" open={g.items.length <= COLLAPSE_AT}>
      <summary class="cursor-pointer">{$t(`knowledge.upload.${g.key}`, { count: g.items.length })}</summary>
      <ul class="mt-1 max-h-40 overflow-auto font-mono text-[13px]">
        {#each g.items as it, i (i)}<li class="break-all">{it.path}</li>{/each}
      </ul>
    </details>
  {/if}
{/each}
{#if truncated}<p role="status" class="mt-2 text-sm text-warning">{$t('knowledge.upload.truncated', { count: MAX_DROP_ENTRIES })}</p>{/if}

<div class="mt-4 flex justify-end gap-2">
  <Button variant="secondary" onclick={oncancel}>{$t('knowledge.editor.cancel')}</Button>
  <Button disabled={sendable === 0} onclick={onstart}>{$t('knowledge.upload.start')}</Button>
</div>
