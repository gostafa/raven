<script lang="ts">
  import { useLocalization } from '../../localization';
  const l = useLocalization();
  import type { MessageKey } from '../../localization';
  import { CardContent, type Card, type MediaURLPort } from '../../cards';

  import WorkspaceIcon from './WorkspaceIcon.svelte';
  let {
    partDialog = $bindable(),
    partDeleteDialog = $bindable(),
    deleteDialog = $bindable(),
    selectedPart = $bindable(),
    partError = $bindable(),
    saving,
    deleteError,
    deletion,
    mediaURL,
    restoreDialogFocus,
    removePart,
    removeSelection,
  }: {
    partDialog: HTMLDialogElement | undefined;
    partDeleteDialog: HTMLDialogElement | undefined;
    deleteDialog: HTMLDialogElement | undefined;
    selectedPart: { card: Card; itemId: string } | null;
    partError: MessageKey | '';
    saving: boolean;
    deleteError: MessageKey | '';
    deletion: { folderId: string; cardId?: string; title: string } | null;
    mediaURL: MediaURLPort;
    restoreDialogFocus: () => Promise<void>;
    removePart: () => Promise<void>;
    removeSelection: (event: SubmitEvent) => Promise<void>;
  } = $props();
</script>

<dialog
  class="workspace-control workspace-card-dialog workspace-part-dialog"
  bind:this={partDialog}
  aria-label={$l.t('workspace.contentPreview')}
  onclose={() => {
    selectedPart = null;
    void restoreDialogFocus();
  }}
  oncancel={(event) => {
    if (saving) event.preventDefault();
  }}
>
  {#if selectedPart}
    <div class="workspace-part-toolbar" aria-busy={saving}>
      <button
        type="button"
        class="workspace-control workspace-icon-button"
        disabled={saving}
        onclick={() => {
          partError = '';
          partDeleteDialog?.showModal();
        }}
        aria-label={saving ? $l.t('workspace.removing') : $l.t('workspace.deletePart')}
        title={$l.t('workspace.deletePart')}><WorkspaceIcon name="trash" /></button
      >
      <button
        type="button"
        class="workspace-control workspace-icon-button"
        disabled={saving}
        onclick={() => partDialog?.close()}
        aria-label={$l.t('cards.cancel')}
        title={$l.t('cards.cancel')}><WorkspaceIcon name="close" /></button
      >
    </div>
    <div class="workspace-part-body">
      <CardContent
        card={{
          ...selectedPart.card,
          items: selectedPart.card.items.filter((item) => item.id === selectedPart?.itemId),
        }}
        {mediaURL}
      />
      {#if selectedPart.card.items.length === 1}
        <p class="workspace-control workspace-field-hint">{$l.t('workspace.lastPartHelp')}</p>
      {/if}
      {#if partError}<p class="workspace-control workspace-error" role="alert">
          {partError ? $l.t(partError) : ''}
        </p>{/if}
    </div>
  {/if}
</dialog>

<dialog
  class="workspace-control"
  bind:this={partDeleteDialog}
  aria-labelledby="part-delete-title"
  aria-describedby="part-delete-description"
  oncancel={(event) => {
    if (saving) event.preventDefault();
  }}
>
  <form
    onsubmit={(event) => {
      event.preventDefault();
      void removePart();
    }}
    aria-busy={saving}
  >
    <h2 class="workspace-control" id="part-delete-title">
      {$l.t('workspace.deleteTitle', {
        kind: $l.t(selectedPart?.card.items.length === 1 ? 'cards.card' : 'cards.part'),
      })}
    </h2>
    <p id="part-delete-description" class="workspace-control workspace-dialog-description">
      {selectedPart?.card.items.length === 1 ? $l.t('workspace.lastPartDelete') : $l.t('workspace.partDelete')}
      {$l.t('workspace.this_cannot_be_undone')}
    </p>
    {#if partError}<p class="workspace-control workspace-error" role="alert">{partError ? $l.t(partError) : ''}</p>{/if}
    <div class="workspace-dialog-actions">
      <button
        type="button"
        class="workspace-control workspace-secondary-button"
        disabled={saving}
        onclick={() => partDeleteDialog?.close()}>{$l.t('cards.cancel')}</button
      >
      <button type="submit" class="workspace-control workspace-primary-button" disabled={saving}
        >{saving ? $l.t('workspace.deleting') : $l.t('workspace.delete_permanently')}</button
      >
    </div>
  </form>
</dialog>

<dialog
  class="workspace-control"
  bind:this={deleteDialog}
  aria-labelledby="delete-title"
  onclose={restoreDialogFocus}
  oncancel={(event) => {
    if (saving) event.preventDefault();
  }}
>
  <form onsubmit={removeSelection} aria-busy={saving}>
    <h2 class="workspace-control" id="delete-title">
      {$l.t('workspace.deleteTitle', { kind: $l.t(deletion?.cardId ? 'cards.card' : 'cards.folder') })}
    </h2>
    <p class="workspace-control workspace-dialog-description">
      {$l.t('workspace.deleteNamed', { name: deletion?.title ?? '' })}
      {deletion?.cardId
        ? $l.t('workspace.its_content_and_review_history_will_be_removed')
        : $l.t('workspace.folderDeleted')}
      {$l.t('workspace.this_cannot_be_undone')}
    </p>
    {#if deleteError}<p class="workspace-control workspace-error" role="alert">
        {deleteError ? $l.t(deleteError) : ''}
      </p>{/if}
    <div class="workspace-dialog-actions">
      <button
        type="button"
        class="workspace-control workspace-secondary-button"
        disabled={saving}
        onclick={() => deleteDialog?.close()}>{$l.t('cards.cancel')}</button
      ><button type="submit" class="workspace-control workspace-primary-button" disabled={saving}
        >{saving ? $l.t('workspace.deleting') : $l.t('workspace.delete_permanently')}</button
      >
    </div>
  </form>
</dialog>
