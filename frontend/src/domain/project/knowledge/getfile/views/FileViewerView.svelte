<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { copyText } from '../../../../../shared/utils/clipboard';
  import { baseName, dirName, type KnowledgeFile } from '../../base';
  import Breadcrumb from '../components/Breadcrumb.svelte';
  import FileMeta from '../components/FileMeta.svelte';
  import FileContent from '../components/FileContent.svelte';
  import { getFile } from '../services/getfile.service';
  import { createViewerStore } from '../stores/viewer.store';

  let {
    projectId,
    path,
    reloadKey = 0,
    resolveLink,
    onedit,
    onmove,
    ondelete,
    ondir,
    onnotfound,
  }: {
    projectId: string;
    path: string;
    reloadKey?: number;
    resolveLink?: (href: string, fromDir: string) => string | null;
    onedit: (file: KnowledgeFile) => void;
    onmove: (file: KnowledgeFile) => void;
    ondelete: (file: KnowledgeFile) => void;
    ondir: (path: string) => void;
    onnotfound: (path: string) => void;
  } = $props();

  const store = createViewerStore();
  let ctrl: AbortController | null = null;
  let retry = $state(0);
  let notified = '';

  async function load(p: string) {
    ctrl?.abort();
    const c = new AbortController();
    ctrl = c;
    store.begin(p);
    try {
      const f = await getFile(projectId, p, c.signal);
      if (!c.signal.aborted) store.resolve(f);
    } catch (e) {
      if (c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return;
      store.fail(p, isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) }));
    }
  }

  $effect(() => {
    const p = path;
    void reloadKey;
    void retry;
    void projectId;
    untrack(() => void load(p));
    return () => ctrl?.abort();
  });

  $effect(() => {
    if ($store.status === 'notFound' && notified !== $store.path) {
      notified = $store.path;
      onnotfound($store.path);
    }
  });

  async function copy() {
    const ok = await copyText(path);
    toast.push({ type: ok ? 'success' : 'error', message: $t(ok ? 'knowledge.copy.done' : 'knowledge.copy.failed') });
  }
</script>

<section aria-busy={$store.status === 'loading'} class="flex flex-col gap-4">
  {#if $store.status === 'idle' || ($store.status === 'loading')}
    <p class="text-sm text-muted">{$t('common.loading')}</p>
  {:else if $store.status === 'notFound'}
    <EmptyState title={$t('knowledge.viewer.notFound')} description={$t('knowledge.viewer.notFoundHint')}>
      {#snippet action()}<Button variant="secondary" onclick={() => ondir(dirName(path))}>{$t('knowledge.viewer.backToFolder')}</Button>{/snippet}
    </EmptyState>
  {:else if $store.status === 'error'}
    <ErrorState error={$store.error} onretry={() => retry++} />
  {:else}
    {@const file = $store.file}
    <header class="flex flex-col gap-3 border-b border-rule pb-4">
      <Breadcrumb {path} {ondir} oncopy={copy} />
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <h2 class="text-lg font-semibold break-words">{file.title || baseName(file.path)}</h2>
          <FileMeta {file} />
        </div>
        <div class="flex gap-2">
          <Button onclick={() => onedit(file)}>{$t('knowledge.actions.edit')}</Button>
          <Button variant="secondary" onclick={() => onmove(file)}>{$t('knowledge.actions.move')}</Button>
          <Button variant="secondary" onclick={() => ondelete(file)}>{$t('knowledge.actions.delete')}</Button>
        </div>
      </div>
    </header>
    <FileContent {file} resolveLink={resolveLink ? (h) => resolveLink(h, dirName(file.path)) : undefined} />
  {/if}
</section>
