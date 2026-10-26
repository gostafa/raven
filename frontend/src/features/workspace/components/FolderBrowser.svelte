<script lang="ts">
  import { tick } from 'svelte';
  import { toggleSelection, ChoiceMenu, type ChoiceOption } from '../../ui';
  import FolderActions from './FolderActions.svelte';
  import FolderProgress from './FolderProgress.svelte';
  import { useLocalization } from '../../localization';
  const l = useLocalization();
  import { FolderTile, parseTags, selectFolderPapers, type Folder, type FolderSort } from '../../folders';
  import type { Card, MediaURLPort, PreviewStartPort } from '../../cards';
  import type { ReviewItem } from '../../reviews';
  import WorkspaceIcon from './WorkspaceIcon.svelte';
  let {
    saving,
    folders,
    filteredFolders,
    folderSort = $bindable(),
    folderTags = $bindable(),
    cards,
    mediaURL,
    nativeStart,
    scheduledItems,
    now,
    openFolder,
    navigate,
    cardsInFolder,
    startFolderReview,
    exportSingleFolder,
    exportFolderSelection,
    editFolder,
    confirmDelete,
    canPaste,
    clearClipboard,
    pasteCards,
  }: {
    canPaste: boolean;
    clearClipboard: () => void;
    pasteCards: (opener: HTMLElement, folderId?: string) => void;
    saving: boolean;
    folders: Folder[];
    filteredFolders: Folder[];
    folderSort: FolderSort;
    folderTags: string[];
    cards: Card[];
    mediaURL: MediaURLPort;
    nativeStart: PreviewStartPort;
    scheduledItems: ReviewItem[];
    now: number;
    openFolder: (event: MouseEvent) => void;
    navigate: (id: string) => void;
    cardsInFolder: (id: string) => Card[];
    startFolderReview: (opener: HTMLElement, folderId: string) => void;
    exportSingleFolder: (opener: HTMLElement, folderId: string) => void;
    exportFolderSelection: (opener: HTMLElement, folderIds: string[]) => void;
    editFolder: (opener: HTMLElement, folder: Folder) => void;
    confirmDelete: (opener: HTMLElement, folderId: string) => void;
  } = $props();

  let selectingFolders = $state(false);
  let selectedFolderIds = $state<string[]>([]);
  let selectButton = $state<HTMLButtonElement>();
  let doneButton = $state<HTMLButtonElement>();
  const currentSelection = $derived(
    selectedFolderIds.filter((id) => filteredFolders.some((folder) => folder.id === id)),
  );
  const selectedIds = $derived(new Set(currentSelection));
  $effect(() => {
    if (currentSelection.length !== selectedFolderIds.length) selectedFolderIds = currentSelection;
  });

  async function toggleFolderSelectionMode() {
    if (saving) return;
    selectingFolders = !selectingFolders;
    selectedFolderIds = [];
    await tick();
    (selectingFolders ? doneButton : selectButton)?.focus();
  }

  function activateFolder(folderId: string) {
    if (saving) return;
    if (selectingFolders) selectedFolderIds = toggleSelection(currentSelection, folderId);
    else navigate(folderId);
  }

  const folderSortOptions: ChoiceOption<FolderSort>[] = $derived([
    { value: 'default', label: $l.t('workspace.default_order') },
    { value: 'tags-desc', label: $l.t('workspace.tags_most_first') },
    { value: 'tags-asc', label: $l.t('workspace.tags_fewest_first') },
    { value: 'cards-desc', label: $l.t('workspace.cards_most_first') },
    { value: 'cards-asc', label: $l.t('workspace.cards_fewest_first') },
    { value: 'review-asc', label: $l.t('workspace.next_review_earliest_first') },
    { value: 'review-desc', label: $l.t('workspace.next_review_latest_first') },
  ]);
  const availableTags = $derived(
    parseTags(folders.flatMap((folder) => folder.tags).join(',')).toSorted((a, b) => $l.compare(a, b)),
  );
  const tagOptions = $derived(availableTags.map((tag) => ({ value: tag, label: tag })));
</script>

<section class="workspace-library-heading">
  <div>
    <p class="workspace-control workspace-eyebrow">{$l.t('workspace.your_space')}</p>
    <h1 class="workspace-control">{$l.t('workspace.your_folders')}</h1>
    <p class="workspace-control">{$l.t('workspace.a_home_for_the_things_you_want_to_remember')}</p>
  </div>
  <div class="workspace-heading-actions">
    {#if canPaste}<button
        type="button"
        class="workspace-control workspace-secondary-button"
        disabled={saving}
        onclick={(event) => {
          pasteCards(event.currentTarget);
        }}>{$l.t('transfer.paste')}</button
      >
      <button type="button" class="workspace-control workspace-text-button" disabled={saving} onclick={clearClipboard}
        >{$l.t('transfer.clear_clipboard')}</button
      >{/if}
    <button class="workspace-control workspace-primary-button" disabled={saving} onclick={openFolder}
      ><WorkspaceIcon name="plus" size={17} /> {$l.t('workspace.new_folder')}</button
    >
  </div>
</section>
<div class="workspace-folder-controls" aria-label={$l.t('workspace.folder_controls')}>
  <div class="workspace-folder-toolbar">
    <ChoiceMenu
      label={$l.t('workspace.sort')}
      options={folderSortOptions}
      selected={[folderSort]}
      onselect={(value: FolderSort) => {
        folderSort = value;
      }}
    />
    <ChoiceMenu
      label={$l.t('workspace.tags')}
      options={tagOptions}
      selected={folderTags}
      multiple
      searchable
      hint={$l.t('workspace.folders_must_match_every_selected_tag')}
      onselect={(tag: string) => {
        folderTags = toggleSelection(folderTags, tag);
      }}
      onclear={() => {
        folderTags = [];
      }}
    />
    {#if !selectingFolders}<button
        bind:this={selectButton}
        type="button"
        class="workspace-control workspace-secondary-button workspace-selection-toggle"
        disabled={saving || !filteredFolders.length}
        onclick={toggleFolderSelectionMode}
        ><WorkspaceIcon name="check" size={15} />{$l.t('workspace.select_folders')}</button
      >{/if}
  </div>
  <span class="workspace-control workspace-folder-result-count" role="status"
    >{$l.t('common.folders', { count: filteredFolders.length })}{folderTags.length
      ? $l.t('common.ofTotal', { count: folders.length })
      : ''}</span
  >
</div>
{#if folderTags.length}<div class="workspace-active-folder-tags" aria-label={$l.t('workspace.selected_tag_filters')}>
    {#each folderTags as tag (tag)}<button
        type="button"
        class="workspace-control workspace-filter-chip"
        aria-label={$l.t('workspace.removeTag', { name: tag })}
        onclick={() => {
          folderTags = folderTags.filter((value) => value !== tag);
        }}
      >
        <span class="workspace-control"><bdi>{tag}</bdi></span><WorkspaceIcon name="close" size={12} />
      </button>{/each}
    <button
      class="workspace-control workspace-clear-folder-tags"
      type="button"
      onclick={() => {
        folderTags = [];
      }}>{$l.t('workspace.clear_all')}</button
    >
  </div>{/if}
{#if folderTags.length && !filteredFolders.length}<p class="workspace-control workspace-folder-filter-empty">
    {$l.t('workspace.no_folders_match_all_selected_tags')}
  </p>{/if}
{#if selectingFolders}<div
    class="workspace-transfer-toolbar"
    role="group"
    aria-label={$l.t('workspace.select_folders')}
  >
    <div class="workspace-selection-summary">
      <button
        bind:this={doneButton}
        type="button"
        class="workspace-control workspace-primary-button"
        disabled={saving}
        onclick={toggleFolderSelectionMode}
        ><WorkspaceIcon name="check" size={15} />{$l.t('transfer.done_selecting')}</button
      >
      <span class="workspace-selection-count" role="status"
        >{$l.t('common.selected', { count: currentSelection.length })}</span
      >
    </div>
    <div class="workspace-selection-utilities">
      <button
        type="button"
        class="workspace-control workspace-text-button"
        disabled={saving || !filteredFolders.length}
        onclick={() => {
          selectedFolderIds = filteredFolders.map((folder) => folder.id);
        }}>{$l.t('transfer.select_all_shown')}</button
      >
      <button
        type="button"
        class="workspace-control workspace-text-button"
        disabled={saving || !currentSelection.length}
        onclick={() => {
          selectedFolderIds = [];
        }}>{$l.t('transfer.clear_selection')}</button
      >
    </div>
    <div class="workspace-selection-actions">
      <button
        type="button"
        class="workspace-control workspace-secondary-button"
        disabled={saving || !currentSelection.length}
        onclick={(event) => {
          exportFolderSelection(event.currentTarget, currentSelection);
        }}
        ><WorkspaceIcon name="export" size={15} />{$l.t(
          saving ? 'archive.exporting' : 'workspace.export_selected_folders',
        )}</button
      >
    </div>
  </div>{/if}
<ul class="workspace-folder-grid" aria-label={$l.t('workspace.your_folders')}>
  {#if !selectingFolders}<li class="workspace-control workspace-folder-tile">
      <FolderTile papers={[]} {mediaURL}>
        <button class="workspace-control workspace-folder-card workspace-new-folder-card" onclick={openFolder}
          ><span class="workspace-control"><WorkspaceIcon name="plus" size={22} /></span><strong
            class="workspace-control">{$l.t('workspace.a_new_curiosity')}</strong
          ><span class="workspace-control">{$l.t('workspace.give_it_a_folder')}</span></button
        >
      </FolderTile>
    </li>{/if}
  {#each filteredFolders as folder (folder.id)}
    <li
      class="workspace-control workspace-folder-tile"
      class:workspace-folder-selecting={selectingFolders}
      class:workspace-folder-selected={selectedIds.has(folder.id)}
    >
      <FolderTile
        papers={selectFolderPapers(cards, folder.id)}
        {mediaURL}
        {nativeStart}
        selecting={selectingFolders}
        selected={selectedIds.has(folder.id)}
      >
        <button
          class="workspace-control workspace-folder-card"
          disabled={saving}
          aria-pressed={selectingFolders ? selectedIds.has(folder.id) : undefined}
          aria-label={selectingFolders ? $l.t('workspace.select_folder', { name: folder.title }) : undefined}
          onclick={() => {
            activateFolder(folder.id);
          }}
        >
          {#if selectingFolders}<span class="workspace-selection-check workspace-folder-selection-check"
              ><WorkspaceIcon name="check" size={14} /></span
            >{/if}
          <div class="workspace-folder-card-content">
            <h3 class="workspace-control"><bdi>{folder.title}</bdi></h3>
            <p class="workspace-control" dir="auto">
              {folder.description || $l.t('workspace.a_collection_of_things_worth_remembering')}
            </p>
            <span class="workspace-control workspace-folder-card-bottom">
              {#if folder.tags.length}<span class="workspace-control workspace-folder-tags"
                  >{#each folder.tags as tag (tag)}<span class="workspace-control workspace-folder-tag"
                      ><bdi>{tag}</bdi></span
                    >{/each}</span
                >{/if}
              {#if selectingFolders}<span class="workspace-folder-selection-count"
                  >{$l.t('common.cards', { count: cardsInFolder(folder.id).length })}</span
                >{:else if scheduledItems.some((entry) => entry.card.folderId === folder.id && entry.dueAt <= now)}<span
                  class="workspace-control workspace-due-dot">{$l.t('workspace.ready_to_review')}</span
                >{/if}
            </span>
          </div>
          {#if !selectingFolders}<FolderProgress cards={cardsInFolder(folder.id)} />{/if}
        </button>
        {#if !selectingFolders}<span class="workspace-control workspace-tile-actions"
            ><FolderActions
              {folder}
              hasReviews={scheduledItems.some((entry) => entry.card.folderId === folder.id)}
              {saving}
              appearance="quiet"
              {startFolderReview}
              {exportSingleFolder}
              {editFolder}
              {confirmDelete}
              {canPaste}
              {pasteCards}
            /></span
          >{/if}
      </FolderTile>
    </li>
  {/each}
</ul>
