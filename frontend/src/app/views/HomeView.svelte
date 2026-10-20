<script lang="ts">
  import { t } from '../../shared/stores/i18n';
  import { session } from '../../shared/stores/session';
</script>

<h1 data-testid="home-title" class="text-2xl font-bold">{$t('home.title')}</h1>
{#if $session.identity}<p class="mt-1">{$t('home.greeting', { name: $session.identity.name })}</p>{/if}
<p class="mt-2 text-muted">{$t('home.subtitle')}</p>
