<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import type { UploadProgress } from '../../../../../shared/services/upload';
  import { t } from '../../../../../shared/stores/i18n';

  let {
    progress,
    targetLabel = '',
    oncancel,
  }: { progress: UploadProgress | null; targetLabel?: string; oncancel: () => void } = $props();

  const percent = $derived(progress && progress.total > 0 ? Math.min(100, Math.round((progress.loaded / progress.total) * 100)) : 0);
</script>

<div class="flex flex-col gap-2" aria-live="polite">
  {#if targetLabel}<p class="text-[13px] text-muted">{targetLabel}</p>{/if}
  {#if progress?.phase === 'sending'}
    <progress class="h-1.5 w-full" max="100" value={percent} aria-label={$t('knowledge.upload.sending', { percent })}></progress>
    <p class="text-sm">{$t('knowledge.upload.sending', { percent })}</p>
  {:else}
    <progress class="h-1.5 w-full" aria-label={$t('knowledge.upload.processing')}></progress>
    <p class="text-sm">{$t('knowledge.upload.processing')}</p>
  {/if}
</div>
<div class="mt-4 flex justify-end">
  <Button variant="secondary" onclick={oncancel}>{$t('knowledge.upload.cancel')}</Button>
</div>
