<script lang="ts">
  import { onMount } from 'svelte';
  import { refreshEmailStatus } from '../../domain/core/auth/getme';
  import { UsersListView } from '../../domain/core/auth/users';
  import { mailStatus } from '../../shared/stores/mailStatus';

  onMount(() => {
    if ($mailStatus === null) void refreshEmailStatus();
  });
</script>

<UsersListView emailEnabled={$mailStatus} />
