<script lang="ts">
  import { onMount, tick } from 'svelte';
  import type { Card } from '../../cards';
  import type { AppPorts } from '../domain/types';
  import { loadWorkspace as loadData, saveCard as saveMemory, deleteSelection } from '../application/funcs';
  import { CardEditor } from '../../cards';
  import { CardContent } from '../../cards';
  import { ReviewSession } from '../../reviews';
  import { ActionMenu } from '../../ui';
  import {
    cardPreview,
    editItems,
    hasContent,
    matchesCard,
    mergeCardSnapshot,
    saveItems,
    textDraft,
    type EditorItem,
  } from '../../cards';
  import { reviewItems, type ReviewItem } from '../../reviews';
  import {
    emptyWorkspace,
    loadPreferences,
    parseTags,
    reconcileFolders,
    savePreferences,
    type Workspace,
  } from '../../folders';

  let { backend }: { backend: AppPorts } = $props();
  let error = $state('');
  let preferencesError = $state('');
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
  let search = $state('');
  let foldersOpen = $state(true);
  const query = $derived(search.trim().toLowerCase());
  const visibleFolders = $derived(
    workspace.folders.filter((folder) =>
      [folder.title, folder.description, ...folder.tags].some((value) => value.toLowerCase().includes(query)),
    ),
  );
  const visibleCards = $derived(
    cards
      .filter(
        (card) =>
          (!selectedFolder || card.folderId === selectedFolder.id) &&
          matchesCard(card, folderName(card.folderId), query),
      )
      .toSorted((a, b) => b.createdAt - a.createdAt),
  );
  const shortcut = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘ K' : 'Ctrl K';
  let searchInput = $state<HTMLInputElement>();
  let sidebarToggle = $state<HTMLButtonElement>();
  let folderDialog = $state<HTMLDialogElement>();
  let cardDialog = $state<HTMLDialogElement>();
  let deleteDialog = $state<HTMLDialogElement>();
  let deletion = $state<{ folderId: string; cardId?: string; title: string } | null>(null);
  let deleteError = $state('');
  let reviewDialog = $state<HTMLDialogElement>();
  let titleInput = $state<HTMLInputElement>();
  let title = $state('');
  let description = $state('');
  let tags = $state('');
  let editorItems = $state<EditorItem[]>([]);
  let editingCard = $state<Card | null>(null);
  let selectingFiles = $state(false);
  let cardFolder = $state('');
  let newFolderTitle = $state('');
  let formError = $state('');
  let saving = $state(false);
  let dialogOpener: HTMLElement | null = null;
  let dialogFallback: HTMLElement | null = null;
  let reviewQueue = $state<ReviewItem[]>([]);
  let reviewOpen = $state(false);
  let reviewSaving = $state(false);
  let savedCard = $state<Card | null>(null);

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
      preferencesError = 'Your navigation preferences could not be loaded.';
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
      error = 'Your saved memories could not be loaded. Try again to reconnect to your local database.';
    }
  }

  function persistPreferences() {
    try {
      savePreferences(backend.preferences, workspace, backend.configuration.preferencesStorageKey);
      preferencesError = '';
    } catch {
      preferencesError = 'Your navigation preferences could not be saved on this device.';
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
      deleteDialog?.open ||
      reviewDialog?.open ||
      !(event.metaKey || event.ctrlKey) ||
      event.key.toLowerCase() !== 'k'
    )
      return;
    event.preventDefault();
    void focusSearch();
  }

  function navigate(folderId: string | null = null) {
    if (folderId) foldersOpen = true;
    search = '';
    savedCard = null;
    workspace = { ...workspace, selectedFolderId: folderId };
    persistPreferences();
    if (window.matchMedia('(max-width: 600px)').matches) void setSidebarOpen(false);
  }

  function folderName(id: string) {
    return workspace.folders.find((folder) => folder.id === id)?.title ?? 'Folder';
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
    title = '';
    description = '';
    tags = '';
    folderDialog?.showModal();
    titleInput?.focus();
  }

  async function createFolder(event: SubmitEvent) {
    event.preventDefault();
    if (saving) return;
    if (!title.trim()) {
      formError = 'Give your folder a name.';
      titleInput?.focus();
      return;
    }
    saving = true;
    formError = '';
    try {
      const folder = await backend.CreateFolder({ title, description, tags: parseTags(tags) });
      workspace = reconcileFolders({ ...workspace, selectedFolderId: folder.id }, [...workspace.folders, folder]);
      search = '';
      persistPreferences();
      folderDialog?.close();
    } catch {
      formError = 'Your folder could not be saved. Your entries are still here; try again.';
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
    newFolderTitle = workspace.folders.length ? '' : 'Everyday learning';
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
      savedCard = null;
      now = backend.now();
      deleteDialog?.close();
    } catch {
      deleteError = 'Could not delete this item. Please try again.';
    } finally {
      saving = false;
    }
  }

  async function createCard(event: SubmitEvent) {
    event.preventDefault();
    if (saving || selectingFiles) return;
    const items = saveItems(editorItems);
    if (!hasContent(items) || (cardFolder === '__new' && !newFolderTitle.trim())) {
      formError = 'Add something you want to remember and choose a folder for this card.';
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
      savedCard = card;
      cardDialog?.close();
    } catch (error) {
      formError = `Your card could not be saved. Your entries are still here. ${error instanceof Error ? error.message : 'Try again.'}`;
    } finally {
      saving = false;
    }
  }

  function startReview(event: MouseEvent | HTMLElement, card?: Card) {
    if (reviewOpen) return;
    rememberOpener(event);
    void openReview(card ? reviewItems([card]) : [...dueCards]);
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
    {:else if name === 'check'}<path d="m5 12 4 4L19 6" />
    {:else if name === 'clock'}<circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" />
    {:else if name === 'close'}<path d="m6 6 12 12M18 6 6 18" />
    {:else if name === 'repeat'}<path d="M20 7h-9a7 7 0 1 0 7 10M16 3l4 4-4 4" />{/if}
  </svg>
{/snippet}

<main class="app-shell">
  <button
    class="sidebar-backdrop"
    class:visible={workspace.sidebarOpen}
    aria-label="Collapse sidebar"
    aria-hidden={!workspace.sidebarOpen}
    disabled={!workspace.sidebarOpen}
    tabindex="-1"
    onclick={() => setSidebarOpen(false)}
  ></button>
  <aside id="sidebar" class:collapsed={!workspace.sidebarOpen} aria-label="Main navigation">
    <header class="sidebar-header">
      <button class="wordmark" onclick={() => navigate()} aria-label="Raven home"
        ><span class="brand-icon"><img src="/raven.svg" alt="" /></span><span class="sidebar-label"
          >raven<span class="brand-dot">.</span></span
        ></button
      ><button
        class="icon-button sidebar-toggle"
        bind:this={sidebarToggle}
        aria-label={workspace.sidebarOpen ? 'Collapse sidebar' : 'Expand sidebar'}
        title={workspace.sidebarOpen ? 'Collapse sidebar' : 'Expand sidebar'}
        aria-expanded={workspace.sidebarOpen}
        aria-controls="sidebar"
        onclick={() => setSidebarOpen(!workspace.sidebarOpen)}>{@render icon('sidebar', 17)}</button
      >
    </header>
    {#if workspace.sidebarOpen}<div class="sidebar-search">
        {@render icon('search', 16)}<input
          type="search"
          aria-label="Search cards and folders"
          placeholder="Find a memory…"
          bind:this={searchInput}
          bind:value={search}
          onkeydown={(event) => {
            if (event.key === 'Escape') search = '';
          }}
        />{#if !search}<kbd>{shortcut}</kbd>{/if}
      </div>
    {:else}<button
        class="icon-button collapsed-search"
        aria-label="Search cards and folders"
        title={`Search (${shortcut})`}
        onclick={focusSearch}>{@render icon('search')}</button
      >{/if}
    <nav aria-label="Workspace">
      <p class="nav-caption sidebar-label">YOUR SPACE</p>
      <div class="folder-nav-row">
        {#if workspace.sidebarOpen}<button
            class="icon-button folder-disclosure"
            class:expanded={foldersOpen || !!query}
            aria-label={foldersOpen || query ? 'Collapse folders' : 'Expand folders'}
            aria-expanded={foldersOpen || !!query}
            aria-controls="folder-navigation"
            onclick={() => {
              foldersOpen = !(foldersOpen || !!query);
              if (query) search = '';
            }}>{@render icon('chevron', 13)}</button
          >{/if}
        <button
          class="nav-item all-folders"
          aria-current={!selectedFolder ? 'page' : undefined}
          aria-label="All folders"
          title="All folders"
          onclick={() => navigate()}
          >{#if workspace.sidebarOpen}<span class="sidebar-label">All folders</span><span
              class="nav-total sidebar-label">{workspace.folders.length}</span
            >{:else}{@render icon('folder', 17)}{/if}</button
        >
        {#if workspace.sidebarOpen}<button
            class="icon-button folder-create"
            aria-label="New folder"
            title="New folder"
            disabled={status !== 'ready'}
            onclick={openFolder}>{@render icon('plus', 16)}</button
          >{/if}
      </div>
      {#if workspace.sidebarOpen && (foldersOpen || query)}<div id="folder-navigation" class="folder-navigation">
          {#each visibleFolders as folder (folder.id)}<button
              class="nav-item folder-link"
              class:selected={selectedFolder?.id === folder.id}
              aria-current={selectedFolder?.id === folder.id ? 'page' : undefined}
              title={folder.title}
              onclick={() => navigate(folder.id)}
              >{@render icon('folder', 15)}<span class="sidebar-label">{folder.title}</span><span
                class="nav-total sidebar-label">{cardsInFolder(folder.id).length}</span
              ></button
            >{:else}<p class="sidebar-empty">
              {query ? 'No matching folders' : 'No folders yet. Use + to create one.'}
            </p>{/each}
        </div>{/if}
      {#if !workspace.sidebarOpen}<button
          class="nav-item"
          title="New folder"
          aria-label="New folder"
          disabled={status !== 'ready'}
          onclick={openFolder}>{@render icon('plus', 16)}</button
        >{/if}
    </nav>
  </aside>
  <section class="content" aria-label={selectedFolder?.title ?? 'Folders'}>
    <div class="page-container">
      {#if selectedFolder}<header class="topbar">
          <div class="breadcrumb">
            Your space <span>/</span><span class="current-page">{selectedFolder?.title ?? 'Folders'}</span>
          </div>
          <div class="topbar-date">
            {new Date(now).toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })}
          </div>
        </header>{/if}
      {#if error || preferencesError}<p class="error" role="alert">{error || preferencesError}</p>{/if}
      {#if status === 'loading'}<div class="loading-state" role="status">
          <span class="loading-mark">✳</span>
          <h1>Gathering your memories…</h1>
          <p>Picking up where you left off.</p>
        </div>
      {:else if status === 'error'}<div class="empty-state">
          <div class="empty-icon">{@render icon('cards', 26)}</div>
          <h1>Let’s reconnect.</h1>
          <p>Your memories are saved in your local database.</p>
          <button class="primary-button" onclick={loadWorkspace}>Try again {@render icon('repeat', 16)}</button>
        </div>
      {:else}
        {#if savedCard}<div class="saved-notice">
            <span role="status">{@render icon('check', 15)} Memory saved. Its items are scheduled for review.</span
            ><button class="text-button" onclick={(event) => startReview(event, savedCard ?? undefined)}
              >Review now {@render icon('arrow', 14)}</button
            ><button
              class="icon-button"
              aria-label="Dismiss saved message"
              onclick={() => {
                savedCard = null;
              }}>{@render icon('close', 14)}</button
            >
          </div>{/if}
        {#if !selectedFolder && !query}
          <section class="library-heading">
            <div>
              <p class="eyebrow">YOUR SPACE</p>
              <h1>Your folders</h1>
              <p>A home for the things you want to remember.</p>
            </div>
            <button class="primary-button" onclick={openFolder}>{@render icon('plus', 17)} New folder</button>
          </section>
          <ul class="folder-grid" aria-label="Your folders">
            {#each workspace.folders as folder (folder.id)}
              <li class="folder-tile">
                <button class="folder-card" onclick={() => navigate(folder.id)}>
                  <h3>{folder.title}</h3>
                  <p>{folder.description || 'A collection of things worth remembering.'}</p>
                  <span class="folder-card-bottom">
                    {#if folder.tags.length}<span class="folder-tags"
                        >{#each folder.tags as tag (tag)}<span class="folder-tag">{tag}</span>{/each}</span
                      >{/if}
                    <span
                      >{cardsInFolder(folder.id).length}
                      {cardsInFolder(folder.id).length === 1 ? 'card' : 'cards'}</span
                    >
                    {#if cardsInFolder(folder.id).some((card) => card.dueAt <= now)}<span class="due-dot"
                        >Ready to review</span
                      >{/if}
                  </span>
                </button>
                <span class="tile-actions"
                  ><ActionMenu
                    label={`Options for folder ${folder.title}`}
                    disabled={saving}
                    actions={[{ label: 'Delete folder', onselect: (opener) => confirmDelete(opener, folder.id) }]}
                  /></span
                >
              </li>
            {/each}
            <li class="new-folder-tile">
              <button class="new-folder-card" onclick={openFolder}
                ><span>{@render icon('plus', 22)}</span><strong>A new curiosity?</strong><span>Give it a folder.</span
                ></button
              >
            </li>
          </ul>
        {:else}
          <section class="library-heading">
            <div>
              <p class="eyebrow">
                {query ? 'A LITTLE HELP FINDING IT' : selectedFolder ? 'YOUR COLLECTION' : 'THINGS WORTH REMEMBERING'}
              </p>
              <h1>{query ? 'Search your memories' : (selectedFolder?.title ?? 'Search your memories')}</h1>
              <p>
                {query
                  ? `Results for “${search.trim()}”`
                  : selectedFolder?.description || 'Big ideas, small details, and everything you want to keep.'}
              </p>
              {#if selectedFolder?.tags.length}<div class="tags">
                  {#each selectedFolder.tags as tag (tag)}<span>{tag}</span>{/each}
                </div>{/if}
            </div>
            <div class="heading-actions">
              <button class="primary-button" onclick={(event) => openCard(event)}
                >{@render icon('plus', 17)} New card</button
              >{#if selectedFolder}{@const folder = selectedFolder}<ActionMenu
                  label={`Options for folder ${folder.title}`}
                  disabled={saving}
                  actions={[{ label: 'Delete folder', onselect: (opener) => confirmDelete(opener, folder.id) }]}
                />{/if}
            </div>
          </section>
          {#if query && !selectedFolder && visibleFolders.length}<div class="section-heading">
              <h2>Folders <span class="count">{visibleFolders.length}</span></h2>
            </div>
            <div class="search-folders">
              {#each visibleFolders as folder (folder.id)}<button
                  class="secondary-button"
                  onclick={() => navigate(folder.id)}
                  >{@render icon('folder', 16)} {folder.title} {@render icon('arrow', 15)}</button
                >{/each}
            </div>{/if}
          <div class="section-heading">
            <h2>{selectedFolder ? 'Memory cards' : 'Your cards'} <span class="count">{visibleCards.length}</span></h2>
            {#if query}<button
                class="text-button"
                onclick={() => {
                  search = '';
                }}>Clear search {@render icon('close', 14)}</button
              >{:else}<span class="section-note">Select a card to practice recall</span>{/if}
          </div>
          <p class="sr-only" role="status">{query ? `${visibleCards.length} cards match your search.` : ''}</p>
          {#if visibleCards.length}<ul class="memory-grid" aria-label="Memory cards">
              {#each visibleCards as card (`${card.folderId}/${card.id}`)}
                <li class="memory-tile">
                  <div class="memory-card">
                    <div class="memory-card-preview"><CardContent {card} mediaURL={backend.mediaURL} compact /></div>
                  </div>
                  <span class="tile-actions"
                    ><ActionMenu
                      label={`Options for card ${cardPreview(card)}`}
                      disabled={saving || selectingFiles}
                      actions={[
                        { label: 'Review card', onselect: (opener) => startReview(opener, card) },
                        { label: 'Edit card', onselect: (opener) => editCard(opener, card) },
                        { label: 'Delete card', onselect: (opener) => confirmDelete(opener, card.folderId, card) },
                      ]}
                    /></span
                  >
                </li>
              {/each}
            </ul>
          {:else}<div class="empty-state">
              <div class="empty-icon">{@render icon(query ? 'search' : 'cards', 27)}</div>
              <h2>{query ? 'No cards found. Keep looking.' : 'Your next memory starts here.'}</h2>
              <p>
                {query
                  ? 'Try a word from the content, a filename, or the folder name.'
                  : 'Add information you want to remember. Raven will bring it back for review.'}
              </p>
              {#if query}<button
                  class="secondary-button"
                  onclick={() => {
                    search = '';
                  }}>Clear search</button
                >{:else}<button class="primary-button" onclick={(event) => openCard(event)}
                  >{@render icon('plus', 16)} Create a card</button
                >{/if}
            </div>{/if}
        {/if}
        <footer class="page-footer"><span>✳</span> A little practice today. A little more remembered tomorrow.</footer>
      {/if}
    </div>
  </section>
</main>

<dialog
  bind:this={folderDialog}
  aria-labelledby="folder-dialog-title"
  onclose={restoreDialogFocus}
  oncancel={(event) => {
    if (saving) event.preventDefault();
  }}
>
  <form onsubmit={createFolder} aria-busy={saving} novalidate>
    <div class="dialog-top">
      <span class="empty-icon">{@render icon('folder', 24)}</span><button
        type="button"
        class="icon-button"
        aria-label="Close new folder"
        disabled={saving}
        onclick={() => folderDialog?.close()}>{@render icon('close')}</button
      >
    </div>
    <p class="eyebrow">MAKE ROOM FOR A CURIOSITY</p>
    <h2 id="folder-dialog-title">A home for related memories.</h2>
    <p class="dialog-description">Keep cards about the same topic together.</p>
    <label for="folder-title">Folder name <span>Required</span></label><input
      id="folder-title"
      bind:this={titleInput}
      bind:value={title}
      placeholder="e.g. Everyday Spanish"
      disabled={saving}
      required
      aria-invalid={!!formError && !title.trim()}
    />
    <label for="folder-description">A little context <span>Optional</span></label><textarea
      id="folder-description"
      bind:value={description}
      rows="2"
      placeholder="What would you like to remember?"
      disabled={saving}></textarea>
    <label for="folder-tags">Tags <span>Optional</span></label><input
      id="folder-tags"
      bind:value={tags}
      placeholder="language, travel, learning"
      aria-describedby="tags-hint"
      disabled={saving}
    />
    <p class="field-hint" id="tags-hint">Separate tags with commas.</p>
    {#if formError}<p class="error" role="alert">{formError}</p>{/if}
    <div class="dialog-actions">
      <button type="button" class="secondary-button" disabled={saving} onclick={() => folderDialog?.close()}
        >Cancel</button
      ><button type="submit" class="primary-button" disabled={saving}
        >{saving ? 'Saving…' : 'Create folder'} {@render icon('arrow', 16)}</button
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
    <h2 id="delete-title">Delete {deletion?.cardId ? 'card' : 'folder'}?</h2>
    <p class="dialog-description">
      “{deletion?.title}” will be permanently deleted. {deletion?.cardId
        ? 'Its content and review history will be removed.'
        : `All ${cardsInFolder(deletion?.folderId ?? '').length} cards in this folder, including their content and review history, will be removed.`}
      This cannot be undone.
    </p>
    {#if deleteError}<p class="error" role="alert">{deleteError}</p>{/if}
    <div class="dialog-actions">
      <button type="button" class="secondary-button" disabled={saving} onclick={() => deleteDialog?.close()}
        >Cancel</button
      ><button type="submit" class="primary-button" disabled={saving}
        >{saving ? 'Deleting…' : 'Delete permanently'}</button
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
        aria-label="Close card editor"
        disabled={saving || selectingFiles}
        onclick={() => cardDialog?.close()}>{@render icon('close')}</button
      >
    </div>
    <p class="eyebrow">A NOTE TO YOUR FUTURE SELF</p>
    <h2 id="card-dialog-title">{editingCard ? 'Edit your memory.' : 'What’s worth remembering?'}</h2>
    <p class="dialog-description">Keep information together with text, images, audio, and video.</p>
    <CardEditor
      media={backend}
      bind:items={editorItems}
      bind:selecting={selectingFiles}
      disabled={saving}
      onerror={(message) => (formError = message)}
    />
    {#if cardFolder === '__new'}<label for="card-new-folder">New folder name</label><input
        id="card-new-folder"
        bind:value={newFolderTitle}
        placeholder="e.g. Everyday Spanish"
        disabled={saving}
        required
        aria-invalid={!!formError && !newFolderTitle.trim()}
      />{/if}
    <p class="save-note">
      {@render icon('clock', 15)}
      {editingCard ? 'Your review schedule stays the same.' : 'Ready for your first review as soon as you save.'}
    </p>
    {#if formError}<p class="error" role="alert">{formError}</p>{/if}
    <div class="dialog-actions">
      <button
        type="button"
        class="secondary-button"
        disabled={saving || selectingFiles}
        onclick={() => cardDialog?.close()}>Cancel</button
      ><button type="submit" class="primary-button" disabled={saving || selectingFiles}
        >{saving ? 'Saving…' : editingCard ? 'Save changes' : 'Save memory'} {@render icon('arrow', 16)}</button
      >
    </div>
  </form>
</dialog>

<dialog
  class="review-dialog"
  bind:this={reviewDialog}
  aria-labelledby="review-title"
  onclose={closeReview}
  oncancel={(event) => {
    if (reviewSaving) event.preventDefault();
  }}
>
  {#if reviewOpen}<ReviewSession
      review={backend}
      mediaURL={backend.mediaURL}
      entries={reviewQueue}
      folders={workspace.folders}
      bind:saving={reviewSaving}
      onsaved={reviewSaved}
      onclose={() => reviewDialog?.close()}
    />{/if}
</dialog>

<style>
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
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
    border-right: 1px solid #ffffff14;
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
    font-weight: 650;
    letter-spacing: -1.3px;
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
    text-align: left;
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
  .nav-total {
    margin-left: auto;
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
    padding-left: 6px;
    padding-right: 6px;
    gap: 8px;
  }
  .all-folders:hover {
    background: transparent;
  }
  .folder-create {
    color: var(--muted);
  }
  .folder-navigation {
    margin: 4px 0 0 13px;
    padding-left: 12px;
    border-left: 1px solid var(--line);
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
    padding-left: 10px;
    padding-right: 10px;
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
  .content .icon-button:hover,
  dialog .icon-button:hover {
    color: var(--ink);
  }
  .content {
    flex: 1;
    min-width: 0;
    overflow: auto;
    padding: 0 38px;
  }
  .page-container {
    display: flex;
    flex-direction: column;
    max-width: 1100px;
    min-height: 100%;
    margin: auto;
  }
  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 20px;
    min-height: 64px;
    border-bottom: 1px solid var(--line);
    font-size: 10px;
  }
  .breadcrumb {
    display: flex;
    align-items: center;
    gap: 10px;
    color: var(--ink);
    min-width: 0;
  }
  .breadcrumb > span:first-child {
    color: var(--ink);
  }
  .current-page {
    color: var(--ink);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .topbar-date {
    flex-shrink: 0;
    color: var(--ink);
    font-size: 10px;
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
    font-family: Georgia, 'Times New Roman', serif;
    font-size: clamp(32px, 3.9vw, 49px);
    font-weight: 400;
    letter-spacing: -1.7px;
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
    text-align: left;
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
  .folder-tile,
  .new-folder-tile {
    padding-top: 15px;
  }
  .folder-tile::before,
  .new-folder-tile::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
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
  .new-folder-tile:focus-within::before,
  .folder-tile:hover::before,
  .folder-tile:focus-within::before {
    border-color: var(--line-strong);
  }
  .tile-actions {
    position: absolute;
    top: 10px;
    right: 10px;
  }
  .folder-tile .tile-actions {
    top: 25px;
  }
  .folder-card,
  .new-folder-card {
    width: 100%;
    height: 100%;
    min-height: 179px;
    padding: 18px;
    border: 1px solid var(--line);
    border-radius: 9px;
    background: var(--paper);
    color: var(--ink);
    text-align: left;
  }
  .folder-card,
  .new-folder-card {
    position: relative;
    display: flex;
    flex-direction: column;
    min-height: 190px;
    border-top-left-radius: 0;
  }
  .folder-card::before,
  .new-folder-card::before {
    content: '';
    position: absolute;
    top: -1px;
    left: 0;
    width: 39%;
    height: 1px;
    background: var(--paper);
  }
  .folder-tile:hover .folder-card,
  .folder-tile:focus-within .folder-card,
  .memory-tile:hover .memory-card {
    border-color: var(--line-strong);
  }
  .folder-card h3 {
    margin: 0 0 8px;
    padding-right: 40px;
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
  .page-footer {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    margin-top: auto;
    padding: 30px 0 22px;
    color: var(--ink);
    font-size: 8px;
    text-align: center;
    line-height: 1.7;
  }
  .page-footer span {
    font-size: 16px;
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
    display: flex;
    flex-direction: column;
    width: 100%;
    aspect-ratio: 4 / 5;
    min-height: 0;
    overflow: hidden;
    padding: 14px;
    border: 1px solid var(--line);
    border-radius: 9px;
    background: var(--paper);
    color: var(--ink);
    text-align: left;
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
  .card-dialog {
    width: min(620px, calc(100vw - 36px));
  }
  .empty-state,
  .loading-state {
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
  .empty-state h2,
  .loading-state h1 {
    margin: 20px 0 10px;
    font:
      26px/1.3 Georgia,
      serif;
    color: var(--ink);
  }
  .empty-state p,
  .loading-state p {
    margin: 0 0 22px;
    color: var(--ink);
    font-size: 11px;
    line-height: 1.8;
  }
  .loading-state {
    margin-top: 35px;
    border: 0;
  }
  .loading-mark {
    color: var(--ink);
    font-size: 35px;
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
  .saved-notice {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 12px;
    margin-top: 15px;
    border: 1px solid var(--line);
    border-radius: 7px;
    background: var(--paper);
    color: var(--ink);
    font-size: 11px;
  }
  .saved-notice > span {
    display: flex;
    align-items: center;
    gap: 7px;
  }
  .saved-notice > .text-button {
    margin-left: auto;
    flex-shrink: 0;
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
    width: min(580px, calc(100vw - 36px));
    padding: 28px 35px 32px;
  }
  @media (min-width: 1250px) {
    .content {
      padding: 0 55px;
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
      padding: 0 25px;
    }
    h1 {
      font-size: 36px;
    }
  }
  @media (max-width: 600px) {
    aside {
      position: absolute;
      z-index: 5;
      inset: 0 auto 0 0;
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
      margin-left: 64px;
      padding: 0 18px;
    }
    .topbar {
      min-height: 65px;
      gap: 10px;
      font-size: 9px;
    }
    .topbar-date {
      font-size: 8px;
    }
    .breadcrumb {
      gap: 6px;
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
    .page-footer {
      font-size: 7px;
      gap: 6px;
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
      text-align: right;
      font-size: 8px;
    }
    dialog {
      padding: 23px;
    }
    .review-dialog {
      padding: 22px;
    }
    dialog h2 {
      font-size: 25px;
    }
  }

  @media (max-width: 600px) {
    .saved-notice {
      flex-wrap: wrap;
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
</style>
