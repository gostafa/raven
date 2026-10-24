<script lang="ts">
  import './style.css';
  import { createViewPreferences } from './preferences/funcs.svelte';
  import FolderEditorDialog from './FolderEditorDialog.svelte';
  import MemoryEditorDialog from './MemoryEditorDialog.svelte';
  import WorkspaceIcon from './WorkspaceIcon.svelte';
  import WorkspaceSidebar from './WorkspaceSidebar.svelte';
  import WorkspaceSkeleton from './WorkspaceSkeleton.svelte';
  import FolderBrowser from './FolderBrowser.svelte';
  import CardBrowser from './CardBrowser.svelte';
  import WorkspaceDialogs from './WorkspaceDialogs.svelte';

  import { useLocalization, type MessageKey } from '../../localization';
  const l = useLocalization();

  import { onMount, tick } from 'svelte';
  import type { Card } from '../../cards';
  import type { AppPorts } from '../domain/types';
  import { loadWorkspace as loadData, deleteSelection } from '../application/funcs';

  import { ReviewSession } from '../../reviews';

  import {
    ArchiveDialog,
    exportFolders,
    importFolders,
    type ArchiveResult,
    type ImportMode,
  } from '../../folderarchive';
  import { cardPreview, selectCards, matchesCard, mergeCardSnapshot, removeCardPart } from '../../cards';
  import { reviewItems, shuffledReviewItems, type ReviewItem } from '../../reviews';
  import {
    selectFolders,
    emptyWorkspace,
    loadPreferences,
    reconcileFolders,
    savePreferences,
    type Workspace,
    type Folder,
  } from '../../folders';

  let { backend }: { backend: AppPorts } = $props();
  const viewPreferences = createViewPreferences(() => backend);
  const view = viewPreferences.view;
  let error = $state<MessageKey | ''>('');
  let preferencesError = $state<MessageKey | ''>('');
  const workspaceError = $derived(error || preferencesError || viewPreferences.error);
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
  const filteredFolders = $derived(
    selectFolders(workspace.folders, view.folderTags, view.folderSort, folderMetrics, $l.compare),
  );
  const query = $derived(view.search.trim().toLowerCase());
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
    selectCards(matchingCards, { sort: view.cardSort, kinds: view.cardKinds, progressRange: view.cardProgressRange }),
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
  let selectingFiles = $state(false);
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
  }
  async function restoreDialogFocus() {
    await tick();
    const target = dialogOpener?.isConnected
      ? dialogOpener
      : dialogFallback?.isConnected
        ? dialogFallback
        : document.querySelector<HTMLElement>(
            '.workspace-folder-grid button, .workspace-memory-grid button, .workspace-library-heading .workspace-primary-button',
          );
    (target ?? sidebarToggle)?.focus();
  }

  let folderEditor: { open: (folder?: Folder) => void };
  let memoryEditor: { open: (card?: Card) => void };

  function openFolder(event: MouseEvent) {
    if (status !== 'ready' || saving) return;
    rememberOpener(event);
    folderEditor.open();
  }

  function editFolder(opener: HTMLElement, folder: Folder) {
    if (saving) return;
    rememberOpener(opener);
    folderEditor.open(folder);
  }

  function folderSaved(folder: Folder, editing: boolean) {
    workspace = reconcileFolders(
      editing ? workspace : { ...workspace, selectedFolderId: folder.id },
      editing
        ? workspace.folders.map((saved) => (saved.id === folder.id ? folder : saved))
        : [...workspace.folders, folder],
    );
    persistPreferences();
  }

  function openCard(event: MouseEvent) {
    if (status !== 'ready' || saving) return;
    rememberOpener(event);
    memoryEditor.open();
  }

  function editCard(event: MouseEvent | HTMLElement, card: Card) {
    if (saving || selectingFiles) return;
    rememberOpener(event);
    memoryEditor.open(card);
  }

  function cardSaved(card: Card, editing: boolean) {
    cards = editing
      ? cards.map((saved) => (saved.folderId === card.folderId && saved.id === card.id ? card : saved))
      : mergeCardSnapshot(cards, card);
    now = backend.now();
    if (selectedFolder?.id !== card.folderId) navigate(card.folderId);
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

<main class="workspace-app-shell">
  <WorkspaceSidebar
    {workspace}
    {status}
    bind:search={view.search}
    bind:foldersOpen={view.foldersOpen}
    {query}
    {visibleFolders}
    {selectedFolder}
    {saving}
    {shortcut}
    bind:sidebarToggle
    bind:searchInput
    {setSidebarOpen}
    {navigate}
    {focusSearch}
    {openFolder}
    {openArchive}
    {cardsInFolder}
  />
  <section class="workspace-content" aria-label={selectedFolder?.title ?? $l.t('workspace.folders')}>
    <p class="workspace-control workspace-visually-hidden" role="status">
      {status === 'loading' ? $l.t('workspace.gathering_your_memories') : ''}
    </p>
    <div class="workspace-page-container" aria-busy={status === 'loading'}>
      {#if workspaceError}<p class="workspace-control workspace-error" role="alert">
          {$l.t(workspaceError)}
        </p>{/if}
      {#if status === 'loading'}
        <WorkspaceSkeleton cardsView={workspace.selectedFolderId !== null || !!query} />
      {:else if status === 'error'}<div class="workspace-empty-state">
          <div class="workspace-empty-icon"><WorkspaceIcon name="cards" size={26} /></div>
          <h1 class="workspace-control">{$l.t('workspace.let_s_reconnect')}</h1>
          <p class="workspace-control">{$l.t('workspace.your_memories_are_saved_in_your_local_database')}</p>
          <button class="workspace-control workspace-primary-button" onclick={loadWorkspace}
            >{$l.t('workspace.try_again')} <WorkspaceIcon name="repeat" size={16} /></button
          >
        </div>
      {:else}
        {#if !selectedFolder && !query}
          <FolderBrowser
            {saving}
            folders={workspace.folders}
            {filteredFolders}
            bind:folderSort={view.folderSort}
            bind:folderTags={view.folderTags}
            {cards}
            mediaURL={backend.mediaURL}
            nativeStart={backend.previewStart}
            {scheduledItems}
            {now}
            {openFolder}
            {navigate}
            {cardsInFolder}
            {startFolderReview}
            {exportSingleFolder}
            {editFolder}
            {confirmDelete}
          />
        {:else}
          <CardBrowser
            {selectedFolder}
            {query}
            bind:search={view.search}
            {saving}
            {selectingFiles}
            {scheduledItems}
            {visibleFolders}
            {visibleCards}
            {matchingCards}
            bind:cardSort={view.cardSort}
            bind:cardKinds={view.cardKinds}
            bind:cardProgressRange={view.cardProgressRange}
            mediaURL={backend.mediaURL}
            nativeStart={backend.previewStart}
            {openCard}
            {navigate}
            {startFolderReview}
            {exportSingleFolder}
            {editFolder}
            {confirmDelete}
            {openPart}
            {startReview}
            {editCard}
          />
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

<WorkspaceDialogs
  bind:partDialog
  bind:partDeleteDialog
  bind:deleteDialog
  bind:selectedPart
  bind:partError
  {saving}
  {deleteError}
  {deletion}
  mediaURL={backend.mediaURL}
  {restoreDialogFocus}
  {removePart}
  {removeSelection}
/>
<FolderEditorDialog
  {backend}
  bind:this={folderEditor}
  bind:folderDialog
  bind:saving
  onsaved={folderSaved}
  onclose={restoreDialogFocus}
/>
<MemoryEditorDialog
  {backend}
  folders={workspace.folders}
  selectedFolderId={workspace.selectedFolderId}
  bind:this={memoryEditor}
  bind:cardDialog
  bind:saving
  bind:selectingFiles
  onsaved={cardSaved}
  onfoldercreated={(folder) => {
    workspace = reconcileFolders(workspace, [...workspace.folders, folder]);
  }}
  onclose={restoreDialogFocus}
/>

<dialog
  class="workspace-control workspace-review-dialog"
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
