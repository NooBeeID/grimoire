<script lang="ts">
  import Spinner from '../../shared/components/Spinner.svelte';
  import { t } from '../../shared/stores/i18n';
</script>

<div aria-busy="true" class="flex min-h-[50dvh] flex-col items-center justify-center gap-2">
  <Spinner size={32} />
  <p class="text-muted">{$t('app.loading')}</p>
</div>
