<script lang="ts">
  import { useLocalization } from '../../localization';
  import { ActionMenu } from '../../ui';
  import type { Folder } from '../../folders';
  const l = useLocalization();
  let {
    folder,
    hasReviews,
    saving,
    appearance = 'default',
    startFolderReview,
    exportSingleFolder,
    editFolder,
    confirmDelete,
  }: {
    folder: Folder;
    hasReviews: boolean;
    saving: boolean;
    appearance?: 'default' | 'quiet';
    startFolderReview: (opener: HTMLElement, folderId: string) => void;
    exportSingleFolder: (opener: HTMLElement, folderId: string) => void;
    editFolder: (opener: HTMLElement, folder: Folder) => void;
    confirmDelete: (opener: HTMLElement, folderId: string) => void;
  } = $props();
</script>

<ActionMenu
  label={$l.t('workspace.folderOptions', { name: folder.title })}
  {appearance}
  disabled={saving}
  actions={[
    ...(hasReviews
      ? [
          {
            label: $l.t('workspace.full_review'),
            onselect: (opener: HTMLButtonElement) => startFolderReview(opener, folder.id),
          },
        ]
      : []),
    { label: $l.t('workspace.export_folder'), onselect: (opener) => exportSingleFolder(opener, folder.id) },
    { label: $l.t('workspace.edit_folder'), onselect: (opener) => editFolder(opener, folder) },
    { label: $l.t('workspace.delete_folder'), onselect: (opener) => confirmDelete(opener, folder.id) },
  ]}
/>
