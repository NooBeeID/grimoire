<script lang="ts">
  import { t } from '../../shared/stores/i18n';
</script>

<h1 data-testid="notfound-title" class="text-[22px] leading-8 font-semibold">{$t('notfound.title')}</h1>
<p class="mt-2 text-muted">{$t('notfound.description')}</p>
<a href="/" class="mt-4 inline-block text-primary underline">{$t('notfound.back')}</a>
