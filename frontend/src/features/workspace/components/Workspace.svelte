<script lang="ts">
  import './style.css';
  import CardTransferDialog from './CardTransferDialog.svelte';
  import {
    reference,
    referenceKey,
    createClipboard,
    visibleSelection,
    applyTransfer,
    executeTransfer,
    type CardReference,
    type CardClipboard,
  } from '../../cardtransfer';
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

  import { onMount, tick, untrack } from 'svelte';
  import type { Card } from '../../cards';
  import type { AppPorts } from '../domain/types';
  import { loadWorkspace as loadData, deleteSelection, reviewSelection } from '../application/funcs';

  import { ReviewSession, reviewItems, shuffledReviewItems, type ReviewItem } from '../../reviews';

  import {
    ArchiveDialog,
    exportFolders,
    importFolders,
    type ArchiveResult,
    type ImportMode,
  } from '../../folderarchive';
  import { cardPreview, selectCards, matchesCard, mergeCardSnapshot, removeCardPart } from '../../cards';
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
  function notify(message: MessageKey, options?: { count: number }) {
    backend.Notify('Raven', $l.t(message, options)).catch(() => {
      // Native delivery is optional; a failed notification must not create an in-app message.
    });
  }
  $effect(() => {
    const message = viewPreferences.error;
    if (message)
      untrack(() => {
        notify(message);
      });
  });
  let workspace = $state(restoreWorkspace());
  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let cards = $state<Card[]>([]);
  let cardsRevision = 0;
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
    selectFolders(workspace.folders, {
      tags: view.folderTags,
      sort: view.folderSort,
      metrics: folderMetrics,
      compare: (left, right) => $l.compare(left, right),
    }),
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
  let selectingCards = $state(false);
  let selectedCards = $state<CardReference[]>([]);
  let clipboard = $state<CardClipboard | null>(null);
  let transferRequest = $state<CardClipboard | null>(null);
  let transferDialog = $state<HTMLDialogElement>();
  let transferError = $state<MessageKey | ''>('');
  const canPaste = $derived(clipboard !== null);
  const currentSelection = $derived(visibleSelection(selectedCards, visibleCards));
  $effect(() => {
    if (currentSelection.length !== selectedCards.length) selectedCards = currentSelection;
  });

  function toggleSelectionMode() {
    selectingCards = !selectingCards;
    selectedCards = [];
    restoreSelectionFocus();
  }
  function toggleSelected(card: Card) {
    const source = reference(card);
    const key = referenceKey(source);
    selectedCards = selectedCards.some((entry) => referenceKey(entry) === key)
      ? selectedCards.filter((entry) => referenceKey(entry) !== key)
      : [...selectedCards, source];
  }
  function selectAllShown() {
    selectedCards = visibleCards.map(reference);
  }
  function clearSelection() {
    selectedCards = [];
  }
  function clearClipboard() {
    clipboard = null;
  }
  function putClipboard(sources: CardReference[]) {
    if (saving || selectingFiles || status !== 'ready') return;
    clipboard = createClipboard('copy', sources);
    selectedCards = [];
    transferError = '';
    notifyClipboard();
  }
  function notifyClipboard() {
    if (clipboard) {
      notify('transfer.clipboard_copy', {
        count: clipboard.sources.length,
      });
    }
  }
  function copyCard(opener: HTMLElement, card: Card) {
    rememberOpener(opener);
    putClipboard([reference(card)]);
  }
  function copySelected(opener: HTMLElement) {
    rememberOpener(opener);
    putClipboard(currentSelection);
    restoreSelectionFocus();
  }
  function restoreSelectionFocus() {
    tick()
      .then(() =>
        document
          .querySelector<HTMLButtonElement>('.workspace-transfer-toolbar button, .workspace-selection-toggle')
          ?.focus(),
      )
      .catch(reportActionFailure);
  }
  function moveCard(opener: HTMLElement, card: Card) {
    beginMove(opener, [reference(card)]);
  }
  function moveSelected(opener: HTMLElement) {
    beginMove(opener, currentSelection);
  }
  function canTransfer() {
    return !saving && !selectingFiles && status === 'ready';
  }
  function showTransferDialog() {
    transferDialog?.showModal();
    transferDialog?.querySelector<HTMLInputElement>('input[type="search"]')?.focus();
  }
  function beginMove(opener: HTMLElement, sources: CardReference[]) {
    if (!canTransfer()) return;
    rememberOpener(opener);
    transferRequest = createClipboard('move', sources);
    transferError = '';
    if (transferRequest) tick().then(showTransferDialog).catch(reportActionFailure);
  }
  function pasteCards(opener: HTMLElement, folderId?: string) {
    if (!clipboard || !canTransfer()) return;
    rememberOpener(opener);
    transferRequest = createClipboard(clipboard.operation, clipboard.sources);
    transferError = '';
    if (folderId !== undefined) performTransfer(folderId).catch(reportActionFailure);
    else tick().then(showTransferDialog).catch(reportActionFailure);
  }
  function beginTransfer() {
    saving = true;
    transferError = '';
  }
  async function performTransfer(folderId: string) {
    if (!transferRequest || saving) return;
    const request = transferRequest;
    beginTransfer();
    try {
      const completed = await executeTransfer(backend, {
        request: { ...request, destinationFolderId: folderId },
        clipboard,
      });
      applyCompletedTransfer(request, completed);
      finishTransferFocus();
    } catch {
      reportTransferFailure();
    } finally {
      saving = false;
    }
  }
  function applyCompletedTransfer(request: CardClipboard, completed: Awaited<ReturnType<typeof executeTransfer>>) {
    cardsRevision++;
    cards = applyTransfer(cards, request.operation, completed.result);
    clipboard = completed.clipboard;
    selectedCards = [];
    now = backend.now();
    notify(request.operation === 'move' ? 'transfer.moved_count' : 'transfer.copied_count', {
      count: completed.result.entries.length,
    });
  }
  function finishTransferFocus() {
    if (transferDialog?.open === true) transferDialog.close();
    else {
      transferRequest = null;
      restoreDialogFocus().catch(reportActionFailure);
    }
  }
  function reportTransferFailure() {
    transferError = 'transfer.transfer_failed';
    if (transferDialog?.open !== true) {
      transferRequest = null;
      notify(transferError);
    }
  }
  function closeTransferDialog() {
    transferRequest = null;
    transferError = '';
    restoreDialogFocus().catch(reportActionFailure);
  }
  function shortcutSources(event: KeyboardEvent): CardReference[] {
    if (currentSelection.length) return currentSelection;
    const key =
      event.target instanceof Element
        ? event.target.closest<HTMLElement>('[data-card-key]')?.dataset.cardKey
        : undefined;
    const card = visibleCards.find((candidate) => referenceKey(reference(candidate)) === key);
    return card ? [reference(card)] : [];
  }
  function editableTarget(target: EventTarget | null) {
    const selector =
      'textarea, select, input:not([type=checkbox]):not([type=radio]):not([type=button])' +
      ':not([type=submit]), [contenteditable]:not([contenteditable="false"])';
    return target instanceof Element && target.closest(selector) !== null;
  }
  function transferEventIgnored(event: KeyboardEvent) {
    return (
      event.defaultPrevented || event.isComposing || !canTransfer() || hasOpenDialog() || editableTarget(event.target)
    );
  }
  function transferCommand(event: KeyboardEvent) {
    return (event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey;
  }
  function handleTransferShortcut(event: KeyboardEvent) {
    if (transferEventIgnored(event)) return;
    if (event.key === 'Escape') {
      handleTransferEscape(event);
      return;
    }
    if (transferCommand(event)) handleTransferCommand(event);
  }
  function popoverEvent(target: EventTarget | null) {
    return (
      (target instanceof Element && target.closest('[popover]') !== null) ||
      document.querySelector(':popover-open') !== null
    );
  }
  function handleTransferEscape(event: KeyboardEvent) {
    if (popoverEvent(event.target)) return;
    if (selectingCards) {
      selectedCards = [];
      selectingCards = false;
      restoreSelectionFocus();
      event.preventDefault();
    }
  }
  function handleTransferCommand(event: KeyboardEvent) {
    const key = event.key.toLowerCase();
    if (key === 'v') pasteFromShortcut(event);
    else if (key === 'c') copyFromShortcut(event);
  }
  function pasteFromShortcut(event: KeyboardEvent) {
    if (!clipboard) return;
    event.preventDefault();
    const opener = event.target instanceof HTMLElement ? event.target : sidebarToggle;
    if (opener) pasteCards(opener, selectedFolder?.id);
  }
  function hasSelectedText() {
    return (window.getSelection()?.toString().length ?? 0) > 0;
  }
  function copyFromShortcut(event: KeyboardEvent) {
    const sources = shortcutSources(event);
    if (!sources.length) return;
    if (currentSelection.length === 0 && hasSelectedText()) return;
    event.preventDefault();
    putClipboard(sources);
    restoreSelectionFocus();
  }

  function reportActionFailure() {
    notify('companion.raven_could_not_complete_that_action_please_try_again');
  }
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
      notify('workspace.folders_were_imported_but_the_library_could_not_be_refreshed_reopen_the_library_to_see_them');
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
      else notify(message);
    } finally {
      saving = false;
    }
  }

  function exportSingleFolder(opener: HTMLElement, folderId: string) {
    rememberOpener(opener);
    transferArchive(() => exportFolders(backend, [folderId])).catch(reportActionFailure);
  }

  function exportSelectedFolders(ids: string[]) {
    transferArchive(() => exportFolders(backend, ids)).catch(reportActionFailure);
  }

  function exportFolderSelection(opener: HTMLElement, ids: string[]) {
    if (saving || status !== 'ready' || !ids.length) return;
    rememberOpener(opener);
    exportSelectedFolders(ids);
  }

  function importArchivedFolders(mode: ImportMode) {
    transferArchive(() => importFolders(backend, mode, refreshImportedFolders)).catch(reportActionFailure);
  }

  async function refreshCards() {
    const revision = cardsRevision;
    try {
      const refreshed = await backend.ListCards();
      if (revision !== cardsRevision) return;
      cards = refreshed;
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
    refreshCards().catch(reportActionFailure);
    backend.RefreshReminders().catch(reportActionFailure);
  }
  onMount(() => {
    const unsubscribe = backend.events.onSchedulesChanged(() => {
      refreshCards().catch(reportActionFailure);
    });
    const unsubscribeReview = backend.events.onShowReviews(() => {
      showReviews().catch(reportActionFailure);
    });
    loadWorkspace().catch(reportActionFailure);
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
      notify('workspace.your_navigation_preferences_could_not_be_loaded');
      return emptyWorkspace();
    }
  }

  async function loadWorkspace() {
    status = 'loading';
    try {
      const loaded = await loadData(backend, workspace);
      workspace = loaded.workspace;
      cards = loaded.cards;
      now = loaded.now;
      status = 'ready';
    } catch {
      status = 'error';
      notify('workspace.your_saved_memories_could_not_be_loaded_try_again_to_reconnect_to_your_local_database');
    }
  }

  function persistPreferences() {
    try {
      savePreferences(backend.preferences, workspace, backend.configuration.preferencesStorageKey);
    } catch {
      notify('workspace.your_navigation_preferences_could_not_be_saved_on_this_device');
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

  function hasOpenDialog(): boolean {
    return (
      [folderDialog, cardDialog, partDialog, partDeleteDialog, deleteDialog, reviewDialog, transferDialog].some(
        (dialog) => dialog?.open === true,
      ) || archiveOperation !== null
    );
  }
  function handleShortcut(event: KeyboardEvent) {
    handleTransferShortcut(event);
    if (hasOpenDialog() || !(event.metaKey || event.ctrlKey) || event.key.toLowerCase() !== 'k') return;
    event.preventDefault();
    focusSearch().catch(reportActionFailure);
  }

  function navigate(folderId: string | null = null) {
    selectedCards = [];
    selectingCards = false;
    workspace = { ...workspace, selectedFolderId: folderId };
    persistPreferences();
    if (window.matchMedia('(max-width: 600px)').matches) setSidebarOpen(false).catch(reportActionFailure);
  }

  function folderName(id: string) {
    return workspace.folders.find((folder) => folder.id === id)?.title ?? $l.t('workspace.folder');
  }
  function cardsInFolder(id: string) {
    return cards.filter((card) => card.folderId === id);
  }
  function tileButton(tile: Element | null | undefined): HTMLElement | null {
    return tile?.querySelector<HTMLElement>('button') ?? null;
  }
  function adjacentButton(tile: Element | null): HTMLElement | null {
    return tileButton(tile?.nextElementSibling) ?? tileButton(tile?.previousElementSibling);
  }
  function rememberOpener(source: MouseEvent | HTMLElement) {
    const opener = source instanceof HTMLElement ? source : source.currentTarget;
    if (!(opener instanceof HTMLElement)) return;
    dialogOpener = opener;
    const tile = opener.closest('li');
    dialogFallback = adjacentButton(tile);
  }
  async function restoreDialogFocus() {
    await tick();
    const target =
      [dialogOpener, dialogFallback].find((element) => element?.isConnected === true) ??
      document.querySelector<HTMLElement>(
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

  function beginPartRemoval() {
    saving = true;
    partError = '';
  }
  function applyPartRemoval(card: Card, saved: Card | null) {
    cards = saved
      ? mergeCardSnapshot(cards, saved)
      : cards.filter((candidate) => candidate.folderId !== card.folderId || candidate.id !== card.id);
    now = backend.now();
    partDeleteDialog?.close();
    partDialog?.close();
  }
  async function removePart() {
    if (!selectedPart || saving) return;
    const { card, itemId } = selectedPart;
    beginPartRemoval();
    try {
      applyPartRemoval(card, await removeCardPart(backend, card, itemId));
    } catch {
      partError = 'workspace.this_part_could_not_be_removed_please_try_again';
    } finally {
      saving = false;
    }
  }
  function beginDeletion() {
    saving = true;
    deleteError = '';
  }
  function applyDeletion(target: { folderId: string; cardId?: string }) {
    cards = cards.filter(
      (card) => card.folderId !== target.folderId || ((target.cardId?.length ?? 0) > 0 && card.id !== target.cardId),
    );
    if ((target.cardId?.length ?? 0) === 0) removeFolderSnapshot(target.folderId);
    now = backend.now();
    deleteDialog?.close();
  }
  function removeFolderSnapshot(folderId: string) {
    workspace = reconcileFolders(
      workspace,
      workspace.folders.filter((folder) => folder.id !== folderId),
    );
    persistPreferences();
  }
  async function removeSelection(event: SubmitEvent) {
    event.preventDefault();
    if (!deletion || saving) return;
    const target = deletion;
    beginDeletion();
    try {
      await deleteSelection(backend, target);
      applyDeletion(target);
    } catch {
      deleteError = 'workspace.could_not_delete_this_item_please_try_again';
    } finally {
      saving = false;
    }
  }

  function startReview(event: MouseEvent | HTMLElement, card?: Card) {
    if (reviewOpen) return;
    rememberOpener(event);
    openReview(card ? reviewItems([card]) : [...dueCards]).catch(reportActionFailure);
  }

  function reviewSelected(opener: HTMLElement) {
    if (reviewOpen || !canTransfer()) return;
    rememberOpener(opener);
    openReview(reviewSelection(cards, currentSelection)).catch(reportActionFailure);
  }

  function startFolderReview(opener: HTMLElement, folderId: string) {
    if (reviewOpen) return;
    rememberOpener(opener);
    openReview(shuffledReviewItems(cardsInFolder(folderId))).catch(reportActionFailure);
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
    restoreDialogFocus().catch(reportActionFailure);
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
      {#if status === 'loading'}
        <WorkspaceSkeleton cardsView={workspace.selectedFolderId !== null || Boolean(query)} />
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
            {exportFolderSelection}
            {editFolder}
            {confirmDelete}
            {canPaste}
            {pasteCards}
            {clearClipboard}
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
            {selectingCards}
            selectedCards={currentSelection}
            {canPaste}
            {toggleSelectionMode}
            {toggleSelected}
            {selectAllShown}
            {clearSelection}
            {moveSelected}
            {copySelected}
            {reviewSelected}
            {moveCard}
            {copyCard}
            {pasteCards}
            {clearClipboard}
          />
        {/if}
      {/if}
    </div>
  </section>
</main>

<CardTransferDialog
  bind:dialog={transferDialog}
  folders={workspace.folders}
  moving={transferRequest?.operation === 'move'}
  busy={saving}
  error={transferError ? $l.t(transferError) : ''}
  onsubmit={(folderId: string) => {
    performTransfer(folderId).catch(reportActionFailure);
  }}
  onclose={closeTransferDialog}
/>

<ArchiveDialog
  operation={archiveOperation}
  folders={workspace.folders}
  busy={saving}
  error={archiveError ? $l.t(archiveError) : ''}
  onexport={exportSelectedFolders}
  onimport={importArchivedFolders}
  onclose={() => {
    archiveOperation = null;
    restoreDialogFocus().catch(reportActionFailure);
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
  onfoldercreated={(folder: Folder) => {
    workspace = reconcileFolders(workspace, [...workspace.folders, folder]);
  }}
  onclose={restoreDialogFocus}
/>

<dialog
  class="workspace-control workspace-review-dialog"
  bind:this={reviewDialog}
  aria-label={$l.t('workspace.review')}
  onclose={closeReview}
  onclick={(event) => {
    if (!reviewSaving && event.target === reviewDialog) reviewDialog?.close();
  }}
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
