<script lang="ts">
  import { DeleteFileDialog } from '../../../../domain/project/knowledge/deletefile';
  import { DeleteFolderDialog } from '../../../../domain/project/knowledge/deletefolder';
  import { MoveFileDialog } from '../../../../domain/project/knowledge/movefile';
  import { UploadDialogView } from '../../../../domain/project/knowledge/uploadfiles';
  import { GitSyncDialog } from '../../../../domain/project/git-sync/runsync';
  import { extraUploadTargets } from '../../../workspace/uploadTargets';

  export type DialogState =
    | { kind: 'upload'; dir: string }
    | { kind: 'move'; from: string }
    | { kind: 'deleteFile'; path: string }
    | { kind: 'deleteFolder'; path: string; count: number }
    | { kind: 'gitSync' }
    | null;

  let {
    projectId,
    dialog,
    onclose,
    onuploaded,
    onsynced,
    onmoved,
    ondeletedfile,
    ondeletedfolder,
    onstale,
    onprojectgone,
  }: {
    projectId: string;
    dialog: DialogState;
    onclose: () => void;
    onuploaded: () => void;
    onsynced: () => void;
    onmoved: (path: string) => void;
    ondeletedfile: (path: string) => void;
    ondeletedfolder: (path: string) => void;
    onstale: () => void;
    onprojectgone: () => void;
  } = $props();
</script>

<UploadDialogView
  open={dialog?.kind === 'upload'}
  {projectId}
  targetDir={dialog?.kind === 'upload' ? dialog.dir : ''}
  extraTargets={extraUploadTargets}
  {onclose}
  {onuploaded}
  {onprojectgone}
/>
<MoveFileDialog
  open={dialog?.kind === 'move'}
  {projectId}
  from={dialog?.kind === 'move' ? dialog.from : ''}
  {onclose}
  onmoved={(f) => onmoved(f.path)}
  onnotfound={() => {
    onclose();
    onstale();
  }}
/>
<DeleteFileDialog
  open={dialog?.kind === 'deleteFile'}
  {projectId}
  path={dialog?.kind === 'deleteFile' ? dialog.path : ''}
  {onclose}
  ondeleted={ondeletedfile}
/>
<DeleteFolderDialog
  open={dialog?.kind === 'deleteFolder'}
  {projectId}
  path={dialog?.kind === 'deleteFolder' ? dialog.path : ''}
  fileCount={dialog?.kind === 'deleteFolder' ? dialog.count : 0}
  {onclose}
  ondeleted={(p) => ondeletedfolder(p)}
/>
<GitSyncDialog open={dialog?.kind === 'gitSync'} {projectId} kind="knowledge" {onclose} {onsynced} {onprojectgone} />
