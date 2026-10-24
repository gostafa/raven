<script lang="ts">
  import { useLocalization, type MessageKey } from '../../localization';
  import { parseTags, type Folder } from '../../folders';
  import type { AppPorts } from '../domain/types';
  import WorkspaceIcon from './WorkspaceIcon.svelte';
  const l = useLocalization();
  let {
    backend,
    folderDialog = $bindable(),
    saving = $bindable(false),
    onsaved,
    onclose,
  }: {
    backend: Pick<AppPorts, 'CreateFolder' | 'UpdateFolder'>;
    folderDialog?: HTMLDialogElement;
    saving?: boolean;
    onsaved: (folder: Folder, editing: boolean) => void;
    onclose: () => Promise<void>;
  } = $props();
  let editingFolder = $state<Folder | null>(null);
  let title = $state('');
  let description = $state('');
  let tags = $state('');
  let formError = $state<MessageKey | ''>('');
  let titleInput: HTMLInputElement;

  export function open(folder: Folder | null = null): void {
    editingFolder = folder;
    fillFolder(folder);
    formError = '';
    folderDialog?.showModal();
    titleInput.focus();
  }

  function fillFolder(folder: Folder | null) {
    title = folder ? folder.title : '';
    description = folder ? folder.description : '';
    tags = folder ? folder.tags.join(', ') : '';
  }
  function validateFolder(): boolean {
    if (title.trim()) return true;
    formError = 'workspace.give_your_folder_a_name';
    titleInput.focus();
    return false;
  }
  async function writeFolder(): Promise<Folder> {
    const input = { title, description, tags: parseTags(tags) };
    return editingFolder
      ? await backend.UpdateFolder({ ...input, id: editingFolder.id })
      : await backend.CreateFolder(input);
  }
  function beginSave() {
    saving = true;
    formError = '';
  }
  async function saveFolder(event: SubmitEvent) {
    event.preventDefault();
    if (saving) return;
    if (!validateFolder()) return;
    beginSave();
    try {
      const folder = await writeFolder();
      onsaved(folder, editingFolder !== null);
      folderDialog?.close();
    } catch {
      formError = 'workspace.your_folder_could_not_be_saved_your_entries_are_still_here_try_again';
    } finally {
      saving = false;
    }
  }
</script>

<dialog
  class="workspace-control"
  bind:this={folderDialog}
  aria-labelledby="folder-dialog-title"
  {onclose}
  oncancel={(event) => {
    if (saving) event.preventDefault();
  }}
>
  <form onsubmit={saveFolder} aria-busy={saving} novalidate>
    <div class="workspace-dialog-top">
      <span class="workspace-control workspace-empty-icon"><WorkspaceIcon name="folder" size={24} /></span><button
        type="button"
        class="workspace-control workspace-icon-button"
        aria-label={editingFolder ? $l.t('workspace.closeEditFolder') : $l.t('workspace.closeNewFolder')}
        disabled={saving}
        onclick={() => folderDialog?.close()}><WorkspaceIcon name="close" /></button
      >
    </div>
    <p class="workspace-control workspace-eyebrow">
      {editingFolder ? $l.t('workspace.your_collection') : 'MAKE ROOM FOR A CURIOSITY'}
    </p>
    <h2 class="workspace-control" id="folder-dialog-title">
      {editingFolder ? 'Edit folder info.' : 'A home for related memories.'}
    </h2>
    <p class="workspace-control workspace-dialog-description">{$l.t('workspace.topicHelp')}</p>
    <label class="workspace-control" for="folder-title"
      >{$l.t('workspace.folder_name')} <span class="workspace-control">{$l.t('workspace.required')}</span></label
    ><input
      class="workspace-control"
      dir="auto"
      id="folder-title"
      bind:this={titleInput}
      bind:value={title}
      placeholder={$l.t('workspace.e_g_everyday_spanish')}
      disabled={saving}
      required
      aria-invalid={Boolean(formError) && !title.trim()}
    />
    <label class="workspace-control" for="folder-description"
      >{$l.t('workspace.a_little_context')} <span class="workspace-control">{$l.t('workspace.optional')}</span></label
    ><textarea
      class="workspace-control"
      dir="auto"
      id="folder-description"
      bind:value={description}
      rows="2"
      placeholder={$l.t('workspace.what_would_you_like_to_remember')}
      disabled={saving}></textarea>
    <label class="workspace-control" for="folder-tags"
      >{$l.t('workspace.tags')} <span class="workspace-control">{$l.t('workspace.optional')}</span></label
    ><input
      class="workspace-control"
      dir="auto"
      id="folder-tags"
      bind:value={tags}
      placeholder={$l.t('workspace.language_travel_learning')}
      aria-describedby="tags-hint"
      disabled={saving}
    />
    <p class="workspace-control workspace-field-hint" id="tags-hint">{$l.t('workspace.separate_tags_with_commas')}</p>
    {#if formError}<p class="workspace-control workspace-error" role="alert">{formError ? $l.t(formError) : ''}</p>{/if}
    <div class="workspace-dialog-actions">
      <button
        type="button"
        class="workspace-control workspace-secondary-button"
        disabled={saving}
        onclick={() => folderDialog?.close()}>{$l.t('cards.cancel')}</button
      ><button type="submit" class="workspace-control workspace-primary-button" disabled={saving}
        >{saving
          ? $l.t('workspace.saving')
          : editingFolder
            ? $l.t('workspace.save_changes')
            : $l.t('workspace.create_folder')}
        <WorkspaceIcon name="arrow" size={16} /></button
      >
    </div>
  </form>
</dialog>
