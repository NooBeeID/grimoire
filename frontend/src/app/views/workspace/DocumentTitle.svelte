<script lang="ts">
  import { t } from '../../../shared/stores/i18n';

  let { name }: { name: string } = $props();

  $effect(() => {
    document.title = `${name} · ${$t('app.name')}`;
  });
</script>
