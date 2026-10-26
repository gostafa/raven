<script lang="ts">
  import type { Folder } from '../../folders';
  import { useLocalization } from '../../localization';
  import WorkspaceIcon from './WorkspaceIcon.svelte';

  let {
    dialog = $bindable(),
    folders,
    moving,
    busy,
    error,
    onsubmit,
    onclose,
  }: {
    dialog?: HTMLDialogElement;
    folders: Folder[];
    moving: boolean;
    busy: boolean;
    error: string;
    onsubmit: (folderId: string) => void;
    onclose: () => void;
  } = $props();
  const l = useLocalization();
  const id = $props.id();
  let search = $state('');
  let destination = $state('');
  const matches = $derived(
    folders.filter((folder) => folder.title.toLowerCase().includes(search.trim().toLowerCase())),
  );
</script>

<dialog
  class="workspace-control workspace-transfer-dialog"
  bind:this={dialog}
  aria-labelledby={id}
  onclick={(event) => {
    if (!busy && event.target === dialog) dialog?.close();
  }}
  oncancel={(event) => {
    if (busy) event.preventDefault();
  }}
  onclose={() => {
    search = '';
    destination = '';
    onclose();
  }}
>
  <form
    onsubmit={(event) => {
      event.preventDefault();
      if (!busy && folders.some((folder) => folder.id === destination)) onsubmit(destination);
    }}
    aria-busy={busy}
  >
    <div class="workspace-dialog-top">
      <h2 {id}>{$l.t(moving ? 'transfer.move_cards' : 'transfer.paste_cards')}</h2>
      <button
        type="button"
        class="workspace-control workspace-icon-button"
        disabled={busy}
        aria-label={$l.t('transfer.close_transfer')}
        onclick={() => dialog?.close()}><WorkspaceIcon name="close" /></button
      >
    </div>
    <label for={`${id}-search`}>{$l.t('transfer.search_folders')}</label>
    <input id={`${id}-search`} type="search" bind:value={search} disabled={busy} />
    <fieldset disabled={busy} class="workspace-transfer-folders">
      <legend>{$l.t('transfer.destination')}</legend>
      {#each matches as folder (folder.id)}
        <label class="workspace-transfer-folder">
          <input type="radio" name={`${id}-folder`} value={folder.id} bind:group={destination} />
          <span dir="auto">{folder.title}</span>
        </label>
      {:else}<p>{$l.t('transfer.no_destination')}</p>{/each}
    </fieldset>
    {#if error}<p class="workspace-control workspace-error" role="alert">{error}</p>{/if}
    <div class="workspace-dialog-actions">
      <button
        type="button"
        class="workspace-control workspace-secondary-button"
        disabled={busy}
        onclick={() => dialog?.close()}>{$l.t('cards.cancel')}</button
      >
      <button
        type="submit"
        class="workspace-control workspace-primary-button"
        disabled={busy || !folders.some((folder) => folder.id === destination)}
      >
        {$l.t(busy ? 'workspace.working' : moving ? 'transfer.move_cards' : 'transfer.paste')}
      </button>
    </div>
  </form>
</dialog>
