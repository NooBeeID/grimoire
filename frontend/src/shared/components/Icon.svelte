<script lang="ts" module>
  import pencilSquare from 'heroicons/24/outline/pencil-square.svg?raw';
  import trash from 'heroicons/24/outline/trash.svg?raw';
  import noSymbol from 'heroicons/24/outline/no-symbol.svg?raw';
  import envelope from 'heroicons/24/outline/envelope.svg?raw';
  import key from 'heroicons/24/outline/key.svg?raw';
  import powerIcon from 'heroicons/24/outline/power.svg?raw';
  import arrowUp from 'heroicons/24/outline/arrow-up-circle.svg?raw';
  import arrowDown from 'heroicons/24/outline/arrow-down-circle.svg?raw';
  import leave from 'heroicons/24/outline/arrow-right-start-on-rectangle.svg?raw';
  import userMinus from 'heroicons/24/outline/user-minus.svg?raw';
  import paperAirplane from 'heroicons/24/outline/paper-airplane.svg?raw';
  import arrowPath from 'heroicons/24/outline/arrow-path.svg?raw';

  const icons = {
    edit: pencilSquare,
    delete: trash,
    revoke: noSymbol,
    resend: envelope,
    resetPassword: key,
    toggle: powerIcon,
    promote: arrowUp,
    demote: arrowDown,
    leave,
    removeMember: userMinus,
    send: paperAirplane,
    replace: arrowPath,
  };
  export type IconName = keyof typeof icons;
</script>

<script lang="ts">
  let { name, size = 16 }: { name: IconName; size?: number } = $props();
</script>

<span class="inline-flex shrink-0" style="width:{size}px;height:{size}px" aria-hidden="true">{@html icons[name].replace('<svg ', '<svg width="100%" height="100%" ')}</span>
