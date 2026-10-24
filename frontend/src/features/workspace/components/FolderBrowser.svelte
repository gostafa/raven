<script lang="ts">
  import { toggleSelection } from '../../ui';
  import FolderActions from './FolderActions.svelte';
  import FolderProgress from './FolderProgress.svelte';
  import { useLocalization } from '../../localization';
  const l = useLocalization();
  import { FolderTile, parseTags, selectFolderPapers, type Folder, type FolderSort } from '../../folders';
  import type { Card, MediaURLPort, PreviewStartPort } from '../../cards';
  import type { ReviewItem } from '../../reviews';
  import { ChoiceMenu, type ChoiceOption } from '../../ui';
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
    editFolder,
    confirmDelete,
  }: {
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
    editFolder: (opener: HTMLElement, folder: Folder) => void;
    confirmDelete: (opener: HTMLElement, folderId: string) => void;
  } = $props();

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
      onselect={(value) => {
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
      onselect={(tag) => {
        folderTags = toggleSelection(folderTags, tag);
      }}
      onclear={() => {
        folderTags = [];
      }}
    />
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
<ul class="workspace-folder-grid" aria-label={$l.t('workspace.your_folders')}>
  <li class="workspace-control workspace-folder-tile">
    <FolderTile papers={[]} {mediaURL}>
      <button class="workspace-control workspace-folder-card workspace-new-folder-card" onclick={openFolder}
        ><span class="workspace-control"><WorkspaceIcon name="plus" size={22} /></span><strong class="workspace-control"
          >{$l.t('workspace.a_new_curiosity')}</strong
        ><span class="workspace-control">{$l.t('workspace.give_it_a_folder')}</span></button
      >
    </FolderTile>
  </li>
  {#each filteredFolders as folder (folder.id)}
    <li class="workspace-control workspace-folder-tile">
      <FolderTile papers={selectFolderPapers(cards, folder.id)} {mediaURL} {nativeStart}>
        <button class="workspace-control workspace-folder-card" onclick={() => navigate(folder.id)}>
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
              {#if scheduledItems.some((entry) => entry.card.folderId === folder.id && entry.dueAt <= now)}<span
                  class="workspace-control workspace-due-dot">{$l.t('workspace.ready_to_review')}</span
                >{/if}
            </span>
          </div>
          <FolderProgress cards={cardsInFolder(folder.id)} />
        </button>
        <span class="workspace-control workspace-tile-actions"
          ><FolderActions
            {folder}
            hasReviews={scheduledItems.some((entry) => entry.card.folderId === folder.id)}
            {saving}
            appearance="quiet"
            {startFolderReview}
            {exportSingleFolder}
            {editFolder}
            {confirmDelete}
          /></span
        >
      </FolderTile>
    </li>
  {/each}
</ul>
