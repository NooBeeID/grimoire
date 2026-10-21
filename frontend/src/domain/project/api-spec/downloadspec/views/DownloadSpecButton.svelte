<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { saveBlob } from '../../../../../shared/utils/download';
  import { downloadSpec } from '../services/downloadspec.service';

  let {
    projectId,
    spec,
    onspecgone,
  }: { projectId: string; spec: { name: string; format: 'yaml' | 'json' }; onspecgone: () => void } = $props();

  let busy = $state(false);
  async function run() {
    busy = true;
    try {
      const r = await downloadSpec(projectId, spec.name, spec.format);
      saveBlob(r.blob, r.fileName);
    } catch (e) {
      if (isApiError(e) && e.status === 404) {
        toast.push({ type: 'error', message: $t('apispec.download.gone') });
        onspecgone();
      } else toast.push({ type: 'error', message: isApiError(e) ? e.message : String(e) });
    } finally {
      busy = false;
    }
  }
</script>

<Button variant="secondary" loading={busy} onclick={run}>{$t('apispec.download.open')}</Button>
