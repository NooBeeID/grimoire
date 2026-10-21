<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import type { ConflictCurrent } from '../../base';
  import CompareView from './CompareView.svelte';

  let {
    conflict,
    mine,
    compareOpen,
    saving,
    oncompare,
    onreload,
    onkeep,
    onoverwrite,
  }: {
    conflict: ConflictCurrent | null;
    mine: string;
    compareOpen: boolean;
    saving: boolean;
    oncompare: () => void;
    onreload: () => void;
    onkeep: () => void;
    onoverwrite: () => void;
  } = $props();
</script>

<Dialog open={conflict !== null} title={$t('knowledge.conflict.title')} size="lg" dismissible={false} onclose={onkeep}>
  {#if conflict}
    <p class="text-sm">
      {$t('knowledge.conflict.body', { name: conflict.updated_by_label, time: formatDateTime(conflict.updated_at, $locale) })}
    </p>
    {#if compareOpen}<CompareView {mine} theirs={conflict.content} />{/if}
  {/if}
  {#snippet footer()}
    {#if compareOpen}
      <Button variant="danger" loading={saving} onclick={onoverwrite}>{$t('knowledge.conflict.overwrite')}</Button>
      <Button variant="secondary" onclick={onreload}>{$t('knowledge.conflict.useTheirs')}</Button>
    {:else}
      <Button variant="secondary" onclick={oncompare}>{$t('knowledge.conflict.compare')}</Button>
      <Button variant="secondary" onclick={onreload}>{$t('knowledge.conflict.reload')}</Button>
    {/if}
    <Button onclick={onkeep}>{$t('knowledge.conflict.keepEditing')}</Button>
  {/snippet}
</Dialog>
