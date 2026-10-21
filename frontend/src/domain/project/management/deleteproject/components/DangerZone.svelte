<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import { t } from '../../../../../shared/stores/i18n';

  let { onrequest }: { onrequest: () => void } = $props();
</script>

<section class="flex flex-col gap-2 rounded-base border border-danger p-4">
  <h2 class="text-lg font-semibold text-danger">{$t('project.delete.zoneTitle')}</h2>
  <p class="text-sm">{$t('project.delete.zoneDescription')}</p>
  <div><Button variant="danger" onclick={onrequest}>{$t('project.delete.open')}</Button></div>
</section>
