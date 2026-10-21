<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import type { UploadProgress as ProgressState } from '../../../../../shared/services/upload';
  import { errorMessage } from '../../base';
  import DropZone from '../components/DropZone.svelte';
  import UploadProgress from '../components/UploadProgress.svelte';
  import UploadReport from '../components/UploadReport.svelte';
  import UploadReview from '../components/UploadReview.svelte';
  import { knowledgeTarget } from '../services/uploadfiles.service';
  import { planUpload } from '../stores/plan';
  import { createUploadStore } from '../stores/upload.store';
  import type { LocalFile, UploadTarget } from '../types';

  let {
    open,
    projectId,
    targetDir,
    extraTargets = [],
    onclose,
    onuploaded,
    onprojectgone,
  }: {
    open: boolean;
    projectId: string;
    targetDir: string;
    extraTargets?: UploadTarget[];
    onclose: () => void;
    onuploaded: () => void;
    onprojectgone?: () => void;
  } = $props();

  const store = createUploadStore();
  const counts = store.counts;
  const filtered = store.filtered;
  const targets = $derived([knowledgeTarget, ...extraTargets]);
  let ctrl: AbortController | null = null;
  let cancelled = $state(false);
  let progress = $state<ProgressState | null>(null);
  let activeTarget = $state('');
  let truncated = $state(false);

  $effect(() => {
    if (open) {
      store.reset();
      cancelled = false;
      progress = null;
      truncated = false;
    }
  });

  // Ekstensi tambahan untuk atribut accept pemilih file (diumumkan target tambahan).
  const extraExts = $derived([...new Set(extraTargets.flatMap((tg) => tg.acceptExts ?? []))]);

  function pick(files: LocalFile[], wasTruncated: boolean) {
    truncated = wasTruncated;
    store.setPlan(planUpload(files, targets, targetDir, $t('knowledge.reason.hidden'), $t('knowledge.upload.unsupportedHere')));
  }

  async function start() {
    const plan = $store.plan;
    if (!plan) return;
    store.start();
    ctrl = new AbortController();
    let anyOk = false;
    for (const target of targets) {
      const files = plan.byTarget.get(target.key);
      if (!files?.length) continue;
      activeTarget = target.labelKey;
      progress = null;
      try {
        const items = await target.upload(projectId, files, { signal: ctrl.signal, onProgress: (p) => (progress = p) });
        store.addReport(items);
        anyOk = true;
      } catch (e) {
        if (ctrl.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) {
          cancelled = true;
          break;
        }
        const err = isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) });
        const msg = errorMessage(err, $t);
        store.targetError(target.key, $t('knowledge.upload.targetFailed', { target: $t(target.labelKey), message: msg }));
        store.addReport(files.map((f) => ({ path: f.path, status: 'rejected' as const, reasonText: msg, target: target.key })));
        if (err.status === 404) onprojectgone?.();
      }
    }
    store.finish(anyOk && !cancelled);
    if (anyOk) onuploaded();
  }

  function cancel() {
    ctrl?.abort();
  }
</script>

<Dialog open={open} title={$t('knowledge.upload.title')} size="lg" dismissible={$store.phase !== 'uploading'} {onclose}>
  {#if $store.phase === 'pick'}
    <p class="mb-3 text-sm text-muted">
      {$t('knowledge.upload.targetDir', { dir: targetDir || $t('knowledge.upload.root') })}
    </p>
    <DropZone acceptExts={extraExts} onpick={pick} />
  {:else if $store.phase === 'review' && $store.plan}
    <UploadReview plan={$store.plan} {targets} {targetDir} {truncated} onstart={start} oncancel={onclose} />
  {:else if $store.phase === 'uploading'}
    <UploadProgress {progress} targetLabel={activeTarget ? $t(activeTarget) : ''} oncancel={cancel} />
  {:else}
    <UploadReport
      items={$filtered}
      counts={$counts}
      filter={$store.filter}
      errors={[...$store.targetErrors.values()]}
      {cancelled}
      onfilter={store.setFilter}
    />
  {/if}

  {#snippet footer()}
    {#if $store.phase === 'done' || $store.phase === 'failed'}
      <Button variant="secondary" onclick={onclose}>{$t('common.close')}</Button>
    {/if}
  {/snippet}
</Dialog>
