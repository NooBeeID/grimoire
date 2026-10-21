<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import { errorMessage, type KnowledgeFile } from '../../base';
  import { getFile } from '../../getfile';
  import ConflictDialog from '../components/ConflictDialog.svelte';
  import EditorSplit from '../components/EditorSplit.svelte';
  import EditorToolbar, { type Pane } from '../components/EditorToolbar.svelte';
  import { parseConflict, updateFile } from '../services/updatefile.service';
  import { clearDraft, loadDraft, saveDraft } from '../stores/draft';
  import { createEditorStore, type EditorStore } from '../stores/editor.store';
  import type { Draft } from '../types';

  let {
    projectId,
    path,
    onclose,
    ondirtychange,
    onnotfound,
    onsaveasnew,
  }: {
    projectId: string;
    path: string;
    onclose: (file: KnowledgeFile | null) => void;
    ondirtychange?: (dirty: boolean) => void;
    onnotfound: (path: string) => void;
    onsaveasnew?: (content: string) => void;
  } = $props();

  type Load = { s: 'loading' } | { s: 'notFound' } | { s: 'error'; e: unknown } | { s: 'ready'; file: KnowledgeFile };
  let load = $state<Load>({ s: 'loading' });
  let store = $state<EditorStore | null>(null);
  let pendingDraft = $state<Draft | null>(null);
  let deletedRemotely = $state(false);
  let confirmDiscard = $state(false);
  let retry = $state(0);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let unsub: (() => void) | undefined;
  let unsubDirty: (() => void) | undefined;

  let draft = $state('');
  let dirty = $state(false);
  let saving = $state(false);
  let conflict = $state<import('../../base').ConflictCurrent | null>(null);
  let compareOpen = $state(false);
  let errorMsg = $state<string | null>(null);
  let version = $state('');
  let pane = $state<Pane>('write');

  function attach(s: EditorStore) {
    unsub?.();
    unsubDirty?.();
    unsub = s.subscribe((x) => {
      draft = x.draft;
      saving = x.saving;
      conflict = x.conflict;
      compareOpen = x.compareOpen;
      errorMsg = x.error;
      version = x.base.version;
    });
    unsubDirty = s.dirty.subscribe((d) => {
      dirty = d;
      ondirtychange?.(d);
    });
  }

  $effect(() => {
    void retry;
    const p = path;
    const id = projectId;
    const c = new AbortController();
    untrack(() => {
      load = { s: 'loading' };
      store = null;
      getFile(id, p, c.signal)
        .then((file) => {
          if (c.signal.aborted) return;
          const s = createEditorStore(file);
          store = s;
          attach(s);
          load = { s: 'ready', file };
          const d = loadDraft(id, p);
          pendingDraft = d && d.content !== file.content ? d : null;
        })
        .catch((e) => {
          if (c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return;
          if (isApiError(e) && e.status === 404) {
            load = { s: 'notFound' };
            onnotfound(p);
          } else load = { s: 'error', e };
        });
    });
    return () => c.abort();
  });

  // Draf disimpan (debounce 1 s) selama dirty.
  $effect(() => {
    const text = draft;
    const d = dirty;
    const v = version;
    if (!d) return;
    clearTimeout(timer);
    timer = setTimeout(() => saveDraft(projectId, path, { content: text, baseVersion: v, savedAt: new Date().toISOString() }), 1000);
    return () => clearTimeout(timer);
  });

  // beforeunload hanya saat dirty.
  $effect(() => {
    if (!dirty) return;
    const h = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener('beforeunload', h);
    return () => window.removeEventListener('beforeunload', h);
  });

  onDestroy(() => {
    unsub?.();
    unsubDirty?.();
    clearTimeout(timer);
  });

  async function save() {
    const s = store;
    if (!s || saving) return;
    s.saving(true);
    s.setError(null);
    try {
      const f = await updateFile(projectId, path, { content: draft, version });
      s.saved(f);
      clearDraft(projectId, path);
      toast.push({ type: 'success', message: $t('knowledge.editor.savedToast') });
    } catch (e) {
      const cur = parseConflict(e);
      if (cur) s.setConflict(cur);
      else if (isApiError(e) && e.status === 404) {
        s.saving(false);
        deletedRemotely = true;
        onnotfound(path);
      } else s.setError(errorMessage(e, $t));
    }
  }

  function restoreDraft() {
    if (pendingDraft) store?.restore(pendingDraft.content, pendingDraft.baseVersion);
    pendingDraft = null;
  }
  function dropDraft() {
    clearDraft(projectId, path);
    pendingDraft = null;
  }
  function cancel() {
    if (dirty) confirmDiscard = true;
    else onclose(null);
  }
  function discard() {
    clearDraft(projectId, path);
    confirmDiscard = false;
    onclose(null);
  }
  function onkeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
      e.preventDefault();
      void save();
    }
  }
  async function overwrite() {
    store?.rebaseOnCurrent();
    await save();
  }
  function useLatest() {
    store?.acceptCurrent();
    clearDraft(projectId, path);
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<section class="flex flex-col gap-4" {onkeydown} aria-busy={load.s === 'loading'}>
  {#if load.s === 'loading'}
    <p class="text-sm text-muted">{$t('common.loading')}</p>
  {:else if load.s === 'notFound'}
    <EmptyState title={$t('knowledge.viewer.notFound')} description={$t('knowledge.viewer.notFoundHint')} />
  {:else if load.s === 'error'}
    <ErrorState error={load.e instanceof ApiError ? load.e : null} onretry={() => retry++} />
  {:else}
    <EditorToolbar {path} {dirty} {saving} {pane} onpane={(p) => (pane = p)} onsave={save} oncancel={cancel} />

    {#if pendingDraft}
      <div role="status" class="flex flex-wrap items-center gap-3 rounded-card border border-border bg-raised p-3 text-sm">
        <span>{$t('knowledge.editor.draftFound', { time: formatDateTime(pendingDraft.savedAt, $locale) })}</span>
        <Button size="sm" onclick={restoreDraft}>{$t('knowledge.editor.restore')}</Button>
        <Button size="sm" variant="secondary" onclick={dropDraft}>{$t('knowledge.editor.discard')}</Button>
      </div>
    {/if}
    {#if deletedRemotely}
      <div role="alert" class="flex flex-wrap items-center gap-3 rounded-card border border-danger p-3 text-sm">
        <span>{$t('knowledge.editor.deletedRemotely')}</span>
        {#if onsaveasnew}<Button size="sm" onclick={() => onsaveasnew(draft)}>{$t('knowledge.editor.saveAsNew')}</Button>{/if}
      </div>
    {/if}
    {#if errorMsg}<p role="alert" class="text-sm text-danger">{errorMsg}</p>{/if}

    <EditorSplit bind:value={() => draft, (v) => store?.edit(v)} {path} {pane} label={$t('knowledge.editor.label')} onsave={save} />
  {/if}
</section>

<ConflictDialog
  {conflict}
  mine={draft}
  {compareOpen}
  {saving}
  oncompare={() => store?.openCompare(true)}
  onreload={useLatest}
  onkeep={() => store?.closeConflict()}
  onoverwrite={overwrite}
/>

<Dialog open={confirmDiscard} title={$t('knowledge.editor.discardConfirm')} onclose={() => (confirmDiscard = false)}>
  {#snippet footer()}
    <Button variant="secondary" onclick={() => (confirmDiscard = false)}>{$t('knowledge.conflict.keepEditing')}</Button>
    <Button variant="danger" onclick={discard}>{$t('knowledge.editor.discard')}</Button>
  {/snippet}
</Dialog>
