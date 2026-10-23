<script lang="ts">
  import { useLocalization, LanguageSelector, type MessageKey } from '../../localization';
  const l = useLocalization();

  import { onMount, tick } from 'svelte';
  import type { Card } from '../../cards';
  import type { AppPorts } from '../domain/types';
  import {
    loadWorkspace as loadData,
    saveCard as saveMemory,
    deleteSelection,
    loadViewPreferences,
    saveViewPreferences,
  } from '../application/funcs';
  import { defaultViewPreferences } from '../domain/funcs';
  import { CardEditor } from '../../cards';
  import { CardContent } from '../../cards';
  import { RecallFilter } from '../../cards';
  import { ReviewSession } from '../../reviews';
  import { ActionMenu, ChoiceMenu, Skeleton, type ChoiceOption } from '../../ui';
  import {
    ArchiveDialog,
    exportFolders,
    importFolders,
    type ArchiveResult,
    type ImportMode,
  } from '../../folderarchive';
  import {
    cardPreview,
    cardProgress,
    selectCards,
    type CardContentKind,
    type CardProgressRange,
    type CardSort,
    editItems,
    hasContent,
    matchesCard,
    mergeCardSnapshot,
    removeCardPart,
    saveItems,
    textDraft,
    type EditorItem,
  } from '../../cards';
  import { reviewItems, shuffledReviewItems, type ReviewItem } from '../../reviews';
  import {
    FolderTile,
    selectFolderPapers,
    selectFolders,
    type FolderSort,
    emptyWorkspace,
    loadPreferences,
    parseTags,
    reconcileFolders,
    savePreferences,
    type Workspace,
    type Folder,
  } from '../../folders';

  let { backend }: { backend: AppPorts } = $props();
  let error = $state<MessageKey | ''>('');
  let preferencesError = $state<MessageKey | ''>('');
  let viewPreferencesError = $state<MessageKey | ''>('');
  const initialView = restoreViewPreferences();
  let workspace = $state(restoreWorkspace());
  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let cards = $state<Card[]>([]);
  let now = $state(readNow());
  function readNow() {
    return backend.now();
  }
  const selectedFolder = $derived(workspace.folders.find((folder) => folder.id === workspace.selectedFolderId));
  const scheduledItems = $derived(reviewItems(cards));
  const dueCards = $derived(scheduledItems.filter((entry) => entry.dueAt <= now));
  let search = $state(initialView.search);
  let cardSort = $state<CardSort>(initialView.cardSort);
  let cardKinds = $state<CardContentKind[]>(initialView.cardKinds);
  let cardProgressRange = $state<CardProgressRange>(initialView.cardProgressRange);
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
  let foldersOpen = $state(initialView.foldersOpen);
  let folderTags = $state<string[]>(initialView.folderTags);
  let folderSort = $state<FolderSort>(initialView.folderSort);
  $effect(() => {
    try {
      saveViewPreferences(backend.preferences, backend.configuration.preferencesStorageKey, {
        search,
        foldersOpen,
        folderTags,
        folderSort,
        cardSort,
        cardKinds,
        cardProgressRange,
      });
      viewPreferencesError = '';
    } catch {
      viewPreferencesError = 'workspace.your_filters_and_sort_preferences_could_not_be_saved_on_this_device';
    }
  });

  function restoreViewPreferences() {
    try {
      return loadViewPreferences(backend.preferences, backend.configuration.preferencesStorageKey);
    } catch {
      viewPreferencesError = 'workspace.your_filters_and_sort_preferences_could_not_be_loaded';
      return defaultViewPreferences();
    }
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
    parseTags(workspace.folders.flatMap((folder) => folder.tags).join(',')).toSorted((a, b) => $l.compare(a, b)),
  );
  const tagOptions = $derived(availableTags.map((tag) => ({ value: tag, label: tag })));
  const folderMetrics = $derived(
    new Map(
      workspace.folders.map((folder) => {
        const reviews = scheduledItems.filter(
          (entry) => entry.card.folderId === folder.id && Number.isFinite(entry.dueAt),
        );
        return [
          folder.id,
          {
            cardCount: cards.filter((card) => card.folderId === folder.id).length,
            nextReview: reviews.length ? Math.min(...reviews.map((entry) => entry.dueAt)) : null,
          },
        ];
      }),
    ),
  );
  const filteredFolders = $derived(selectFolders(workspace.folders, folderTags, folderSort, folderMetrics, $l.compare));
  const query = $derived(search.trim().toLowerCase());
  const visibleFolders = $derived(
    filteredFolders.filter((folder) =>
      [folder.title, folder.description, ...folder.tags].some((value) => value.toLowerCase().includes(query)),
    ),
  );
  const matchingCards = $derived(
    cards.filter(
      (card) =>
        (!selectedFolder || card.folderId === selectedFolder.id) && matchesCard(card, folderName(card.folderId), query),
    ),
  );
  const visibleCards = $derived(
    selectCards(matchingCards, { sort: cardSort, kinds: cardKinds, progressRange: cardProgressRange }),
  );
  const shortcut = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘ K' : 'Ctrl K';
  let searchInput = $state<HTMLInputElement>();
  let sidebarToggle = $state<HTMLButtonElement>();
  let folderDialog = $state<HTMLDialogElement>();
  let cardDialog = $state<HTMLDialogElement>();
  let partDialog = $state<HTMLDialogElement>();
  let selectedPart = $state<{ card: Card; itemId: string } | null>(null);
  let partError = $state<MessageKey | ''>('');
  let partDeleteDialog = $state<HTMLDialogElement>();
  let deleteDialog = $state<HTMLDialogElement>();
  let deletion = $state<{ folderId: string; cardId?: string; title: string } | null>(null);
  let deleteError = $state<MessageKey | ''>('');
  let reviewDialog = $state<HTMLDialogElement>();
  let titleInput = $state<HTMLInputElement>();
  let editingFolder = $state<Folder | null>(null);
  let title = $state('');
  let description = $state('');
  let tags = $state('');
  let editorItems = $state<EditorItem[]>([]);
  let editingCard = $state<Card | null>(null);
  let selectingFiles = $state(false);
  let cardFolder = $state('');
  let newFolderTitle = $state('');
  let formError = $state<MessageKey | ''>('');
  let saving = $state(false);
  let dialogOpener: HTMLElement | null = null;
  let dialogFallback: HTMLElement | null = null;
  let reviewQueue = $state<ReviewItem[]>([]);
  let reviewOpen = $state(false);
  let reviewSaving = $state(false);
  let archiveOperation = $state<'import' | 'export' | null>(null);
  let archiveError = $state<MessageKey | ''>('');

  function openArchive(event: MouseEvent, operation: 'import' | 'export') {
    if (saving || status !== 'ready') return;
    rememberOpener(event);
    archiveError = '';
    archiveOperation = operation;
  }

  async function refreshImportedFolders() {
    try {
      const data = await loadData(backend, workspace);
      workspace = data.workspace;
      cards = data.cards;
      now = data.now;
    } catch {
      error = 'workspace.folders_were_imported_but_the_library_could_not_be_refreshed_reopen_the_library_to_see_them';
    }
  }

  async function transferArchive(action: () => Promise<ArchiveResult>) {
    if (saving) return;
    saving = true;
    archiveError = '';
    try {
      await action();
      archiveOperation = null;
      await restoreDialogFocus();
    } catch {
      const message = 'workspace.the_folder_archive_could_not_be_processed_please_try_again';
      if (archiveOperation) archiveError = message;
      else error = message;
    } finally {
      saving = false;
    }
  }

  function exportSingleFolder(opener: HTMLElement, folderId: string) {
    rememberOpener(opener);
    void transferArchive(() => exportFolders(backend, [folderId]));
  }

  function exportSelectedFolders(ids: string[]) {
    void transferArchive(() => exportFolders(backend, ids));
  }

  function importArchivedFolders(mode: ImportMode) {
    void transferArchive(() => importFolders(backend, mode, refreshImportedFolders));
  }

  async function refreshCards() {
    try {
      cards = (await backend.ListCards()) ?? [];
      now = backend.now();
    } catch {
      /* Preserve the last snapshot; loadWorkspace reports initial errors. */
    }
  }
  async function showReviews() {
    if (reviewOpen) return;
    await refreshCards();
    await openReview([...dueCards]);
  }
  function refreshOnFocus() {
    void refreshCards();
    void backend.RefreshReminders();
  }
  onMount(() => {
    const unsubscribe = backend.events.onSchedulesChanged(() => {
      void refreshCards();
    });
    const unsubscribeReview = backend.events.onShowReviews(() => {
      void showReviews();
    });
    void loadWorkspace();
    const timer = window.setInterval(() => {
      now = backend.now();
    }, 1000);
    return () => {
      window.clearInterval(timer);
      unsubscribe();
      unsubscribeReview();
    };
  });

  function restoreWorkspace(): Workspace {
    try {
      return {
        ...emptyWorkspace(),
        ...loadPreferences(backend.preferences, backend.configuration.preferencesStorageKey),
      };
    } catch {
      preferencesError = 'workspace.your_navigation_preferences_could_not_be_loaded';
      return emptyWorkspace();
    }
  }

  async function loadWorkspace() {
    status = 'loading';
    error = '';
    try {
      const loaded = await loadData(backend, workspace);
      workspace = loaded.workspace;
      cards = loaded.cards;
      now = loaded.now;
      status = 'ready';
    } catch {
      status = 'error';
      error = 'workspace.your_saved_memories_could_not_be_loaded_try_again_to_reconnect_to_your_local_database';
    }
  }

  function persistPreferences() {
    try {
      savePreferences(backend.preferences, workspace, backend.configuration.preferencesStorageKey);
      preferencesError = '';
    } catch {
      preferencesError = 'workspace.your_navigation_preferences_could_not_be_saved_on_this_device';
    }
  }

  async function setSidebarOpen(open: boolean) {
    workspace = { ...workspace, sidebarOpen: open };
    persistPreferences();
    await tick();
    sidebarToggle?.focus();
  }

  async function focusSearch() {
    await setSidebarOpen(true);
    searchInput?.focus();
    searchInput?.select();
  }

  function handleShortcut(event: KeyboardEvent) {
    if (
      folderDialog?.open ||
      cardDialog?.open ||
      partDialog?.open ||
      partDeleteDialog?.open ||
      deleteDialog?.open ||
      reviewDialog?.open ||
      archiveOperation !== null ||
      !(event.metaKey || event.ctrlKey) ||
      event.key.toLowerCase() !== 'k'
    )
      return;
    event.preventDefault();
    void focusSearch();
  }

  function navigate(folderId: string | null = null) {
    workspace = { ...workspace, selectedFolderId: folderId };
    persistPreferences();
    if (window.matchMedia('(max-width: 600px)').matches) void setSidebarOpen(false);
  }

  function folderName(id: string) {
    return workspace.folders.find((folder) => folder.id === id)?.title ?? $l.t('workspace.folder');
  }
  function cardsInFolder(id: string) {
    return cards.filter((card) => card.folderId === id);
  }
  function rememberOpener(source: MouseEvent | HTMLElement) {
    dialogOpener = source instanceof HTMLElement ? source : (source.currentTarget as HTMLElement);
    const tile = dialogOpener.closest('li');
    dialogFallback =
      tile?.nextElementSibling?.querySelector<HTMLElement>('button') ??
      tile?.previousElementSibling?.querySelector<HTMLElement>('button') ??
      null;
    formError = '';
  }
  async function restoreDialogFocus() {
    await tick();
    const target = dialogOpener?.isConnected
      ? dialogOpener
      : dialogFallback?.isConnected
        ? dialogFallback
        : document.querySelector<HTMLElement>(
            '.folder-grid button, .memory-grid button, .library-heading .primary-button',
          );
    (target ?? sidebarToggle)?.focus();
  }

  function openFolder(event: MouseEvent) {
    if (status !== 'ready' || saving) return;
    rememberOpener(event);
    editingFolder = null;
    title = '';
    description = '';
    tags = '';
    folderDialog?.showModal();
    titleInput?.focus();
  }

  function editFolder(opener: HTMLElement, folder: Folder) {
    if (saving) return;
    rememberOpener(opener);
    editingFolder = folder;
    title = folder.title;
    description = folder.description;
    tags = folder.tags.join(', ');
    folderDialog?.showModal();
    titleInput?.focus();
  }

  async function saveFolder(event: SubmitEvent) {
    event.preventDefault();
    if (saving) return;
    if (!title.trim()) {
      formError = 'workspace.give_your_folder_a_name';
      titleInput?.focus();
      return;
    }
    saving = true;
    formError = '';
    try {
      const input = { title, description, tags: parseTags(tags) };
      const folder = editingFolder
        ? await backend.UpdateFolder({ ...input, id: editingFolder.id })
        : await backend.CreateFolder(input);
      workspace = reconcileFolders(
        editingFolder ? workspace : { ...workspace, selectedFolderId: folder.id },
        editingFolder
          ? workspace.folders.map((saved) => (saved.id === folder.id ? folder : saved))
          : [...workspace.folders, folder],
      );
      persistPreferences();
      folderDialog?.close();
    } catch {
      formError = 'workspace.your_folder_could_not_be_saved_your_entries_are_still_here_try_again';
    } finally {
      saving = false;
    }
  }

  function openCard(event: MouseEvent) {
    if (status !== 'ready' || saving) return;
    rememberOpener(event);
    editingCard = null;
    editorItems = [textDraft('', backend.newKey())];
    cardFolder = selectedFolder?.id ?? workspace.folders[0]?.id ?? '__new';
    newFolderTitle = workspace.folders.length ? '' : $l.t('workspace.everyday_learning');
    cardDialog?.showModal();
    void tick().then(() => cardDialog?.querySelector('textarea')?.focus());
  }

  function editCard(event: MouseEvent | HTMLElement, card: Card) {
    if (saving || selectingFiles) return;
    rememberOpener(event);
    editingCard = card;
    editorItems = editItems(card, backend.mediaURL);
    cardFolder = card.folderId;
    cardDialog?.showModal();
    void tick().then(() => cardDialog?.querySelector('textarea')?.focus());
  }

  function confirmDelete(event: MouseEvent | HTMLElement, folderId: string, card?: Card) {
    if (saving) return;
    rememberOpener(event);
    deletion = { folderId, cardId: card?.id, title: card ? cardPreview(card) : folderName(folderId) };
    deleteError = '';
    deleteDialog?.showModal();
  }

  function openPart(card: Card, itemId: string, opener: HTMLElement) {
    if (saving) return;
    rememberOpener(opener);
    selectedPart = { card, itemId };
    partError = '';
    partDialog?.showModal();
  }

  async function removePart() {
    if (!selectedPart || saving) return;
    const { card, itemId } = selectedPart;
    saving = true;
    partError = '';
    try {
      const saved = await removeCardPart(backend, card, itemId);
      if (saved) {
        cards = mergeCardSnapshot(cards, saved);
      } else {
        cards = cards.filter((candidate) => candidate.folderId !== card.folderId || candidate.id !== card.id);
      }
      now = backend.now();
      partDeleteDialog?.close();
      partDialog?.close();
    } catch {
      partError = 'workspace.this_part_could_not_be_removed_please_try_again';
    } finally {
      saving = false;
    }
  }

  async function removeSelection(event: SubmitEvent) {
    event.preventDefault();
    if (!deletion || saving) return;
    saving = true;
    deleteError = '';
    const target = deletion;
    try {
      await deleteSelection(backend, target);
      cards = cards.filter(
        (card) => card.folderId !== target.folderId || (!!target.cardId && card.id !== target.cardId),
      );
      if (!target.cardId) {
        workspace = reconcileFolders(
          workspace,
          workspace.folders.filter((folder) => folder.id !== target.folderId),
        );
        persistPreferences();
      }
      now = backend.now();
      deleteDialog?.close();
    } catch {
      deleteError = 'workspace.could_not_delete_this_item_please_try_again';
    } finally {
      saving = false;
    }
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
      const card = await saveMemory(
        backend,
        { folderId: cardFolder, newFolderTitle, editing: editingCard, items },
        (folder) => {
          workspace = reconcileFolders(workspace, [...workspace.folders, folder]);
          cardFolder = folder.id;
        },
      );
      cards = editingCard
        ? cards.map((saved) => (saved.folderId === card.folderId && saved.id === card.id ? card : saved))
        : mergeCardSnapshot(cards, card);
      now = backend.now();
      if (selectedFolder?.id !== card.folderId) navigate(card.folderId);
      cardDialog?.close();
    } catch {
      formError = 'workspace.your_card_could_not_be_saved_your_entries_are_still_here_try_again';
    } finally {
      saving = false;
    }
  }

  function startReview(event: MouseEvent | HTMLElement, card?: Card) {
    if (reviewOpen) return;
    rememberOpener(event);
    void openReview(card ? reviewItems([card]) : [...dueCards]);
  }

  function startFolderReview(opener: HTMLElement, folderId: string) {
    if (reviewOpen) return;
    rememberOpener(opener);
    void openReview(shuffledReviewItems(cardsInFolder(folderId)));
  }

  async function openReview(entries: ReviewItem[]) {
    if (!entries.length || reviewOpen) return;
    reviewQueue = entries;
    reviewOpen = true;
    await tick();
    reviewDialog?.showModal();
  }

  function reviewSaved(updated: Card) {
    cards = cards.map((card) => (card.id === updated.id && card.folderId === updated.folderId ? updated : card));
    now = backend.now();
  }

  function closeReview() {
    reviewOpen = false;
    reviewQueue = [];
    void restoreDialogFocus();
  }
</script>

<svelte:window onkeydown={handleShortcut} onfocus={refreshOnFocus} />

{#snippet icon(name: string, size = 18)}
  <svg
    class:directional-icon={name === 'arrow'}
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.7"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    {#if name === 'folder'}<path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z" />
    {:else if name === 'cards'}<rect x="6" y="7" width="14" height="14" rx="2" /><path
        d="M16 3H4a1 1 0 0 0-1 1v12m7-4h6m-6 4h4"
      />
    {:else if name === 'search'}<circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 4.5 4.5" />
    {:else if name === 'sidebar'}<rect x="3" y="4" width="18" height="16" rx="2" /><path d="M9 4v16" />
    {:else if name === 'plus'}<path d="M12 5v14M5 12h14" />
    {:else if name === 'chevron'}<path d="m9 5 7 7-7 7" />
    {:else if name === 'arrow'}<path d="M4 12h16m-6-6 6 6-6 6" />
    {:else if name === 'import'}<path d="M12 3v12m-4-4 4 4 4-4M4 15v5h16v-5" />
    {:else if name === 'export'}<path d="M12 15V3m-4 4 4-4 4 4M4 15v5h16v-5" />
    {:else if name === 'check'}<path d="m5 12 4 4L19 6" />
    {:else if name === 'clock'}<circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" />
    {:else if name === 'close'}<path d="m6 6 12 12M18 6 6 18" />
    {:else if name === 'trash'}<path d="M3 6h18M9 6V4h6v2M5 6l1 14h12l1-14M10 10v6M14 10v6" />
    {:else if name === 'repeat'}<path d="M20 7h-9a7 7 0 1 0 7 10M16 3l4 4-4 4" />{/if}
  </svg>
{/snippet}

{#snippet workspaceSkeleton(cardsView: boolean)}
  <div class="library-heading" aria-hidden="true">
    <div class="skeleton-heading-copy">
      <Skeleton width="80px" height="9px" />
      <Skeleton width="min(330px, 70vw)" height="46px" />
      <Skeleton width="min(400px, 65vw)" height="14px" />
    </div>
    <div class="heading-actions"><Skeleton width="125px" height="36px" radius="7px" /></div>
  </div>
  {#if cardsView}<div class="section-heading" aria-hidden="true">
      <Skeleton width="min(130px, 45%)" height="18px" /><Skeleton width="min(170px, 40%)" height="9px" />
    </div>{/if}
  <div class="folder-controls" aria-hidden="true">
    <div class="folder-toolbar">
      <Skeleton width="76px" height="32px" radius="6px" />
      <Skeleton width={cardsView ? '115px' : '76px'} height="32px" radius="6px" />
      {#if cardsView}<Skeleton width="100px" height="32px" radius="6px" />{/if}
    </div>
    <Skeleton width="64px" height="10px" />
  </div>
  <ul class:memory-grid={cardsView} class:folder-grid={!cardsView} aria-hidden="true">
    {#each [0, 1, 2, 3, 4, 5] as index (index)}
      <li class="skeleton-tile" class:skeleton-memory={cardsView}>
        {#if cardsView}
          <div class="skeleton-progress"><Skeleton width="30px" height="11px" /></div>
          <div class="skeleton-preview"><Skeleton height="100%" radius="5px" /></div>
        {:else}
          <div class="skeleton-folder-figure"><Skeleton height="100%" radius="9px" /></div>
          <div class="skeleton-folder-copy">
            <Skeleton width={index % 2 ? '55%' : '70%'} height="16px" />
            <Skeleton width="90%" height="11px" />
            <Skeleton width="65%" height="11px" />
            <div class="skeleton-folder-count"><Skeleton width="50px" height="9px" /></div>
          </div>
        {/if}
      </li>
    {/each}
  </ul>
{/snippet}

<main class="app-shell">
  <button
    class="sidebar-backdrop"
    class:visible={workspace.sidebarOpen}
    aria-label={$l.t('workspace.collapse_sidebar')}
    aria-hidden={!workspace.sidebarOpen}
    disabled={!workspace.sidebarOpen}
    tabindex="-1"
    onclick={() => setSidebarOpen(false)}
  ></button>
  <aside id="sidebar" class:collapsed={!workspace.sidebarOpen} aria-label={$l.t('workspace.main_navigation')}>
    <header class="sidebar-header">
      <button class="wordmark" onclick={() => navigate()} aria-label={$l.t('workspace.raven_home')}
        ><span class="brand-icon"><img src="/raven.svg" alt="" /></span><span class="sidebar-label"
          >raven<span class="brand-dot">.</span></span
        ></button
      ><button
        class="icon-button sidebar-toggle"
        bind:this={sidebarToggle}
        aria-label={workspace.sidebarOpen ? $l.t('workspace.collapse_sidebar') : $l.t('workspace.expand_sidebar')}
        title={workspace.sidebarOpen ? $l.t('workspace.collapse_sidebar') : $l.t('workspace.expand_sidebar')}
        aria-expanded={workspace.sidebarOpen}
        aria-controls="sidebar"
        onclick={() => setSidebarOpen(!workspace.sidebarOpen)}>{@render icon('sidebar', 17)}</button
      >
    </header>
    {#if workspace.sidebarOpen}<div class="sidebar-search">
        {@render icon('search', 16)}<input
          dir="auto"
          type="search"
          aria-label={$l.t('workspace.search_cards_and_folders')}
          placeholder={$l.t('workspace.find_a_memory')}
          bind:this={searchInput}
          bind:value={search}
          onkeydown={(event) => {
            if (event.key === 'Escape') search = '';
          }}
        />{#if !search}<kbd dir="ltr">{shortcut}</kbd>{/if}
      </div>
    {:else}<button
        class="icon-button collapsed-search"
        aria-label={$l.t('workspace.search_cards_and_folders')}
        title={$l.t('workspace.searchShortcut', { shortcut })}
        onclick={focusSearch}>{@render icon('search')}</button
      >{/if}
    <nav aria-label={$l.t('workspace.workspace')}>
      <p class="nav-caption sidebar-label">{$l.t('workspace.your_space')}</p>
      <div class="folder-nav-row">
        {#if workspace.sidebarOpen}<button
            class="icon-button folder-disclosure"
            class:expanded={foldersOpen || !!query}
            aria-label={foldersOpen || query ? $l.t('workspace.collapse_folders') : $l.t('workspace.expand_folders')}
            aria-expanded={foldersOpen || !!query}
            aria-controls="folder-navigation"
            onclick={() => {
              foldersOpen = !(foldersOpen || !!query);
              if (query) search = '';
            }}>{@render icon('chevron', 13)}</button
          >{/if}
        <button
          class="nav-item all-folders"
          aria-current={!workspace.selectedFolderId ? 'page' : undefined}
          aria-label={$l.t('workspace.all_folders')}
          title={$l.t('workspace.all_folders')}
          onclick={() => navigate()}
          >{#if workspace.sidebarOpen}<span class="sidebar-label">{$l.t('workspace.all_folders')}</span>
            {#if status === 'ready'}<span class="nav-total sidebar-label">{$l.number(workspace.folders.length)}</span
              >{/if}
          {:else}{@render icon('folder', 17)}{/if}</button
        >
        {#if workspace.sidebarOpen}<button
            class="icon-button folder-create"
            aria-label={$l.t('workspace.new_folder')}
            title={$l.t('workspace.new_folder')}
            disabled={status !== 'ready'}
            onclick={openFolder}>{@render icon('plus', 16)}</button
          >{/if}
      </div>
      {#if workspace.sidebarOpen && (foldersOpen || query)}<div
          id="folder-navigation"
          class="folder-navigation"
          aria-busy={status === 'loading'}
        >
          {#if status === 'loading'}
            {#each [0, 1, 2, 3] as index (index)}<div class="sidebar-skeleton" aria-hidden="true">
                <Skeleton width="15px" height="15px" radius="3px" />
                <Skeleton width={index % 2 ? '55%' : '70%'} height="11px" />
              </div>{/each}
          {:else if status === 'ready'}
            {#each visibleFolders as folder (folder.id)}<button
                class="nav-item folder-link"
                class:selected={selectedFolder?.id === folder.id}
                aria-current={selectedFolder?.id === folder.id ? 'page' : undefined}
                title={folder.title}
                onclick={() => navigate(folder.id)}
                >{@render icon('folder', 15)}<span class="sidebar-label"><bdi>{folder.title}</bdi></span><span
                  class="nav-total sidebar-label">{$l.number(cardsInFolder(folder.id).length)}</span
                ></button
              >{:else}<p class="sidebar-empty">
                {query ? $l.t('workspace.no_matching_folders') : $l.t('workspace.no_folders_yet_use_to_create_one')}
              </p>{/each}
          {/if}
        </div>{/if}
      {#if !workspace.sidebarOpen}<button
          class="nav-item"
          title={$l.t('workspace.new_folder')}
          aria-label={$l.t('workspace.new_folder')}
          disabled={status !== 'ready'}
          onclick={openFolder}>{@render icon('plus', 16)}</button
        >{/if}
    </nav>
    <footer class="sidebar-footer">
      <div class="sidebar-transfers" role="group" aria-label={$l.t('workspace.import_and_export_folders')}>
        <button
          class="footer-action"
          type="button"
          aria-label={$l.t('workspace.import_folders')}
          data-tooltip={$l.t('workspace.import_folders')}
          disabled={saving || status !== 'ready'}
          onclick={(event) => openArchive(event, 'import')}>{@render icon('import', 18)}</button
        >
        <button
          class="footer-action"
          type="button"
          aria-label={$l.t('workspace.export_folders')}
          data-tooltip={$l.t('workspace.export_folders')}
          disabled={saving || status !== 'ready' || !workspace.folders.length}
          onclick={(event) => openArchive(event, 'export')}>{@render icon('export', 18)}</button
        >
      </div>
      <div class="footer-language"><LanguageSelector compact /></div>
    </footer>
  </aside>
  <section class="content" aria-label={selectedFolder?.title ?? $l.t('workspace.folders')}>
    <p class="visually-hidden" role="status">{status === 'loading' ? $l.t('workspace.gathering_your_memories') : ''}</p>
    <div class="page-container" aria-busy={status === 'loading'}>
      {#if error || preferencesError || viewPreferencesError}<p class="error" role="alert">
          {error || preferencesError || viewPreferencesError}
        </p>{/if}
      {#if status === 'loading'}
        {@render workspaceSkeleton(workspace.selectedFolderId !== null || !!query)}
      {:else if status === 'error'}<div class="empty-state">
          <div class="empty-icon">{@render icon('cards', 26)}</div>
          <h1>{$l.t('workspace.let_s_reconnect')}</h1>
          <p>{$l.t('workspace.your_memories_are_saved_in_your_local_database')}</p>
          <button class="primary-button" onclick={loadWorkspace}
            >{$l.t('workspace.try_again')} {@render icon('repeat', 16)}</button
          >
        </div>
      {:else}
        {#if !selectedFolder && !query}
          <section class="library-heading">
            <div>
              <p class="eyebrow">{$l.t('workspace.your_space')}</p>
              <h1>{$l.t('workspace.your_folders')}</h1>
              <p>{$l.t('workspace.a_home_for_the_things_you_want_to_remember')}</p>
            </div>
            <div class="heading-actions">
              <button class="primary-button" disabled={saving} onclick={openFolder}
                >{@render icon('plus', 17)} {$l.t('workspace.new_folder')}</button
              >
            </div>
          </section>
          <div class="folder-controls" aria-label={$l.t('workspace.folder_controls')}>
            <div class="folder-toolbar">
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
                  folderTags = folderTags.includes(tag)
                    ? folderTags.filter((value) => value !== tag)
                    : [...folderTags, tag];
                }}
                onclear={() => {
                  folderTags = [];
                }}
              />
            </div>
            <span class="folder-result-count" role="status"
              >{$l.t('common.folders', { count: filteredFolders.length })}{folderTags.length
                ? $l.t('common.ofTotal', { count: workspace.folders.length })
                : ''}</span
            >
          </div>
          {#if folderTags.length}<div class="active-folder-tags" aria-label={$l.t('workspace.selected_tag_filters')}>
              {#each folderTags as tag (tag)}<button
                  type="button"
                  class="filter-chip"
                  aria-label={$l.t('workspace.removeTag', { name: tag })}
                  onclick={() => {
                    folderTags = folderTags.filter((value) => value !== tag);
                  }}
                >
                  <span><bdi>{tag}</bdi></span>{@render icon('close', 12)}
                </button>{/each}
              <button
                class="clear-folder-tags"
                type="button"
                onclick={() => {
                  folderTags = [];
                }}>{$l.t('workspace.clear_all')}</button
              >
            </div>{/if}
          {#if folderTags.length && !filteredFolders.length}<p class="folder-filter-empty">
              {$l.t('workspace.no_folders_match_all_selected_tags')}
            </p>{/if}
          <ul class="folder-grid" aria-label={$l.t('workspace.your_folders')}>
            {#each filteredFolders as folder (folder.id)}
              <li class="folder-tile">
                <FolderTile papers={selectFolderPapers(cards, folder.id)}>
                  {#snippet children(illustration)}
                    <button class="folder-card" onclick={() => navigate(folder.id)}>
                      {@render illustration()}
                      <h3><bdi>{folder.title}</bdi></h3>
                      <p dir="auto">
                        {folder.description || $l.t('workspace.a_collection_of_things_worth_remembering')}
                      </p>
                      <span class="folder-card-bottom">
                        {#if folder.tags.length}<span class="folder-tags"
                            >{#each folder.tags as tag (tag)}<span class="folder-tag"><bdi>{tag}</bdi></span
                              >{/each}</span
                          >{/if}
                        <span>{$l.t('common.cards', { count: cardsInFolder(folder.id).length })}</span>
                        {#if scheduledItems.some((entry) => entry.card.folderId === folder.id && entry.dueAt <= now)}<span
                            class="due-dot">{$l.t('workspace.ready_to_review')}</span
                          >{/if}
                      </span>
                    </button>
                    <span class="tile-actions"
                      ><ActionMenu
                        label={$l.t('workspace.folderOptions', { name: folder.title })}
                        disabled={saving}
                        actions={[
                          ...(scheduledItems.some((entry) => entry.card.folderId === folder.id)
                            ? [
                                {
                                  label: $l.t('workspace.full_review'),
                                  onselect: (opener: HTMLButtonElement) => startFolderReview(opener, folder.id),
                                },
                              ]
                            : []),
                          {
                            label: $l.t('workspace.export_folder'),
                            onselect: (opener) => exportSingleFolder(opener, folder.id),
                          },
                          { label: $l.t('workspace.edit_folder'), onselect: (opener) => editFolder(opener, folder) },
                          {
                            label: $l.t('workspace.delete_folder'),
                            onselect: (opener) => confirmDelete(opener, folder.id),
                          },
                        ]}
                      /></span
                    >
                  {/snippet}
                </FolderTile>
              </li>
            {/each}
            <li class="new-folder-tile">
              <button class="new-folder-card" onclick={openFolder}
                ><span>{@render icon('plus', 22)}</span><strong>{$l.t('workspace.a_new_curiosity')}</strong><span
                  >{$l.t('workspace.give_it_a_folder')}</span
                ></button
              >
            </li>
          </ul>
        {:else}
          <section class="library-heading">
            <div>
              <p class="eyebrow">
                {query
                  ? $l.t('workspace.a_little_help_finding_it')
                  : selectedFolder
                    ? $l.t('workspace.your_collection')
                    : $l.t('workspace.things_worth_remembering')}
              </p>
              <h1>
                {query
                  ? $l.t('workspace.search_your_memories')
                  : (selectedFolder?.title ?? $l.t('workspace.search_your_memories'))}
              </h1>
              <p>
                {query
                  ? $l.t('workspace.searchResults', { query: search.trim() })
                  : selectedFolder?.description ||
                    $l.t('workspace.big_ideas_small_details_and_everything_you_want_to_keep')}
              </p>
              {#if selectedFolder?.tags.length}<div class="tags">
                  {#each selectedFolder.tags as tag (tag)}<span><bdi>{tag}</bdi></span>{/each}
                </div>{/if}
            </div>
            <div class="heading-actions">
              <button class="primary-button" onclick={(event) => openCard(event)}
                >{@render icon('plus', 17)} {$l.t('workspace.new_card')}</button
              >{#if selectedFolder}{@const folder = selectedFolder}<ActionMenu
                  label={$l.t('workspace.folderOptions', { name: folder.title })}
                  disabled={saving}
                  actions={[
                    ...(scheduledItems.some((entry) => entry.card.folderId === folder.id)
                      ? [
                          {
                            label: $l.t('workspace.full_review'),
                            onselect: (opener: HTMLButtonElement) => startFolderReview(opener, folder.id),
                          },
                        ]
                      : []),
                    {
                      label: $l.t('workspace.export_folder'),
                      onselect: (opener) => exportSingleFolder(opener, folder.id),
                    },
                    { label: $l.t('workspace.edit_folder'), onselect: (opener) => editFolder(opener, folder) },
                    { label: $l.t('workspace.delete_folder'), onselect: (opener) => confirmDelete(opener, folder.id) },
                  ]}
                />{/if}
            </div>
          </section>
          {#if query && !selectedFolder && visibleFolders.length}<div class="section-heading">
              <h2>{$l.t('workspace.folders')} <span class="count">{$l.number(visibleFolders.length)}</span></h2>
            </div>
            <div class="search-folders">
              {#each visibleFolders as folder (folder.id)}<button
                  class="secondary-button"
                  onclick={() => navigate(folder.id)}
                  >{@render icon('folder', 16)} <bdi>{folder.title}</bdi> {@render icon('arrow', 15)}</button
                >{/each}
            </div>{/if}
          <div class="section-heading">
            <h2>
              {selectedFolder ? $l.t('workspace.memory_cards') : $l.t('workspace.your_cards')}
              <span class="count">{$l.number(visibleCards.length)}</span>
            </h2>
            {#if query}<button
                class="text-button"
                onclick={() => {
                  search = '';
                }}>{$l.t('workspace.clear_search')} {@render icon('close', 14)}</button
              >{:else}<span class="section-note">{$l.t('workspace.select_a_card_to_practice_recall')}</span>{/if}
          </div>
          <div class="folder-controls" aria-label={$l.t('workspace.card_controls')}>
            <div class="folder-toolbar">
              <ChoiceMenu
                label={$l.t('workspace.sort')}
                options={cardSortOptions}
                selected={[cardSort]}
                onselect={(value) => {
                  cardSort = value;
                }}
              />
              <ChoiceMenu
                label={$l.t('workspace.content_type')}
                options={cardKindOptions}
                selected={cardKinds}
                multiple
                hint={$l.t('workspace.cards_match_any_selected_content_type_including_hints')}
                onselect={(kind) => {
                  cardKinds = cardKinds.includes(kind)
                    ? cardKinds.filter((value) => value !== kind)
                    : [...cardKinds, kind];
                }}
                onclear={() => {
                  cardKinds = [];
                }}
              />
              <RecallFilter
                range={cardProgressRange}
                onchange={(range) => {
                  cardProgressRange = range;
                }}
              />
            </div>
            <span class="folder-result-count" role="status" aria-live="polite">
              {$l.t('common.cardsOfTotal', { count: matchingCards.length, shown: $l.number(visibleCards.length) })}
            </span>
          </div>
          {#if hasCardFilters}<div class="active-folder-tags" aria-label={$l.t('workspace.selected_card_filters')}>
              {#each cardKinds as kind (kind)}<button
                  type="button"
                  class="filter-chip"
                  aria-label={$l.t('workspace.removeKind', {
                    kind: cardKindOptions.find((option) => option.value === kind)?.label ?? '',
                  })}
                  onclick={() => {
                    cardKinds = cardKinds.filter((value) => value !== kind);
                  }}
                >
                  <span>{cardKindOptions.find((option) => option.value === kind)?.label}</span>{@render icon(
                    'close',
                    12,
                  )}
                </button>{/each}
              {#if hasRecallFilter}<button
                  type="button"
                  class="filter-chip"
                  aria-label={$l.t('workspace.removeRecall')}
                  onclick={() => {
                    cardProgressRange = { minimum: 0, maximum: 100 };
                  }}
                >
                  <span
                    >{$l.t('workspace.recallRange', {
                      minimum: $l.percent(cardProgressRange.minimum),
                      maximum: $l.percent(cardProgressRange.maximum),
                    })}</span
                  >{@render icon('close', 12)}
                </button>{/if}
              <button type="button" class="clear-folder-tags" onclick={clearCardFilters}
                >{$l.t('workspace.clear_filters')}</button
              >
            </div>{/if}
          {#if visibleCards.length}<ul class="memory-grid" aria-label={$l.t('workspace.memory_cards')}>
              {#each visibleCards as card (`${card.folderId}/${card.id}`)}
                {@const progress = cardProgress(card)}
                <li class="memory-tile">
                  <div
                    class="memory-card"
                    style:--progress-fill={`hsl(${progress <= 50 ? 0 : progress <= 80 ? (progress - 50) * 2 : 60 + (progress - 80) * 3} 75% 50%)`}
                  >
                    <span
                      class="card-progress-percentage"
                      aria-label={$l.t('workspace.recallSuccess', { percentage: $l.percent(progress) })}
                      title={$l.t('workspace.recallExplanation')}>{$l.percent(progress)}</span
                    >
                    <div class="memory-card-preview">
                      <CardContent
                        {card}
                        mediaURL={backend.mediaURL}
                        nativeStart={backend.previewStart}
                        compact
                        onopen={(itemId, opener) => openPart(card, itemId, opener)}
                      />
                    </div>
                  </div>
                  <span class="tile-actions"
                    ><ActionMenu
                      label={$l.t('workspace.cardOptions', { name: cardPreview(card) })}
                      disabled={saving || selectingFiles}
                      actions={[
                        { label: $l.t('workspace.reviewCard'), onselect: (opener) => startReview(opener, card) },
                        { label: $l.t('workspace.edit_card'), onselect: (opener) => editCard(opener, card) },
                        {
                          label: $l.t('workspace.delete_card'),
                          onselect: (opener) => confirmDelete(opener, card.folderId, card),
                        },
                      ]}
                    /></span
                  >
                </li>
              {/each}
            </ul>
          {:else}<div class="empty-state">
              <div class="empty-icon">{@render icon(query || hasCardFilters ? 'search' : 'cards', 27)}</div>
              <h2>{query || hasCardFilters ? $l.t('workspace.noCardsMatch') : $l.t('workspace.nextMemory')}</h2>
              <p>
                {hasCardFilters
                  ? $l.t('workspace.removeFilterHelp')
                  : query
                    ? $l.t('workspace.searchHelp')
                    : $l.t('workspace.addHelp')}
              </p>
              {#if hasCardFilters}<button class="secondary-button" onclick={clearCardFilters}
                  >{$l.t('workspace.clear_filters')}</button
                >{/if}
              {#if query}<button
                  class="secondary-button"
                  onclick={() => {
                    search = '';
                  }}>{$l.t('workspace.clear_search')}</button
                >{:else if !hasCardFilters}<button class="primary-button" onclick={(event) => openCard(event)}
                  >{@render icon('plus', 16)} {$l.t('workspace.createCard')}</button
                >{/if}
            </div>{/if}
        {/if}
      {/if}
    </div>
  </section>
</main>

<ArchiveDialog
  operation={archiveOperation}
  folders={workspace.folders}
  busy={saving}
  error={archiveError ? $l.t(archiveError) : ''}
  onexport={exportSelectedFolders}
  onimport={importArchivedFolders}
  onclose={() => {
    archiveOperation = null;
    void restoreDialogFocus();
  }}
/>

<dialog
  class="card-dialog part-dialog"
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
    <div class="part-toolbar" aria-busy={saving}>
      <button
        type="button"
        class="icon-button"
        disabled={saving}
        onclick={() => {
          partError = '';
          partDeleteDialog?.showModal();
        }}
        aria-label={saving ? $l.t('workspace.removing') : $l.t('workspace.deletePart')}
        title={$l.t('workspace.deletePart')}>{@render icon('trash')}</button
      >
      <button
        type="button"
        class="icon-button"
        disabled={saving}
        onclick={() => partDialog?.close()}
        aria-label={$l.t('cards.cancel')}
        title={$l.t('cards.cancel')}>{@render icon('close')}</button
      >
    </div>
    <div class="part-body">
      <CardContent
        card={{
          ...selectedPart.card,
          items: selectedPart.card.items.filter((item) => item.id === selectedPart?.itemId),
        }}
        mediaURL={backend.mediaURL}
      />
      {#if selectedPart.card.items.length === 1}
        <p class="field-hint">{$l.t('workspace.lastPartHelp')}</p>
      {/if}
      {#if partError}<p class="error" role="alert">{partError ? $l.t(partError) : ''}</p>{/if}
    </div>
  {/if}
</dialog>

<dialog
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
    <h2 id="part-delete-title">
      {$l.t('workspace.deleteTitle', {
        kind: $l.t(selectedPart?.card.items.length === 1 ? 'cards.card' : 'cards.part'),
      })}
    </h2>
    <p id="part-delete-description" class="dialog-description">
      {selectedPart?.card.items.length === 1 ? $l.t('workspace.lastPartDelete') : $l.t('workspace.partDelete')}
      {$l.t('workspace.this_cannot_be_undone')}
    </p>
    {#if partError}<p class="error" role="alert">{partError ? $l.t(partError) : ''}</p>{/if}
    <div class="dialog-actions">
      <button type="button" class="secondary-button" disabled={saving} onclick={() => partDeleteDialog?.close()}
        >{$l.t('cards.cancel')}</button
      >
      <button type="submit" class="primary-button" disabled={saving}
        >{saving ? $l.t('workspace.deleting') : $l.t('workspace.delete_permanently')}</button
      >
    </div>
  </form>
</dialog>

<dialog
  bind:this={folderDialog}
  aria-labelledby="folder-dialog-title"
  onclose={restoreDialogFocus}
  oncancel={(event) => {
    if (saving) event.preventDefault();
  }}
>
  <form onsubmit={saveFolder} aria-busy={saving} novalidate>
    <div class="dialog-top">
      <span class="empty-icon">{@render icon('folder', 24)}</span><button
        type="button"
        class="icon-button"
        aria-label={editingFolder ? $l.t('workspace.closeEditFolder') : $l.t('workspace.closeNewFolder')}
        disabled={saving}
        onclick={() => folderDialog?.close()}>{@render icon('close')}</button
      >
    </div>
    <p class="eyebrow">{editingFolder ? $l.t('workspace.your_collection') : 'MAKE ROOM FOR A CURIOSITY'}</p>
    <h2 id="folder-dialog-title">{editingFolder ? 'Edit folder info.' : 'A home for related memories.'}</h2>
    <p class="dialog-description">{$l.t('workspace.topicHelp')}</p>
    <label for="folder-title">{$l.t('workspace.folder_name')} <span>{$l.t('workspace.required')}</span></label><input
      dir="auto"
      id="folder-title"
      bind:this={titleInput}
      bind:value={title}
      placeholder={$l.t('workspace.e_g_everyday_spanish')}
      disabled={saving}
      required
      aria-invalid={!!formError && !title.trim()}
    />
    <label for="folder-description"
      >{$l.t('workspace.a_little_context')} <span>{$l.t('workspace.optional')}</span></label
    ><textarea
      dir="auto"
      id="folder-description"
      bind:value={description}
      rows="2"
      placeholder={$l.t('workspace.what_would_you_like_to_remember')}
      disabled={saving}></textarea>
    <label for="folder-tags">{$l.t('workspace.tags')} <span>{$l.t('workspace.optional')}</span></label><input
      dir="auto"
      id="folder-tags"
      bind:value={tags}
      placeholder={$l.t('workspace.language_travel_learning')}
      aria-describedby="tags-hint"
      disabled={saving}
    />
    <p class="field-hint" id="tags-hint">{$l.t('workspace.separate_tags_with_commas')}</p>
    {#if formError}<p class="error" role="alert">{formError ? $l.t(formError) : ''}</p>{/if}
    <div class="dialog-actions">
      <button type="button" class="secondary-button" disabled={saving} onclick={() => folderDialog?.close()}
        >{$l.t('cards.cancel')}</button
      ><button type="submit" class="primary-button" disabled={saving}
        >{saving
          ? $l.t('workspace.saving')
          : editingFolder
            ? $l.t('workspace.save_changes')
            : $l.t('workspace.create_folder')}
        {@render icon('arrow', 16)}</button
      >
    </div>
  </form>
</dialog>

<dialog
  bind:this={deleteDialog}
  aria-labelledby="delete-title"
  onclose={restoreDialogFocus}
  oncancel={(event) => {
    if (saving) event.preventDefault();
  }}
>
  <form onsubmit={removeSelection} aria-busy={saving}>
    <h2 id="delete-title">
      {$l.t('workspace.deleteTitle', { kind: $l.t(deletion?.cardId ? 'cards.card' : 'cards.folder') })}
    </h2>
    <p class="dialog-description">
      {$l.t('workspace.deleteNamed', { name: deletion?.title ?? '' })}
      {deletion?.cardId
        ? $l.t('workspace.its_content_and_review_history_will_be_removed')
        : $l.t('workspace.folderDeleted')}
      {$l.t('workspace.this_cannot_be_undone')}
    </p>
    {#if deleteError}<p class="error" role="alert">{deleteError ? $l.t(deleteError) : ''}</p>{/if}
    <div class="dialog-actions">
      <button type="button" class="secondary-button" disabled={saving} onclick={() => deleteDialog?.close()}
        >{$l.t('cards.cancel')}</button
      ><button type="submit" class="primary-button" disabled={saving}
        >{saving ? $l.t('workspace.deleting') : $l.t('workspace.delete_permanently')}</button
      >
    </div>
  </form>
</dialog>

<dialog
  class="card-dialog"
  bind:this={cardDialog}
  aria-labelledby="card-dialog-title"
  onclose={restoreDialogFocus}
  oncancel={(event) => {
    if (saving || selectingFiles) event.preventDefault();
  }}
>
  <form onsubmit={createCard} aria-busy={saving || selectingFiles} novalidate>
    <div class="dialog-top">
      <span class="empty-icon">{@render icon('cards', 24)}</span><button
        type="button"
        class="icon-button"
        aria-label={$l.t('workspace.close_card_editor')}
        disabled={saving || selectingFiles}
        onclick={() => cardDialog?.close()}>{@render icon('close')}</button
      >
    </div>
    <p class="eyebrow">{$l.t('workspace.a_note_to_your_future_self')}</p>
    <h2 id="card-dialog-title">
      {editingCard ? $l.t('workspace.edit_your_memory') : $l.t('workspace.what_s_worth_remembering')}
    </h2>
    <p class="dialog-description">{$l.t('workspace.keep_information_together_with_text_images_audio_and_video')}</p>
    <CardEditor
      media={backend}
      bind:items={editorItems}
      bind:selecting={selectingFiles}
      disabled={saving}
      onerror={(message) => (formError = message)}
    />
    {#if cardFolder === '__new'}<label for="card-new-folder">{$l.t('workspace.new_folder_name')}</label><input
        dir="auto"
        id="card-new-folder"
        bind:value={newFolderTitle}
        placeholder={$l.t('workspace.e_g_everyday_spanish')}
        disabled={saving}
        required
        aria-invalid={!!formError && !newFolderTitle.trim()}
      />{/if}
    <p class="save-note">
      {@render icon('clock', 15)}
      {editingCard
        ? $l.t('workspace.your_review_schedule_stays_the_same')
        : $l.t('workspace.ready_for_your_first_review_as_soon_as_you_save')}
    </p>
    {#if formError}<p class="error" role="alert">{formError ? $l.t(formError) : ''}</p>{/if}
    <div class="dialog-actions">
      <button
        type="button"
        class="secondary-button"
        disabled={saving || selectingFiles}
        onclick={() => cardDialog?.close()}>{$l.t('cards.cancel')}</button
      ><button type="submit" class="primary-button" disabled={saving || selectingFiles}
        >{saving
          ? $l.t('workspace.saving')
          : editingCard
            ? $l.t('workspace.save_changes')
            : $l.t('workspace.save_memory')}
        {@render icon('arrow', 16)}</button
      >
    </div>
  </form>
</dialog>

<dialog
  class="review-dialog"
  bind:this={reviewDialog}
  aria-label={$l.t('workspace.review')}
  onclose={closeReview}
  oncancel={(event) => {
    if (reviewSaving) event.preventDefault();
  }}
>
  {#if reviewOpen}<ReviewSession
      review={backend}
      mediaURL={backend.mediaURL}
      entries={reviewQueue}
      nativeStart={backend.previewStart}
      folders={workspace.folders}
      bind:saving={reviewSaving}
      onsaved={reviewSaved}
      onclose={() => reviewDialog?.close()}
    />{/if}
</dialog>

<style>
  .folder-controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding-bottom: 19px;
  }
  .folder-toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    min-width: 0;
    max-width: 100%;
  }
  .folder-result-count {
    color: var(--muted);
    font-size: 10px;
    white-space: nowrap;
  }
  .active-folder-tags {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 7px;
    margin: -8px 0 23px;
  }
  .filter-chip {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    max-width: 100%;
    padding: 6px 8px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--selected);
    color: var(--ink);
    font-size: 10px;
  }
  .filter-chip span {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .filter-chip :global(svg) {
    flex-shrink: 0;
    color: var(--muted);
  }
  .filter-chip:hover {
    border-color: var(--line-strong);
    background: var(--hover);
  }
  .clear-folder-tags {
    border: 0;
    padding: 6px 5px;
    background: transparent;
    color: var(--muted);
    font-size: 10px;
  }
  .clear-folder-tags:hover {
    color: var(--ink);
  }
  .filter-chip:focus-visible,
  .clear-folder-tags:focus-visible {
    outline: 1px solid var(--focus);
    outline-offset: 3px;
  }
  .folder-filter-empty {
    color: var(--muted);
    font-size: 12px;
  }

  .app-shell {
    display: flex;
    height: 100svh;
    overflow: hidden;
    background: var(--paper);
  }
  button {
    transition:
      background 0.18s,
      color 0.18s,
      border-color 0.18s,
      transform 0.18s;
  }
  button:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .icon-button {
    display: grid;
    place-items: center;
    flex-shrink: 0;
    width: 30px;
    height: 30px;
    padding: 0;
    border: 0;
    border-radius: 6px;
    color: var(--muted);
    background: transparent;
  }
  .icon-button:hover {
    background: var(--hover);
    color: var(--ink);
  }
  aside {
    --sidebar-motion: 220ms ease;
    display: flex;
    flex-direction: column;
    flex: 0 0 224px;
    width: 224px;
    min-height: 0;
    padding: 28px 17px 20px;
    border-inline-end: 1px solid #ffffff14;
    background: var(--sidebar);
    transition:
      width var(--sidebar-motion),
      flex-basis var(--sidebar-motion),
      padding var(--sidebar-motion);
    -webkit-backdrop-filter: blur(24px) saturate(0.7);
    backdrop-filter: blur(24px) saturate(0.7);
  }
  .sidebar-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 31px;
    min-height: 32px;
  }
  .wordmark {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0;
    border: 0;
    background: none;
    color: var(--ink);
    font-size: 25px;
    font-family: 'Fredericka the Great', Georgia, 'Times New Roman', serif;
    direction: ltr;
    font-weight: 400;
    letter-spacing: 0;
  }
  .brand-icon {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border-radius: 7px;
    overflow: hidden;
    background: transparent;
  }
  .brand-icon img {
    width: 100%;
    height: 100%;
    filter: invert(1);
  }
  .brand-dot {
    color: var(--accent);
  }
  .sidebar-label {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .sidebar-search {
    display: flex;
    align-items: center;
    gap: 7px;
    height: 36px;
    padding: 0 9px;
    margin-bottom: 29px;
    border: 1px solid var(--line);
    border-radius: 7px;
    background: #ffffff08;
    color: var(--muted);
  }
  .sidebar-search:focus-within {
    outline: none;
  }
  .sidebar-search input {
    width: 100%;
    min-width: 0;
    padding: 0;
    border: 0;
    background: transparent;
    font-size: 11px;
    outline: none;
  }
  .sidebar-search input:focus-visible {
    outline: none;
  }
  .sidebar-search svg {
    flex-shrink: 0;
  }
  kbd {
    flex-shrink: 0;
    font: inherit;
    font-size: 9px;
    color: var(--muted);
  }
  nav {
    flex: 1;
    min-height: 0;
    overflow: auto;
    scrollbar-width: thin;
  }
  .nav-caption {
    margin: 0 11px 12px;
    color: var(--muted);
    font-size: 9px;
    font-weight: 600;
    letter-spacing: 1.3px;
  }
  .nav-item {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    min-height: 39px;
    margin: 3px 0;
    padding: 9px 11px;
    border: 1px solid transparent;
    border-radius: 7px;
    background: transparent;
    color: var(--ink);
    font-size: 12px;
    text-align: start;
  }
  .nav-item > svg {
    flex-shrink: 0;
  }
  .nav-item:hover {
    background: var(--hover);
  }
  .nav-item.selected {
    color: var(--ink);
    font-weight: 600;
    background: var(--selected);
    border-color: var(--line) fff1a;
  }
  .sidebar-footer {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    flex-shrink: 0;
    margin-top: 16px;
    padding-top: 12px;
  }
  .sidebar-transfers {
    display: flex;
    gap: 12px;
  }
  .footer-action {
    position: relative;
    display: grid;
    place-items: center;
    width: 40px;
    height: 40px;
    padding: 0;
    border: 1px solid transparent;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
  }
  .footer-action:hover:not(:disabled),
  .footer-action:focus-visible {
    border-color: var(--line-strong);
    background: var(--hover);
    color: var(--ink);
  }
  .footer-action:active:not(:disabled) {
    background: var(--selected);
  }
  .footer-action::after {
    content: attr(data-tooltip);
    position: absolute;
    z-index: 6;
    inset-block-end: calc(100% + 8px);
    inset-inline-start: 0;
    width: max-content;
    max-width: 180px;
    padding: 7px 9px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--paper);
    color: var(--ink);
    font-size: 11px;
    line-height: 1.4;
    text-align: start;
    pointer-events: none;
    visibility: hidden;
  }
  .footer-action:hover::after,
  .footer-action:focus-visible::after {
    visibility: visible;
  }
  .collapsed .sidebar-footer,
  .collapsed .sidebar-transfers {
    flex-direction: column;
  }
  .collapsed .footer-language {
    --language-tip-start: 0;
    --language-tip-end: auto;
  }
  @media (pointer: coarse) {
    .footer-action {
      width: 44px;
      height: 44px;
    }
  }
  .nav-total {
    margin-inline-start: auto;
    font-size: 10px;
  }
  .nav-total {
    color: var(--muted);
  }
  .folder-nav-row {
    display: flex;
    align-items: center;
    gap: 2px;
  }
  .folder-nav-row .icon-button {
    flex-shrink: 0;
    width: 24px;
    height: 30px;
  }
  .folder-disclosure {
    transition: transform 0.15s;
  }
  .folder-disclosure.expanded {
    transform: rotate(90deg);
  }
  .all-folders {
    flex: 1;
    min-width: 0;
    padding-inline-start: 6px;
    padding-inline-end: 6px;
    gap: 8px;
  }
  .all-folders:hover {
    background: transparent;
  }
  .folder-create {
    color: var(--muted);
  }
  .folder-navigation {
    margin: 4px 0 0;
    margin-inline-start: 13px;
    padding-inline-start: 12px;
    border-inline-start: 1px solid var(--line);
  }
  .folder-link {
    font-size: 11px;
    min-height: 36px;
  }
  .sidebar-empty {
    margin: 10px 11px 15px;
    font-size: 10px;
    color: var(--muted);
    line-height: 1.6;
  }
  .sidebar-backdrop {
    display: none;
  }
  .sidebar-backdrop:disabled {
    opacity: 0;
  }
  aside.collapsed {
    width: 64px;
    flex-basis: 64px;
    padding-inline-start: 10px;
    padding-inline-end: 10px;
  }
  .collapsed .sidebar-label {
    display: none;
  }
  .collapsed .sidebar-header {
    flex-direction: column;
    gap: 14px;
    margin-bottom: 15px;
  }
  .collapsed .nav-item {
    justify-content: center;
    padding: 9px;
  }
  .collapsed-search {
    width: 100%;
    margin-bottom: 24px;
  }
  .content,
  dialog {
    --ink: #fff;
    --muted: #a3a3a3;
    --paper: #000;
    --line: #303030;
    --line-strong: #555;
    --hover: #ffffff0d;
    --selected: #ffffff14;
    --focus: #bdbdbd;
  }
  dialog .icon-button:hover {
    color: var(--ink);
  }
  .content {
    flex: 1;
    min-width: 0;
    overflow: auto;
    padding: 0 38px 38px;
  }
  .page-container {
    display: flex;
    flex-direction: column;
    max-width: 1100px;
    min-height: 100%;
    margin: auto;
  }
  .eyebrow {
    margin: 0 0 13px;
    color: var(--ink);
    font-size: 8px;
    font-weight: 600;
    letter-spacing: 1.5px;
    line-height: 1.7;
  }
  h1,
  h2,
  h3,
  p {
    overflow-wrap: anywhere;
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: clamp(32px, 3.9vw, 49px);
    font-weight: 400;
    letter-spacing: 0;
    line-height: 1.13;
  }
  .primary-button,
  .secondary-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 9px;
    padding: 11px 16px;
    border: 1px solid transparent;
    border-radius: 6px;
    font-size: 11px;
    font-weight: 500;
    line-height: 1.5;
    text-decoration: none;
  }
  .primary-button {
    color: #000;
    border-color: var(--line);
    background: #fff;
  }
  .primary-button:hover {
    border-color: var(--line-strong);
    background: #151515;
    color: #fff;
  }
  .secondary-button {
    background: var(--paper);
    border-color: var(--line);
    color: var(--ink);
  }
  .secondary-button:hover {
    border-color: var(--line-strong);
    background: #151515;
    color: #fff;
  }
  .section-heading {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 16px;
    margin: 0 0 17px;
  }
  .section-heading h2 {
    display: flex;
    align-items: center;
    gap: 9px;
    margin: 0;
    font-size: 15px;
    font-weight: 500;
    letter-spacing: -0.3px;
  }
  .count {
    display: inline-grid;
    place-items: center;
    min-width: 20px;
    padding: 2px 6px;
    border-radius: 4px;
    background: var(--paper);
    color: var(--ink);
    font-size: 9px;
    font-weight: 400;
  }
  .text-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    padding: 6px 0;
    border: 0;
    background: none;
    color: var(--ink);
    font-size: 10px;
    line-height: 1.6;
    text-align: start;
  }
  .text-button:hover {
    color: var(--ink);
  }
  .folder-grid,
  .memory-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 207px), 1fr));
    gap: 13px;
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .folder-grid li,
  .memory-grid li {
    min-width: 0;
  }
  .folder-grid {
    row-gap: 22px;
  }
  .folder-tile,
  .new-folder-tile,
  .memory-tile {
    position: relative;
  }
  .new-folder-tile {
    padding-top: 15px;
  }
  .new-folder-tile::before {
    content: '';
    position: absolute;
    top: 0;
    inset-inline-start: 0;
    width: 39%;
    height: 25px;
    border: 1px solid var(--line);
    border-bottom: 0;
    border-radius: 9px 16px 0 0;
    background: var(--paper);
    transition: border-color 0.18s;
  }
  .new-folder-tile::before {
    border-style: dashed;
  }
  .new-folder-tile:hover::before,
  .new-folder-tile:focus-within::before {
    border-color: var(--line-strong);
  }
  .tile-actions {
    position: absolute;
    top: 10px;
    inset-inline-end: 10px;
  }
  .folder-tile .tile-actions {
    top: 10px;
  }
  .memory-tile .tile-actions {
    inset-inline-end: auto;
    inset-inline-start: 10px;
  }
  .new-folder-card {
    width: 100%;
    height: 100%;
    min-height: 179px;
    padding: 18px;
    border: 1px solid var(--line);
    border-radius: 9px;
    background: var(--paper);
    color: var(--ink);
    text-align: start;
  }
  .folder-card,
  .new-folder-card {
    position: relative;
    display: flex;
    flex-direction: column;
    min-height: 190px;
    border-start-start-radius: 0;
  }
  .new-folder-card::before {
    content: '';
    position: absolute;
    top: -1px;
    inset-inline-start: 0;
    width: 39%;
    height: 1px;
    background: var(--paper);
  }
  .folder-tile:hover .folder-card,
  .folder-tile:focus-within .folder-card {
    border-color: var(--line-strong);
  }
  .folder-card {
    position: relative;
    display: flex;
    flex-direction: column;
    width: 100%;
    height: 100%;
    padding: 0 12px 12px;
    border: 1px solid transparent;
    border-radius: 9px;
    background: #000;
    color: var(--ink);
    text-align: start;
  }
  .folder-card:focus-visible {
    outline: 1px solid var(--line-strong);
    outline-offset: 3px;
  }
  .folder-card h3 {
    margin: 0 0 8px;
    padding-inline-end: 40px;
    font-size: 14px;
    font-weight: 550;
    line-height: 1.5;
  }
  .folder-card p {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
    margin: 0 0 22px;
    color: var(--muted);
    font-size: 11px;
    line-height: 1.75;
  }
  .folder-card-bottom {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-top: auto;
    width: 100%;
    color: var(--muted);
    font-size: 9px;
  }
  .due-dot {
    display: flex;
    align-items: center;
    gap: 5px;
    color: var(--ink);
  }
  .due-dot::before {
    content: '';
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: #fff;
  }
  .folder-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    width: 100%;
  }
  .folder-tag {
    max-width: 100%;
    padding: 3px 6px;
    border: 1px solid var(--line);
    border-radius: 4px;
    overflow-wrap: anywhere;
  }
  .new-folder-card {
    display: flex;
    align-items: center;
    justify-content: center;
    flex-direction: column;
    gap: 8px;
    background: transparent;
    border-style: dashed;
    border-color: var(--line);
    text-align: center;
    color: var(--ink);
  }
  .new-folder-card:focus-visible,
  .new-folder-card:hover {
    background: var(--paper);
    border-color: var(--line-strong);
  }
  .new-folder-card > span:first-child {
    display: grid;
    place-items: center;
    width: 33px;
    height: 33px;
    margin-bottom: 5px;
    border: 1px solid var(--line);
    border-radius: 50%;
  }
  .new-folder-card strong {
    color: var(--ink);
    font-size: 10px;
    font-weight: 500;
  }
  .new-folder-card > span:last-child {
    font-size: 9px;
  }
  .empty-icon {
    display: inline-grid;
    place-items: center;
    flex-shrink: 0;
    width: 43px;
    height: 43px;
    border-radius: 11px;
    background: var(--paper);
    color: var(--ink);
  }
  .library-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 25px;
    padding: 38px 0 32px;
  }
  .library-heading h1 {
    font-size: 38px;
    line-height: 1.2;
  }
  .library-heading p:not(.eyebrow) {
    max-width: 580px;
    margin: 13px 0 0;
    color: var(--ink);
    font-size: 11px;
    line-height: 1.8;
    white-space: pre-wrap;
  }
  .library-heading .primary-button {
    flex-shrink: 0;
    margin-top: 20px;
  }
  .heading-actions {
    display: flex;
    align-items: center;
    flex-shrink: 0;
    gap: 10px;
    margin-top: 20px;
  }
  .heading-actions .primary-button {
    margin-top: 0;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 15px;
  }
  .tags span {
    padding: 4px 7px;
    border: 1px solid var(--line);
    border-radius: 4px;
    color: var(--ink);
    font-size: 9px;
  }
  .section-note {
    color: var(--ink);
    font-size: 9px;
  }
  .search-folders {
    display: flex;
    flex-wrap: wrap;
    gap: 9px;
    margin: 0 0 25px;
  }
  .memory-grid {
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 220px), 1fr));
  }
  .memory-card {
    position: relative;
    display: flex;
    flex-direction: column;
    width: 100%;
    aspect-ratio: 4 / 5;
    min-height: 0;
    overflow: hidden;
    padding: 44px 14px 14px;
    border: 1px solid color-mix(in srgb, var(--progress-fill) 35%, transparent);
    border-radius: 9px;
    background: var(--paper);
    color: var(--ink);
    text-align: start;
  }
  .memory-grid li {
    display: flex;
    flex-direction: column;
  }
  .memory-card-preview {
    flex: 1;
    min-height: 0;
    overflow: hidden;
    margin: 0 -10px;
  }
  .card-progress-percentage {
    position: absolute;
    top: 14px;
    inset-inline-end: 14px;
    color: color-mix(in srgb, var(--progress-fill) 70%, transparent);
    font-size: 11px;
    font-variant-numeric: tabular-nums;
    line-height: 18px;
  }
  .card-dialog {
    width: min(620px, calc(100vw - 36px));
  }
  .part-dialog {
    padding: 0;
    overflow: hidden;
  }
  .part-toolbar {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 12px;
    border-bottom: 1px solid var(--line);
    background: #000;
  }
  .part-toolbar .icon-button {
    width: 36px;
    height: 36px;
    border: 1px solid var(--line);
    border-radius: 7px;
  }
  .part-toolbar .icon-button:hover {
    border-color: var(--line-strong);
  }
  .part-toolbar .icon-button:focus-visible {
    outline: 2px solid var(--muted);
    outline-offset: 2px;
  }
  .part-body {
    max-height: calc(100svh - 98px);
    overflow-y: auto;
    padding: 24px;
  }
  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
    border: 0;
  }
  .sidebar-skeleton {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 35px;
    padding-inline: 12px;
  }
  .skeleton-heading-copy {
    display: flex;
    flex-direction: column;
    gap: 16px;
    min-width: 0;
    max-width: 100%;
  }
  .skeleton-tile {
    display: flex;
    flex-direction: column;
    min-height: 190px;
  }
  .skeleton-folder-figure {
    aspect-ratio: 5 / 4;
    margin-inline: 12px;
  }
  .skeleton-folder-copy {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 12px;
  }
  .skeleton-folder-count {
    margin-top: 16px;
  }
  .skeleton-memory {
    aspect-ratio: 4 / 5;
    min-height: 0;
    padding: 14px;
    border: 1px solid var(--line);
    border-radius: 9px;
  }
  .skeleton-progress {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 19px;
  }
  .skeleton-preview {
    flex: 1;
    min-height: 0;
  }
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 55px 20px;
    min-height: 300px;
    text-align: center;
    border: 1px dashed var(--line);
    border-radius: 12px;
  }
  .empty-state h1,
  .empty-state h2 {
    margin: 20px 0 10px;
    font:
      26px/1.3 Georgia,
      serif;
    color: var(--ink);
  }
  .empty-state p {
    margin: 0 0 22px;
    color: var(--ink);
    font-size: 11px;
    line-height: 1.8;
  }
  .error {
    padding: 12px;
    margin: 15px 0;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--paper);
    color: var(--ink);
    font-size: 11px;
    line-height: 1.7;
  }
  dialog {
    width: min(500px, calc(100vw - 36px));
    max-height: calc(100svh - 36px);
    margin: auto;
    padding: 28px;
    overflow-y: auto;
    border: 1px solid var(--line);
    border-radius: 15px;
    background: var(--paper);
    color: var(--ink);
    box-shadow: 0 30px 100px #00000055;
  }
  dialog::backdrop {
    -webkit-backdrop-filter: blur(8px);
    backdrop-filter: blur(8px);
    background: transparent;
  }
  .dialog-top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    margin-bottom: 19px;
  }
  dialog h2 {
    margin: 0 0 10px;
    font:
      28px/1.2 Georgia,
      serif;
    letter-spacing: -0.8px;
  }
  .dialog-description {
    margin: 0 0 22px;
    color: var(--ink);
    font-size: 12px;
    line-height: 1.8;
  }
  label {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin: 19px 0 8px;
    color: var(--ink);
    font-size: 11px;
  }
  label span {
    color: var(--ink);
    font-size: 9px;
  }
  input,
  textarea {
    width: 100%;
    min-width: 0;
    padding: 10px 11px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--paper);
    color: var(--ink);
    font: inherit;
    font-size: 12px;
    line-height: 1.6;
  }
  textarea {
    resize: vertical;
  }
  input::placeholder,
  textarea::placeholder {
    color: var(--ink);
    opacity: 1;
  }
  input:focus-visible,
  textarea:focus-visible {
    outline: none;
  }
  [aria-invalid='true'] {
    border-color: #a3a3a3;
  }
  .field-hint {
    margin: 7px 0 0;
    color: var(--ink);
    font-size: 9px;
  }
  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: 9px;
    margin-top: 25px;
  }
  .save-note {
    display: flex;
    align-items: center;
    gap: 7px;
    margin: 18px 0 0;
    font-size: 9px;
    color: var(--ink);
  }
  .save-note > svg {
    flex-shrink: 0;
  }
  .review-dialog {
    box-sizing: border-box;
    width: min(360px, calc((100svh - 36px) / 1.7), calc(100vw - 36px));
    height: min(612px, calc(100svh - 36px), calc((100vw - 36px) * 1.7));
    aspect-ratio: 1 / 1.7;
    padding: 0;
  }
  @media (min-width: 1250px) {
    .content {
      padding: 0 55px 38px;
    }
    .folder-grid {
      grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    }
  }
  @media (max-width: 850px) {
    aside {
      flex-basis: 200px;
      width: 200px;
      padding: 25px 12px 18px;
    }
    .content {
      padding: 0 25px 38px;
    }
    h1 {
      font-size: 36px;
    }
  }
  @media (max-width: 600px) {
    aside {
      position: absolute;
      z-index: 5;
      inset-block: 0;
      inset-inline-start: 0;
      inset-inline-end: auto;
      width: min(250px, calc(100vw - 45px));
      box-shadow: 15px 0 50px #00000055;
    }
    aside.collapsed {
      box-shadow: none;
    }
    .sidebar-backdrop {
      display: block;
      position: absolute;
      z-index: 4;
      inset: 0;
      border: 0;
      background: var(--paper);
      opacity: 0;
      pointer-events: none;
    }
    .sidebar-backdrop.visible {
      opacity: 1;
      pointer-events: auto;
    }
    .content {
      margin-inline-start: 64px;
      padding: 0 18px 38px;
    }
    h1 {
      font-size: 34px;
    }
    .section-heading {
      gap: 8px;
    }
    .section-heading h2 {
      font-size: 14px;
    }
    .text-button {
      font-size: 9px;
    }
    .library-heading {
      flex-direction: column;
      gap: 0;
      padding-top: 27px;
    }
    .library-heading h1 {
      font-size: 32px;
    }
    .library-heading .primary-button {
      margin-top: 18px;
    }
    .section-note {
      max-width: 90px;
      text-align: end;
      font-size: 8px;
    }
    dialog {
      padding: 23px;
    }
    .review-dialog {
      padding: 0;
    }
    dialog h2 {
      font-size: 25px;
    }
  }

  @media (max-width: 600px) {
    .heading-actions {
      margin-top: 18px;
    }
    .heading-actions .primary-button {
      margin-top: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    * {
      transition: none;
    }
    .folder-card:hover,
    .memory-card:hover {
      transform: none;
    }
  }
  @media (prefers-reduced-transparency: reduce) {
    dialog::backdrop {
      -webkit-backdrop-filter: none;
      backdrop-filter: none;
    }
  }
  :global([dir='rtl']) .folder-disclosure:not(.expanded) {
    transform: scaleX(-1);
  }
  :global([dir='rtl']) .sidebar-toggle svg,
  :global([dir='rtl']) .directional-icon {
    transform: scaleX(-1);
  }
  @media (max-width: 600px) {
    :global([dir='rtl']) aside:not(.collapsed) {
      box-shadow: -15px 0 50px #00000055;
    }
  }
</style>
