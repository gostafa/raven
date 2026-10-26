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
    canPaste,
    pasteCards,
  }: {
    canPaste: boolean;
    pasteCards: (opener: HTMLElement, folderId?: string) => void;
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
    ...(canPaste
      ? [
          {
            label: $l.t('transfer.paste_here'),
            onselect: (opener: HTMLButtonElement) => {
              pasteCards(opener, folder.id);
            },
          },
        ]
      : []),
    ...(hasReviews
      ? [
          {
            label: $l.t('workspace.full_review'),
            onselect: (opener: HTMLButtonElement) => {
              startFolderReview(opener, folder.id);
            },
          },
        ]
      : []),
    {
      label: $l.t('workspace.export_folder'),
      onselect: (opener: HTMLButtonElement) => {
        exportSingleFolder(opener, folder.id);
      },
    },
    {
      label: $l.t('workspace.edit_folder'),
      onselect: (opener: HTMLButtonElement) => {
        editFolder(opener, folder);
      },
    },
    {
      label: $l.t('workspace.delete_folder'),
      onselect: (opener: HTMLButtonElement) => {
        confirmDelete(opener, folder.id);
      },
    },
  ]}
/>
