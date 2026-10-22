<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import {
    ConfigSummary,
    RepoOriginChip,
    ReportSummary,
    StatusBadge,
    SyncReportTable,
    errorText,
    parseRepoUrlLite,
    type GitSyncConfig,
    type Kind,
  } from '../../base';
  import { createGitSyncConfigStore } from '../../getconfig';
  import CredentialNotice from '../components/CredentialNotice.svelte';
  import SyncErrorBlock from '../components/SyncErrorBlock.svelte';
  import SyncForm from '../components/SyncForm.svelte';
  import { syncErrorMessage } from '../errorMessage';
  import { createRunSyncStore } from '../stores/runsync.store';
  import type { FormTouched, FormValues } from '../types';

  let {
    open,
    projectId,
    kind,
    onclose,
    onsynced,
    onprojectgone,
  }: {
    open: boolean;
    projectId: string;
    kind: Kind;
    onclose: () => void;
    onsynced: () => void;
    onprojectgone?: () => void;
  } = $props();

  const cfg = createGitSyncConfigStore();
  const run = createRunSyncStore();

  let values = $state<FormValues>({ url: '', branch: '', folder: '', targetPrefix: '' });
  let touched = $state<FormTouched>({ branch: false, folder: false });
  let filled = false;
  let cooldown = $state(0);
  let timer: ReturnType<typeof setInterval> | null = null;
  let announced = false;

  const data = $derived($cfg.status === 'ready' ? $cfg.data : $cfg.status === 'loading' || $cfg.status === 'error' ? $cfg.previous : null);
  const config = $derived(data?.config ?? null);
  const loadingConfig = $derived($cfg.status === 'loading' && data === null);
  const running = $derived($run.status === 'running');
  const featureOff = $derived(data?.feature_enabled === false);
  const parsed = $derived(parseRepoUrlLite(values.url));
  const provider = $derived(parsed.ok ? parsed.provider : (config?.provider ?? 'github'));
  const providerName = $derived($t(`gitCredential.provider.${provider}`));
  const hostMatches = $derived(parsed.ok && config !== null && parsed.host === config.host);
  const credBad = $derived(config !== null && data !== null && (!data.has_credential || data.credential_unreadable));
  // F5/F6: tanpa config, token baru diperiksa server; dengan config, kunci hanya bila host form = host config.
  const syncLocked = $derived(featureOff || (credBad && hostMatches) || cooldown > 0);
  const rerunLocked = $derived(featureOff || !config || credBad || cooldown > 0);
  const urlProblem = $derived(!parsed.ok && values.url.trim() !== '' ? $t(`gitSync.validation.${parsed.problem}`) : undefined);
  const canSync = $derived(parsed.ok && !running && !syncLocked && !loadingConfig);
  const noticeState = $derived(featureOff ? 'disabled' : data?.credential_unreadable ? 'unreadable' : 'missing');
  const showNotice = $derived(featureOff || (credBad && hostMatches));

  const runError = $derived($run.status === 'error' ? $run.error : null);
  const fieldError = $derived(runError?.field ? syncErrorMessage($t, runError, cooldown) : undefined);
  const errors = $derived({
    url: urlProblem ?? (runError?.field === 'url' ? fieldError : undefined),
    branch: runError?.field === 'branch' ? fieldError : undefined,
    folder: runError?.field === 'folder' ? fieldError : undefined,
    target_prefix: runError?.field === 'target_prefix' ? fieldError : undefined,
  });

  function fillFrom(c: GitSyncConfig | null) {
    values = { url: c?.repo_url ?? '', branch: c?.branch ?? '', folder: c?.folder ?? '', targetPrefix: c?.target_prefix ?? '' };
    touched = { branch: false, folder: false };
  }

  function stopTimer() {
    if (timer) clearInterval(timer);
    timer = null;
  }
  function startCooldown(seconds: number) {
    stopTimer();
    cooldown = seconds;
    if (seconds <= 0) return;
    timer = setInterval(() => {
      cooldown = Math.max(0, cooldown - 1);
      if (cooldown === 0) stopTimer();
    }, 1000);
  }
  onDestroy(stopTimer);

  $effect(() => {
    if (open) {
      untrack(() => {
        filled = false;
        announced = false;
        run.reset();
        stopTimer();
        cooldown = 0;
        fillFrom(null);
        void cfg.load(projectId, kind);
      });
    } else {
      untrack(() => {
        cfg.reset();
        run.reset();
        stopTimer();
      });
    }
  });

  $effect(() => {
    if (open && $cfg.status === 'ready' && !filled) {
      filled = true;
      const c = $cfg.data.config;
      untrack(() => fillFrom(c));
    }
  });

  $effect(() => {
    const st = $run.status === 'error' ? $run.error : null;
    if (!st) return;
    untrack(() => {
      if (st.code === 'NOT_FOUND') {
        onclose();
        onprojectgone?.();
      }
      if (st.code === 'RATE_LIMITED' && st.retryAfter) startCooldown(st.retryAfter);
    });
  });

  async function start(useSaved: boolean) {
    if (running) return;
    const c = config;
    const input =
      useSaved && c
        ? { kind, url: c.repo_url, branch: c.branch, folder: c.folder, target_prefix: c.target_prefix }
        : { kind, url: values.url, branch: values.branch, folder: values.folder, target_prefix: values.targetPrefix };
    announced = false;
    const r = await run.start(projectId, input);
    if (r.status === 'done') {
      cfg.setFromRun(r.config, r.report);
      if (r.report.status !== 'failed' && !announced) {
        announced = true;
        onsynced();
      }
    }
  }

  function again() {
    run.reset();
    const c = $cfg.status === 'ready' ? $cfg.data.config : null;
    fillFrom(c);
  }

  function cancel() {
    if (running) {
      run.abort();
      toast.push({ type: 'info', message: $t('gitSync.cancelledToast') });
    }
    onclose();
  }

  const total = $derived($run.status === 'done' ? Object.values($run.report.summary).reduce((a, b) => a + b, 0) : 0);
  const announce = $derived(
    $run.status === 'done' ? $t('gitSync.report.announce', { created: $run.report.summary.created, skipped: $run.report.summary.skipped, rejected: $run.report.summary.rejected }) : '',
  );
</script>

<Dialog {open} title={$t(`gitSync.dialog.title.${kind}`)} size="lg" dismissible={!running} onclose={cancel}>
  <div class="flex flex-col gap-4">
    {#if $run.status === 'done'}
      <div class="flex flex-col gap-3" aria-live="polite">
        <p class="sr-only">{announce}</p>
        <div class="flex flex-wrap items-center gap-2">
          <RepoOriginChip host={$run.config.host} repoPath={$run.config.repo_path} branch={$run.config.branch} folder={$run.config.folder} />
          <StatusBadge status={$run.report.status} />
        </div>
        {#if $run.report.status === 'failed'}
          <p class="text-sm text-danger">{errorText($t, $run.report.error_code, $run.report.error_message)}</p>
        {/if}
        <ReportSummary summary={$run.report.summary} />
        <SyncReportTable items={$run.report.items} {kind} truncated={$run.report.truncated} {total} />
      </div>
    {:else}
      {#if loadingConfig}
        <div class="h-5 w-2/3 rounded-base bg-raised" aria-hidden="true"></div>
        <p class="sr-only">{$t('common.loading')}</p>
      {:else if $cfg.status === 'error' && data === null}
        <div class="rounded-card border border-border">
          <ErrorState error={$cfg.error} onretry={() => cfg.load(projectId, kind)} />
        </div>
      {:else}
        <ConfigSummary {config} {kind} />
      {/if}

      {#if showNotice}
        <CredentialNotice state={noticeState} host={config?.host ?? ''} />
      {/if}

      <SyncForm {kind} bind:values bind:touched {errors} disabled={running || loadingConfig || featureOff} />

      <p class="flex items-start gap-2 text-[13px] text-warning">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" aria-hidden="true" class="mt-0.5 shrink-0">
          <circle cx="8" cy="8" r="6" />
          <path d="M8 7.5v3.5M8 5.25v.01" />
        </svg>
        <span>{$t('gitSync.overwriteWarning')}</span>
      </p>

      {#if running}
        <p class="text-[13px] text-muted" aria-live="polite">{$t('gitSync.runningHint')}</p>
      {/if}
      {#if runError && !runError.field && runError.code !== 'ABORTED'}
        <div aria-live="polite">
          <SyncErrorBlock error={runError} seconds={cooldown} onretry={() => start(false)} />
        </div>
      {/if}
    {/if}
  </div>

  {#snippet footer()}
    {#if $run.status === 'done'}
      <Button variant="secondary" onclick={again}>{$t('gitSync.again')}</Button>
      <Button variant="primary" onclick={onclose}>{$t('gitSync.close')}</Button>
    {:else if featureOff}
      <Button variant="primary" onclick={onclose}>{$t('gitSync.close')}</Button>
    {:else if loadingConfig}
      <Button variant="secondary" onclick={onclose}>{$t('gitSync.cancel')}</Button>
    {:else}
      <Button variant="ghost" onclick={cancel}>{$t('gitSync.cancel')}</Button>
      <Button variant="secondary" disabled={rerunLocked || running} onclick={() => start(true)}>{$t('gitSync.rerun')}</Button>
      <Button variant="primary" loading={running} disabled={!canSync && !running} onclick={() => start(false)}>
        {running ? $t('gitSync.running', { provider: providerName }) : $t('gitSync.run')}
      </Button>
    {/if}
  {/snippet}
</Dialog>
