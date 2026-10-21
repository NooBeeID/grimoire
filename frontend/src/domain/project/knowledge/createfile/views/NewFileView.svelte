<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { reasonOf } from '../../../../../shared/validation/error';
  import { errorMessage, reasonMessageKey, type KnowledgeFile } from '../../base';
  import NewFileForm from '../components/NewFileForm.svelte';
  import { createFile } from '../services/createfile.service';
  import { createNewFileStore } from '../stores/newFile.store';

  let {
    projectId,
    dir,
    initialContent = '',
    oncreated,
    oncancel,
    ondirtychange,
    onopenexisting,
    onprojectgone,
  }: {
    projectId: string;
    dir: string;
    initialContent?: string;
    oncreated: (file: KnowledgeFile) => void;
    oncancel: () => void;
    ondirtychange?: (dirty: boolean) => void;
    onopenexisting?: (path: string) => void;
    onprojectgone?: () => void;
  } = $props();

  const store = createNewFileStore(
    untrack(() => dir),
    untrack(() => initialContent),
  );
  const valid = store.valid;
  const clientReason = store.pathError;
  const unsub = store.dirty.subscribe((d) => ondirtychange?.(d));
  onDestroy(unsub);

  const pathError = $derived(
    $store.serverPathError ?? ($clientReason ? $t(reasonMessageKey($clientReason) ?? 'error.internal') : undefined),
  );

  async function submit() {
    store.touch();
    if (!$valid || $store.submitting) return;
    store.submitting(true);
    store.clearErrors();
    try {
      const f = await createFile(projectId, { path: $store.path, content: $store.content });
      toast.push({ type: 'success', message: $t('knowledge.new.created') });
      ondirtychange?.(false);
      oncreated(f);
    } catch (err) {
      if (isApiError(err)) {
        const reason = reasonOf(err.details);
        if (err.status === 404) {
          store.formFailed($t('knowledge.move.sourceGone'));
          ondirtychange?.(false);
          onprojectgone?.();
        } else if (err.status === 409 && reason === 'path_exists') store.pathFailed(errorMessage(err, $t), true);
        else if (err.status === 400 && reason && /^(path_|segment_|hidden|unsupported)/.test(reason)) store.pathFailed(errorMessage(err, $t));
        else store.formFailed(errorMessage(err, $t));
      } else store.formFailed(errorMessage(err, $t));
    } finally {
      store.submitting(false);
    }
  }
</script>

<NewFileForm
  path={$store.path}
  content={$store.content}
  {pathError}
  formError={$store.formError}
  exists={$store.exists}
  submitting={$store.submitting}
  valid={$valid}
  onpathchange={store.setPath}
  oncontentchange={store.setContent}
  onsubmit={submit}
  {oncancel}
  onopenexisting={onopenexisting ? () => onopenexisting($store.path) : undefined}
  ontouch={store.touch}
/>
