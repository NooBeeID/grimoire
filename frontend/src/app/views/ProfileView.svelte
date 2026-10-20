<script lang="ts">
  import { ChangePasswordView } from '../../domain/core/auth/changepassword';
  import { t } from '../../shared/stores/i18n';
  import { session } from '../../shared/stores/session';
</script>

<div class="mx-auto flex max-w-sm flex-col gap-6">
  <h1 class="text-2xl font-bold">{$t('auth.profile.title')}</h1>
  {#if $session.identity}
    <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1" data-testid="profile-card">
      <dt class="text-muted">{$t('users.form.name')}</dt>
      <dd>{$session.identity.name}</dd>
      <dt class="text-muted">{$t('users.form.email')}</dt>
      <dd>{$session.identity.email}</dd>
      <dt class="text-muted">{$t('auth.profile.role')}</dt>
      <dd>{$t(`users.role.${$session.identity.role}`)}</dd>
    </dl>
  {/if}
  <ChangePasswordView mode="profile" />
</div>
