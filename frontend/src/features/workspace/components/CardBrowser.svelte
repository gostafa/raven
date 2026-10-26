<script lang="ts">
  import { toggleSelection, ChoiceMenu, type ChoiceOption } from '../../ui';
  import { reference, referenceKey, type CardReference } from '../../cardtransfer';
  import MemoryTile from './MemoryTile.svelte';
  import FolderActions from './FolderActions.svelte';
  import { useLocalization } from '../../localization';
  const l = useLocalization();
  import type { Folder } from '../../folders';
  import type { ReviewItem } from '../../reviews';
  import {
    RecallFilter,
    type Card,
    type CardSort,
    type CardContentKind,
    type CardProgressRange,
    type MediaURLPort,
    type PreviewStartPort,
  } from '../../cards';
  import WorkspaceIcon from './WorkspaceIcon.svelte';
  let {
    selectedFolder,
    query,
    search = $bindable(),
    saving,
    selectingFiles,
    scheduledItems,
    visibleFolders,
    visibleCards,
    matchingCards,
    cardSort = $bindable(),
    cardKinds = $bindable(),
    cardProgressRange = $bindable(),
    mediaURL,
    nativeStart,
    openCard,
    navigate,
    startFolderReview,
    exportSingleFolder,
    editFolder,
    confirmDelete,
    openPart,
    startReview,
    editCard,
    selectingCards,
    selectedCards,
    canPaste,
    clearClipboard,
    toggleSelectionMode,
    toggleSelected,
    selectAllShown,
    clearSelection,
    moveSelected,
    copySelected,
    reviewSelected,
    moveCard,
    copyCard,
    pasteCards,
  }: {
    selectingCards: boolean;
    selectedCards: CardReference[];
    canPaste: boolean;
    clearClipboard: () => void;
    toggleSelectionMode: () => void;
    toggleSelected: (card: Card) => void;
    selectAllShown: () => void;
    clearSelection: () => void;
    moveSelected: (opener: HTMLElement) => void;
    copySelected: (opener: HTMLElement) => void;
    reviewSelected: (opener: HTMLElement) => void;
    moveCard: (opener: HTMLElement, card: Card) => void;
    copyCard: (opener: HTMLElement, card: Card) => void;
    pasteCards: (opener: HTMLElement, folderId?: string) => void;
    selectedFolder: Folder | undefined;
    query: string;
    search: string;
    saving: boolean;
    selectingFiles: boolean;
    scheduledItems: ReviewItem[];
    visibleFolders: Folder[];
    visibleCards: Card[];
    matchingCards: Card[];
    cardSort: CardSort;
    cardKinds: CardContentKind[];
    cardProgressRange: CardProgressRange;
    mediaURL: MediaURLPort;
    nativeStart: PreviewStartPort;
    openCard: (event: MouseEvent) => void;
    navigate: (id: string) => void;
    startFolderReview: (opener: HTMLElement, folderId: string) => void;
    exportSingleFolder: (opener: HTMLElement, folderId: string) => void;
    editFolder: (opener: HTMLElement, folder: Folder) => void;
    confirmDelete: (opener: HTMLElement, folderId: string, card?: Card) => void;
    openPart: (card: Card, itemId: string, opener: HTMLElement) => void;
    startReview: (opener: HTMLElement, card: Card) => void;
    editCard: (opener: HTMLElement, card: Card) => void;
  } = $props();

  const selectedKeys = $derived(new Set(selectedCards.map(referenceKey)));

  const cardSortOptions: ChoiceOption<CardSort>[] = $derived([
    { value: 'created-desc', label: $l.t('workspace.newest_first') },
    { value: 'created-asc', label: $l.t('workspace.oldest_first') },
    { value: 'review-asc', label: $l.t('workspace.next_review_earliest_first') },
    { value: 'review-desc', label: $l.t('workspace.next_review_latest_first') },
    { value: 'progress-asc', label: $l.t('workspace.recall_progress_lowest_first') },
    { value: 'progress-desc', label: $l.t('workspace.recall_progress_highest_first') },
  ]);
  const cardKindOptions: ChoiceOption<CardContentKind>[] = $derived([
    { value: 'text', label: $l.t('workspace.text') },
    { value: 'image', label: $l.t('workspace.image') },
    { value: 'audio', label: $l.t('cards.audio') },
    { value: 'video', label: $l.t('cards.video') },
  ]);
  const hasRecallFilter = $derived(cardProgressRange.minimum > 0 || cardProgressRange.maximum < 100);
  const hasCardFilters = $derived(cardKinds.length > 0 || hasRecallFilter);
  function clearCardFilters() {
    cardKinds = [];
    cardProgressRange = { minimum: 0, maximum: 100 };
  }
</script>

<section class="workspace-library-heading">
  <div>
    <p class="workspace-control workspace-eyebrow">
      {query
        ? $l.t('workspace.a_little_help_finding_it')
        : selectedFolder
          ? $l.t('workspace.your_collection')
          : $l.t('workspace.things_worth_remembering')}
    </p>
    <h1 class="workspace-control">
      {query
        ? $l.t('workspace.search_your_memories')
        : (selectedFolder?.title ?? $l.t('workspace.search_your_memories'))}
    </h1>
    <p class="workspace-control">
      {query
        ? $l.t('workspace.searchResults', { query: search.trim() })
        : (selectedFolder?.description ?? '') ||
          $l.t('workspace.big_ideas_small_details_and_everything_you_want_to_keep')}
    </p>
    {#if selectedFolder?.tags.length}<div class="workspace-tags">
        {#each selectedFolder.tags as tag (tag)}<span class="workspace-control"><bdi>{tag}</bdi></span>{/each}
      </div>{/if}
  </div>
  <div class="workspace-heading-actions">
    {#if canPaste}<button
        type="button"
        class="workspace-control workspace-secondary-button"
        disabled={saving || selectingFiles}
        onclick={(event) => {
          pasteCards(event.currentTarget, selectedFolder?.id);
        }}>{$l.t('transfer.paste')}</button
      >
      <button
        type="button"
        class="workspace-control workspace-text-button"
        disabled={saving || selectingFiles}
        onclick={clearClipboard}>{$l.t('transfer.clear_clipboard')}</button
      >{/if}
    <button
      class="workspace-control workspace-primary-button"
      onclick={(event) => {
        openCard(event);
      }}><WorkspaceIcon name="plus" size={17} /> {$l.t('workspace.new_card')}</button
    >{#if selectedFolder}{@const folder = selectedFolder}<FolderActions
        {folder}
        hasReviews={scheduledItems.some((entry) => entry.card.folderId === folder.id)}
        {saving}
        {startFolderReview}
        {exportSingleFolder}
        {editFolder}
        {confirmDelete}
        {canPaste}
        {pasteCards}
      />{/if}
  </div>
</section>
{#if query && !selectedFolder && visibleFolders.length}<div class="workspace-section-heading">
    <h2 class="workspace-control">
      {$l.t('workspace.folders')}
      <span class="workspace-control workspace-count">{$l.number(visibleFolders.length)}</span>
    </h2>
  </div>
  <div class="workspace-search-folders">
    {#each visibleFolders as folder (folder.id)}<button
        class="workspace-control workspace-secondary-button"
        onclick={() => {
          navigate(folder.id);
        }}
        ><WorkspaceIcon name="folder" size={16} /> <bdi>{folder.title}</bdi>
        <WorkspaceIcon name="arrow" size={15} /></button
      >{/each}
  </div>{/if}
<div class="workspace-section-heading">
  <h2 class="workspace-control">
    {selectedFolder ? $l.t('workspace.memory_cards') : $l.t('workspace.your_cards')}
    <span class="workspace-control workspace-count">{$l.number(visibleCards.length)}</span>
  </h2>
  {#if query}<button
      class="workspace-control workspace-text-button"
      onclick={() => {
        search = '';
      }}>{$l.t('workspace.clear_search')} <WorkspaceIcon name="close" size={14} /></button
    >{:else}<span class="workspace-control workspace-section-note"
      >{$l.t(selectingCards ? 'transfer.selection_help' : 'workspace.select_a_card_to_practice_recall')}</span
    >{/if}
</div>
<div class="workspace-folder-controls" aria-label={$l.t('workspace.card_controls')}>
  <div class="workspace-folder-toolbar">
    <ChoiceMenu
      label={$l.t('workspace.sort')}
      options={cardSortOptions}
      selected={[cardSort]}
      onselect={(value: CardSort) => {
        cardSort = value;
      }}
    />
    <ChoiceMenu
      label={$l.t('workspace.content_type')}
      options={cardKindOptions}
      selected={cardKinds}
      multiple
      hint={$l.t('workspace.cards_match_any_selected_content_type_including_hints')}
      onselect={(kind: CardContentKind) => {
        cardKinds = toggleSelection(cardKinds, kind);
      }}
      onclear={() => {
        cardKinds = [];
      }}
    />
    <RecallFilter
      range={cardProgressRange}
      onchange={(range: CardProgressRange) => {
        cardProgressRange = range;
      }}
    />
    {#if !selectingCards}<button
        type="button"
        class="workspace-control workspace-secondary-button workspace-selection-toggle"
        disabled={saving || selectingFiles || !visibleCards.length}
        onclick={toggleSelectionMode}><WorkspaceIcon name="check" size={15} />{$l.t('transfer.select_cards')}</button
      >{/if}
  </div>
  <span class="workspace-control workspace-folder-result-count" role="status" aria-live="polite">
    {$l.t('common.cardsOfTotal', { count: matchingCards.length, shown: $l.number(visibleCards.length) })}
  </span>
</div>
{#if hasCardFilters}<div class="workspace-active-folder-tags" aria-label={$l.t('workspace.selected_card_filters')}>
    {#each cardKinds as kind (kind)}<button
        type="button"
        class="workspace-control workspace-filter-chip"
        aria-label={$l.t('workspace.removeKind', {
          kind: cardKindOptions.find((option) => option.value === kind)?.label ?? '',
        })}
        onclick={() => {
          cardKinds = cardKinds.filter((value) => value !== kind);
        }}
      >
        <span class="workspace-control">{cardKindOptions.find((option) => option.value === kind)?.label}</span
        ><WorkspaceIcon name="close" size={12} />
      </button>{/each}
    {#if hasRecallFilter}<button
        type="button"
        class="workspace-control workspace-filter-chip"
        aria-label={$l.t('workspace.removeRecall')}
        onclick={() => {
          cardProgressRange = { minimum: 0, maximum: 100 };
        }}
      >
        <span class="workspace-control"
          >{$l.t('workspace.recallRange', {
            minimum: $l.percent(cardProgressRange.minimum),
            maximum: $l.percent(cardProgressRange.maximum),
          })}</span
        ><WorkspaceIcon name="close" size={12} />
      </button>{/if}
    <button type="button" class="workspace-control workspace-clear-folder-tags" onclick={clearCardFilters}
      >{$l.t('workspace.clear_filters')}</button
    >
  </div>{/if}
{#if selectingCards}<div class="workspace-transfer-toolbar" role="group" aria-label={$l.t('transfer.select_cards')}>
    <div class="workspace-selection-summary">
      <button
        type="button"
        class="workspace-control workspace-primary-button"
        disabled={saving || selectingFiles}
        onclick={toggleSelectionMode}><WorkspaceIcon name="check" size={15} />{$l.t('transfer.done_selecting')}</button
      >
      <span class="workspace-selection-count" role="status"
        >{$l.t('transfer.selected_count', { count: selectedCards.length })}</span
      >
    </div>
    <div class="workspace-selection-utilities">
      <button
        type="button"
        class="workspace-control workspace-text-button"
        disabled={saving || selectingFiles || !visibleCards.length}
        onclick={selectAllShown}>{$l.t('transfer.select_all_shown')}</button
      >
      <button
        type="button"
        class="workspace-control workspace-text-button"
        disabled={saving || selectingFiles || !selectedCards.length}
        onclick={clearSelection}>{$l.t('transfer.clear_selection')}</button
      >
    </div>
    <div class="workspace-selection-actions">
      <button
        type="button"
        class="workspace-control workspace-secondary-button"
        disabled={saving || selectingFiles || !selectedCards.length}
        onclick={(event) => {
          moveSelected(event.currentTarget);
        }}><WorkspaceIcon name="folder" size={15} />{$l.t('transfer.move_to')}</button
      >
      <button
        type="button"
        class="workspace-control workspace-secondary-button"
        disabled={saving || selectingFiles || !selectedCards.length}
        onclick={(event) => {
          copySelected(event.currentTarget);
        }}>{$l.t('transfer.copy')}</button
      >
      <button
        type="button"
        class="workspace-control workspace-secondary-button"
        disabled={saving || selectingFiles || !selectedCards.length}
        onclick={(event) => {
          reviewSelected(event.currentTarget);
        }}>{$l.t('workspace.review_now')}</button
      >
    </div>
  </div>{/if}
{#if visibleCards.length}<ul class="workspace-memory-grid" aria-label={$l.t('workspace.memory_cards')}>
    {#each visibleCards as card (`${card.folderId}/${card.id}`)}
      <MemoryTile
        {card}
        selecting={selectingCards}
        selected={selectedKeys.has(referenceKey(reference(card)))}
        {toggleSelected}
        {moveCard}
        {copyCard}
        {mediaURL}
        {nativeStart}
        disabled={saving || selectingFiles}
        {openPart}
        {startReview}
        {editCard}
        {confirmDelete}
      />
    {/each}
  </ul>
{:else}<div class="workspace-empty-state">
    <div class="workspace-empty-icon">
      <WorkspaceIcon name={query || hasCardFilters ? 'search' : 'cards'} size={27} />
    </div>
    <h2 class="workspace-control">
      {query || hasCardFilters ? $l.t('workspace.noCardsMatch') : $l.t('workspace.nextMemory')}
    </h2>
    <p class="workspace-control">
      {hasCardFilters
        ? $l.t('workspace.removeFilterHelp')
        : query
          ? $l.t('workspace.searchHelp')
          : $l.t('workspace.addHelp')}
    </p>
    {#if hasCardFilters}<button class="workspace-control workspace-secondary-button" onclick={clearCardFilters}
        >{$l.t('workspace.clear_filters')}</button
      >{/if}
    {#if query}<button
        class="workspace-control workspace-secondary-button"
        onclick={() => {
          search = '';
        }}>{$l.t('workspace.clear_search')}</button
      >{:else if !hasCardFilters}<button
        class="workspace-control workspace-primary-button"
        onclick={(event) => {
          openCard(event);
        }}><WorkspaceIcon name="plus" size={16} /> {$l.t('workspace.createCard')}</button
      >{/if}
  </div>{/if}
