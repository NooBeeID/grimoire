<script lang="ts">
  import { t } from '../stores/i18n';

  let { emailEnabled }: { emailEnabled: boolean | null } = $props();
  const key = $derived(emailEnabled === true ? 'enabled' : emailEnabled === false ? 'disabled' : 'unknown');
</script>

<p class="text-[13px] text-muted" data-testid="email-delivery-note">{$t(`invitation.note.${key}`)}</p>
