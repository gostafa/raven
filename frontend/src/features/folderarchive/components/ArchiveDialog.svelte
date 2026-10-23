<script lang="ts">
  import { useLocalization } from '../../localization';
  const l = useLocalization();
  import type { Folder } from '../../folders';
  import type { ImportMode } from '../domain/types';

  let {
    operation,
    folders,
    busy,
    error,
    onexport,
    onimport,
    onclose,
  }: {
    operation: 'import' | 'export' | null;
    folders: Folder[];
    busy: boolean;
    error: string;
    onexport(ids: string[]): void;
    onimport(mode: ImportMode): void;
    onclose(): void;
  } = $props();
  let dialog = $state<HTMLDialogElement>();
  let selected = $state<string[]>([]);
  let mode = $state<ImportMode | ''>('');

  $effect(() => {
    if (operation) {
      selected = [];
      mode = '';
      dialog?.showModal();
    } else {
      dialog?.close();
    }
  });

  function submit(event: SubmitEvent) {
    event.preventDefault();
    if (busy) return;
    if (operation === 'export' && selected.length) onexport(selected);
    if (operation === 'import' && mode) onimport(mode);
  }
</script>

<dialog
  bind:this={dialog}
  aria-labelledby="archive-title"
  aria-describedby="archive-description"
  {onclose}
  oncancel={(event) => {
    if (busy) event.preventDefault();
  }}
>
  <form onsubmit={submit} aria-busy={busy}>
    <h2 id="archive-title">
      {operation === 'export' ? $l.t('workspace.export_folders') : $l.t('workspace.import_folders')}
    </h2>
    {#if operation === 'export'}
      <p id="archive-description">
        {$l.t('folderarchive.save_folders_in_one_raven_zip_file_including_all_cards_media_and_review_history')}
      </p>
      <div class="selection-actions">
        <span aria-live="polite"
          >{$l.t('archive.selection', { selected: $l.number(selected.length), total: $l.number(folders.length) })}</span
        >
        <button
          type="button"
          class="select-all"
          disabled={busy}
          onclick={() => (selected = selected.length === folders.length ? [] : folders.map((folder) => folder.id))}
          >{selected.length === folders.length
            ? $l.t('folderarchive.clear_selection')
            : $l.t('folderarchive.select_all')}</button
        >
      </div>
      <fieldset class="folder-selection" disabled={busy}>
        <legend>{$l.t('folderarchive.folders_to_export')}</legend>
        {#each folders as folder (folder.id)}
          <label
            ><input type="checkbox" bind:group={selected} value={folder.id} /><span><bdi>{folder.title}</bdi></span
            ></label
          >
        {/each}
      </fieldset>
    {:else}
      <p id="archive-description">
        {$l.t('folderarchive.open_a_raven_zip_file_to_add_copies_of_its_folders_to_your_library')}
      </p>
      <fieldset disabled={busy}>
        <legend>{$l.t('folderarchive.how_would_you_like_to_review_the_imported_cards')}</legend>
        <label
          ><input type="radio" bind:group={mode} value="fresh" /><span
            ><strong>{$l.t('folderarchive.start_from_zero')}</strong><small
              >{$l.t('folderarchive.reset_review_progress_and_treat_every_imported_card_as_new')}</small
            ></span
          ></label
        >
        <label
          ><input type="radio" bind:group={mode} value="resume" /><span
            ><strong>{$l.t('folderarchive.continue_from_last_review')}</strong><small
              >{$l.t(
                'folderarchive.keep_saved_review_history_and_due_dates_cards_already_due_are_ready_to_review',
              )}</small
            ></span
          ></label
        >
      </fieldset>
    {/if}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <div class="actions">
      <button type="button" disabled={busy} onclick={() => dialog?.close()}>{$l.t('cards.cancel')}</button>
      <button class="primary" type="submit" disabled={busy || (operation === 'export' ? !selected.length : !mode)}
        >{busy
          ? operation === 'export'
            ? $l.t('archive.exporting')
            : $l.t('archive.importing')
          : operation === 'export'
            ? $l.t('folderarchive.choose_save_location')
            : $l.t('folderarchive.choose_file')}</button
      >
    </div>
  </form>
</dialog>

<style>
  dialog {
    box-sizing: border-box;
    width: min(480px, calc(100vw - 32px));
    max-height: calc(100dvh - 32px);
    overflow-y: auto;
    padding: 28px;
    border: 1px solid var(--line);
    border-radius: 16px;
    background: #000;
    color: #fff;
  }
  dialog::backdrop {
    background: #000b;
  }
  h2 {
    margin: 0 0 12px;
    font-size: 24px;
  }
  p {
    color: #a3a3a3;
    line-height: 1.5;
  }
  fieldset {
    min-width: 0;
    margin: 16px 0;
    padding: 0;
    border: 0;
  }
  legend {
    box-sizing: border-box;
    width: 100%;
    padding: 0;
    margin-bottom: 10px;
    color: #a3a3a3;
    font-size: 13px;
    line-height: 1.5;
  }
  .folder-selection {
    max-height: 45dvh;
    overflow-y: auto;
  }
  label + label {
    margin-top: 8px;
  }
  label {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 12px;
    border: 1px solid var(--line);
    border-radius: 8px;
    cursor: pointer;
  }
  label:hover {
    border-color: var(--line-strong);
  }
  label:has(input:checked) {
    border-color: var(--line-strong);
    background: #ffffff0d;
  }
  label span {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  input {
    flex: 0 0 auto;
    margin-top: 3px;
    accent-color: #fff;
  }
  strong,
  small {
    display: block;
  }
  small {
    margin-top: 6px;
    color: #a3a3a3;
    line-height: 1.5;
  }
  button {
    padding: 10px 16px;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: #000;
    color: #fff;
    cursor: pointer;
    font: inherit;
  }
  button:hover {
    border-color: var(--line-strong);
  }
  button:focus-visible,
  input:focus-visible {
    outline: 2px solid #a3a3a3;
    outline-offset: 3px;
  }
  button:disabled {
    opacity: 0.45;
    cursor: default;
  }
  .primary {
    background: #fff;
    color: #000;
  }
  .select-all {
    font-size: 13px;
  }
  .selection-actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    color: #a3a3a3;
    font-size: 13px;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 10px;
    margin-top: 24px;
  }
  .error {
    color: #fff;
  }
  @media (max-width: 480px) {
    dialog {
      padding: 20px;
    }
    .actions button {
      flex: 1 1 auto;
    }
  }
</style>
