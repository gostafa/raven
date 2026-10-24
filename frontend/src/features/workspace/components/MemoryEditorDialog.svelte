<script lang="ts">
  import { tick } from 'svelte';
  import { useLocalization, type MessageKey } from '../../localization';
  import { CardEditor, editItems, textDraft, saveItems, hasContent, type Card, type EditorItem } from '../../cards';
  import type { Folder } from '../../folders';
  import { saveCard } from '../application/funcs';
  import type { AppPorts } from '../domain/types';
  import WorkspaceIcon from './WorkspaceIcon.svelte';
  const l = useLocalization();
  let {
    backend,
    folders,
    selectedFolderId,
    cardDialog = $bindable(),
    saving = $bindable(false),
    selectingFiles = $bindable(false),
    onsaved,
    onfoldercreated,
    onclose,
  }: {
    backend: AppPorts;
    folders: Folder[];
    selectedFolderId: string | null;
    cardDialog?: HTMLDialogElement;
    saving?: boolean;
    selectingFiles?: boolean;
    onsaved: (card: Card, editing: boolean) => void;
    onfoldercreated: (folder: Folder) => void;
    onclose: () => Promise<void>;
  } = $props();
  let editingCard = $state<Card | null>(null);
  let editorItems = $state<EditorItem[]>([]);
  let cardFolder = $state('');
  let newFolderTitle = $state('');
  let formError = $state<MessageKey | ''>('');

  export function open(card: Card | null = null) {
    editingCard = card;
    editorItems = card ? editItems(card, backend.mediaURL) : [textDraft('', backend.newKey())];
    cardFolder = card?.folderId ?? selectedFolderId ?? folders[0]?.id ?? '__new';
    newFolderTitle = folders.length ? '' : $l.t('workspace.everyday_learning');
    formError = '';
    cardDialog?.showModal();
    void tick().then(() => cardDialog?.querySelector('textarea')?.focus());
  }

  async function createCard(event: SubmitEvent) {
    event.preventDefault();
    if (saving || selectingFiles) return;
    const items = saveItems(editorItems);
    if (!hasContent(items) || (cardFolder === '__new' && !newFolderTitle.trim())) {
      formError = 'workspace.add_something_you_want_to_remember_and_choose_a_folder_for_this_card';
      return;
    }
    saving = true;
    formError = '';
    try {
      const card = await saveCard(
        backend,
        { folderId: cardFolder, newFolderTitle, editing: editingCard, items },
        (folder) => {
          cardFolder = folder.id;
          onfoldercreated(folder);
        },
      );
      onsaved(card, editingCard !== null);
      cardDialog?.close();
    } catch {
      formError = 'workspace.your_card_could_not_be_saved_your_entries_are_still_here_try_again';
    } finally {
      saving = false;
    }
  }
</script>

<dialog
  class="workspace-control workspace-card-dialog"
  bind:this={cardDialog}
  aria-labelledby="card-dialog-title"
  {onclose}
  oncancel={(event) => {
    if (saving || selectingFiles) event.preventDefault();
  }}
>
  <form onsubmit={createCard} aria-busy={saving || selectingFiles} novalidate>
    <div class="workspace-dialog-top">
      <span class="workspace-control workspace-empty-icon"><WorkspaceIcon name="cards" size={24} /></span><button
        type="button"
        class="workspace-control workspace-icon-button"
        aria-label={$l.t('workspace.close_card_editor')}
        disabled={saving || selectingFiles}
        onclick={() => cardDialog?.close()}><WorkspaceIcon name="close" /></button
      >
    </div>
    <p class="workspace-control workspace-eyebrow">{$l.t('workspace.a_note_to_your_future_self')}</p>
    <h2 class="workspace-control" id="card-dialog-title">
      {editingCard ? $l.t('workspace.edit_your_memory') : $l.t('workspace.what_s_worth_remembering')}
    </h2>
    <p class="workspace-control workspace-dialog-description">
      {$l.t('workspace.keep_information_together_with_text_images_audio_and_video')}
    </p>
    <CardEditor
      media={backend}
      bind:items={editorItems}
      bind:selecting={selectingFiles}
      disabled={saving}
      onerror={(message) => (formError = message)}
    />
    {#if cardFolder === '__new'}<label class="workspace-control" for="card-new-folder"
        >{$l.t('workspace.new_folder_name')}</label
      ><input
        class="workspace-control"
        dir="auto"
        id="card-new-folder"
        bind:value={newFolderTitle}
        placeholder={$l.t('workspace.e_g_everyday_spanish')}
        disabled={saving}
        required
        aria-invalid={!!formError && !newFolderTitle.trim()}
      />{/if}
    <p class="workspace-control workspace-save-note">
      <WorkspaceIcon name="clock" size={15} />
      {editingCard
        ? $l.t('workspace.your_review_schedule_stays_the_same')
        : $l.t('workspace.ready_for_your_first_review_as_soon_as_you_save')}
    </p>
    {#if formError}<p class="workspace-control workspace-error" role="alert">{formError ? $l.t(formError) : ''}</p>{/if}
    <div class="workspace-dialog-actions">
      <button
        type="button"
        class="workspace-control workspace-secondary-button"
        disabled={saving || selectingFiles}
        onclick={() => cardDialog?.close()}>{$l.t('cards.cancel')}</button
      ><button type="submit" class="workspace-control workspace-primary-button" disabled={saving || selectingFiles}
        >{saving
          ? $l.t('workspace.saving')
          : editingCard
            ? $l.t('workspace.save_changes')
            : $l.t('workspace.save_memory')}
        <WorkspaceIcon name="arrow" size={16} /></button
      >
    </div>
  </form>
</dialog>
