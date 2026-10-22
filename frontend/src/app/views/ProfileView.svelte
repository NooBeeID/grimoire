<script lang="ts">
  import { tick } from 'svelte';
  import { ChangePasswordView } from '../../domain/core/auth/changepassword';
  import { DeleteCredentialDialog, type DeleteTarget } from '../../domain/core/git-credential/deletecredential';
  import { GitTokensSection, gitCredentials, type SaveRequest } from '../../domain/core/git-credential/listcredentials';
  import { SaveTokenDialog, type SaveTarget } from '../../domain/core/git-credential/savecredential';
  import { t } from '../../shared/stores/i18n';
  import { session } from '../../shared/stores/session';
  import { current } from '../routes/router';

  let saveTarget = $state<SaveTarget | null>(null);
  let deleteTarget = $state<DeleteTarget | null>(null);
  let ready = $state(false);

  // Dialog menyimpan target terakhir agar konten tidak hilang saat animasi tutup.
  let lastSave = $state<SaveTarget>({ provider: null, host: null, gitlabHosts: [] });
  $effect(() => {
    if (saveTarget) lastSave = saveTarget;
  });

  function openSave(req: SaveRequest) {
    saveTarget = { provider: req.provider, host: req.host, gitlabHosts: req.gitlabHosts };
  }

  // #token-git: gulir dan fokus ke judul seksi setelah data siap, juga saat navigasi in-app mengubah hash.
  async function focusAnchor() {
    void $current;
    if (!ready || window.location.hash !== '#token-git') return;
    await tick();
    const h = document.getElementById('token-git-title');
    h?.scrollIntoView({ block: 'start' });
    h?.focus({ preventScroll: true });
  }
  $effect(() => {
    void focusAnchor();
  });
</script>

<div class="mx-auto flex max-w-2xl flex-col gap-6">
  <div class="flex max-w-sm flex-col gap-6">
    <h1 class="text-[22px] leading-8 font-semibold">{$t('auth.profile.title')}</h1>
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

  <hr class="border-rule" />

  <section id="token-git" aria-labelledby="token-git-title" class="scroll-mt-6">
    <GitTokensSection
      onready={() => (ready = true)}
      onsave={(row, hosts) => openSave({ provider: row.provider, host: row.host, gitlabHosts: row.provider === 'gitlab' ? (hosts.length ? hosts : [row.host]) : [] })}
      onadd={openSave}
      ondelete={(row) => row.saved && (deleteTarget = { provider: row.provider, host: row.host, last4: row.saved.last4 })}
    />
  </section>
</div>

<SaveTokenDialog open={saveTarget !== null} target={lastSave} onclose={() => (saveTarget = null)} onsaved={() => gitCredentials.invalidate()} />
<DeleteCredentialDialog open={deleteTarget !== null} target={deleteTarget} onclose={() => (deleteTarget = null)} ondeleted={() => gitCredentials.invalidate()} />
