<script lang="ts">
  import { useLocalization } from '../../localization';
  const l = useLocalization();
  import { LanguageSelector } from '../../localization';
  import { Skeleton } from '../../ui';
  import type { Workspace, Folder } from '../../folders';
  import type { Card } from '../../cards';
  import WorkspaceIcon from './WorkspaceIcon.svelte';
  let {
    workspace,
    status,
    search = $bindable(),
    foldersOpen = $bindable(),
    query,
    visibleFolders,
    selectedFolder,
    saving,
    shortcut,
    sidebarToggle = $bindable(),
    searchInput = $bindable(),
    setSidebarOpen,
    navigate,
    focusSearch,
    openFolder,
    openArchive,
    cardsInFolder,
  }: {
    workspace: Workspace;
    status: 'loading' | 'ready' | 'error';
    search: string;
    foldersOpen: boolean;
    query: string;
    visibleFolders: Folder[];
    selectedFolder: Folder | undefined;
    saving: boolean;
    shortcut: string;
    sidebarToggle: HTMLButtonElement | undefined;
    searchInput: HTMLInputElement | undefined;
    setSidebarOpen: (open: boolean) => Promise<void>;
    navigate: (id?: string | null) => void;
    focusSearch: () => Promise<void>;
    openFolder: (event: MouseEvent) => void;
    openArchive: (event: MouseEvent, operation: 'import' | 'export') => void;
    cardsInFolder: (id: string) => Card[];
  } = $props();
</script>

<button
  class="workspace-control workspace-sidebar-backdrop"
  class:workspace-visible={workspace.sidebarOpen}
  aria-label={$l.t('workspace.collapse_sidebar')}
  aria-hidden={!workspace.sidebarOpen}
  disabled={!workspace.sidebarOpen}
  tabindex="-1"
  onclick={() => setSidebarOpen(false)}
></button>
<aside
  class="workspace-control"
  id="sidebar"
  class:workspace-collapsed={!workspace.sidebarOpen}
  aria-label={$l.t('workspace.main_navigation')}
>
  <header class="workspace-sidebar-header">
    <button
      class="workspace-control workspace-wordmark"
      onclick={() => navigate()}
      aria-label={$l.t('workspace.raven_home')}
      ><span class="workspace-control workspace-brand-icon"
        ><img class="workspace-control" src="/raven.svg" alt="" /></span
      ><span class="workspace-control workspace-sidebar-label"
        >raven<span class="workspace-control workspace-brand-dot">.</span></span
      ></button
    ><button
      class="workspace-control workspace-icon-button workspace-sidebar-toggle"
      bind:this={sidebarToggle}
      aria-label={workspace.sidebarOpen ? $l.t('workspace.collapse_sidebar') : $l.t('workspace.expand_sidebar')}
      title={workspace.sidebarOpen ? $l.t('workspace.collapse_sidebar') : $l.t('workspace.expand_sidebar')}
      aria-expanded={workspace.sidebarOpen}
      aria-controls="sidebar"
      onclick={() => setSidebarOpen(!workspace.sidebarOpen)}><WorkspaceIcon name="sidebar" size={17} /></button
    >
  </header>
  {#if workspace.sidebarOpen}<div class="workspace-sidebar-search">
      <WorkspaceIcon name="search" size={16} /><input
        class="workspace-control"
        dir="auto"
        type="search"
        aria-label={$l.t('workspace.search_cards_and_folders')}
        placeholder={$l.t('workspace.find_a_memory')}
        bind:this={searchInput}
        bind:value={search}
        onkeydown={(event) => {
          if (event.key === 'Escape') search = '';
        }}
      />{#if !search}<kbd class="workspace-control" dir="ltr">{shortcut}</kbd>{/if}
    </div>
  {:else}<button
      class="workspace-control workspace-icon-button workspace-collapsed-search"
      aria-label={$l.t('workspace.search_cards_and_folders')}
      title={$l.t('workspace.searchShortcut', { shortcut })}
      onclick={focusSearch}><WorkspaceIcon name="search" /></button
    >{/if}
  <nav class="workspace-control" aria-label={$l.t('workspace.workspace')}>
    <p class="workspace-control workspace-nav-caption workspace-sidebar-label">{$l.t('workspace.your_space')}</p>
    <div class="workspace-folder-nav-row">
      {#if workspace.sidebarOpen}<button
          class="workspace-control workspace-icon-button workspace-folder-disclosure"
          class:workspace-expanded={foldersOpen || !!query}
          aria-label={foldersOpen || query ? $l.t('workspace.collapse_folders') : $l.t('workspace.expand_folders')}
          aria-expanded={foldersOpen || !!query}
          aria-controls="folder-navigation"
          onclick={() => {
            foldersOpen = !(foldersOpen || !!query);
            if (query) search = '';
          }}><WorkspaceIcon name="chevron" size={13} /></button
        >{/if}
      <button
        class="workspace-control workspace-nav-item workspace-all-folders"
        aria-current={!workspace.selectedFolderId ? 'page' : undefined}
        aria-label={$l.t('workspace.all_folders')}
        title={$l.t('workspace.all_folders')}
        onclick={() => navigate()}
        >{#if workspace.sidebarOpen}<span class="workspace-control workspace-sidebar-label"
            >{$l.t('workspace.all_folders')}</span
          >
          {#if status === 'ready'}<span class="workspace-control workspace-nav-total workspace-sidebar-label"
              >{$l.number(workspace.folders.length)}</span
            >{/if}
        {:else}<WorkspaceIcon name="folder" size={17} />{/if}</button
      >
      {#if workspace.sidebarOpen}<button
          class="workspace-control workspace-icon-button workspace-folder-create"
          aria-label={$l.t('workspace.new_folder')}
          title={$l.t('workspace.new_folder')}
          disabled={status !== 'ready'}
          onclick={openFolder}><WorkspaceIcon name="plus" size={16} /></button
        >{/if}
    </div>
    {#if workspace.sidebarOpen && (foldersOpen || query)}<div
        id="folder-navigation"
        class="workspace-folder-navigation"
        aria-busy={status === 'loading'}
      >
        {#if status === 'loading'}
          {#each [0, 1, 2, 3] as index (index)}<div class="workspace-sidebar-skeleton" aria-hidden="true">
              <Skeleton width="15px" height="15px" radius="3px" />
              <Skeleton width={index % 2 ? '55%' : '70%'} height="11px" />
            </div>{/each}
        {:else if status === 'ready'}
          {#each visibleFolders as folder (folder.id)}<button
              class="workspace-control workspace-nav-item workspace-folder-link"
              class:workspace-selected={selectedFolder?.id === folder.id}
              aria-current={selectedFolder?.id === folder.id ? 'page' : undefined}
              title={folder.title}
              onclick={() => navigate(folder.id)}
              ><WorkspaceIcon name="folder" size={15} /><span class="workspace-control workspace-sidebar-label"
                ><bdi>{folder.title}</bdi></span
              ><span class="workspace-control workspace-nav-total workspace-sidebar-label"
                >{$l.number(cardsInFolder(folder.id).length)}</span
              ></button
            >{:else}<p class="workspace-control workspace-sidebar-empty">
              {query ? $l.t('workspace.no_matching_folders') : $l.t('workspace.no_folders_yet_use_to_create_one')}
            </p>{/each}
        {/if}
      </div>{/if}
    {#if !workspace.sidebarOpen}<button
        class="workspace-control workspace-nav-item"
        title={$l.t('workspace.new_folder')}
        aria-label={$l.t('workspace.new_folder')}
        disabled={status !== 'ready'}
        onclick={openFolder}><WorkspaceIcon name="plus" size={16} /></button
      >{/if}
  </nav>
  <footer class="workspace-sidebar-footer">
    <div class="workspace-sidebar-transfers" role="group" aria-label={$l.t('workspace.import_and_export_folders')}>
      <button
        class="workspace-control workspace-footer-action"
        type="button"
        aria-label={$l.t('workspace.import_folders')}
        data-tooltip={$l.t('workspace.import_folders')}
        disabled={saving || status !== 'ready'}
        onclick={(event) => openArchive(event, 'import')}><WorkspaceIcon name="import" size={18} /></button
      >
      <button
        class="workspace-control workspace-footer-action"
        type="button"
        aria-label={$l.t('workspace.export_folders')}
        data-tooltip={$l.t('workspace.export_folders')}
        disabled={saving || status !== 'ready' || !workspace.folders.length}
        onclick={(event) => openArchive(event, 'export')}><WorkspaceIcon name="export" size={18} /></button
      >
    </div>
    <div class="workspace-footer-language"><LanguageSelector compact /></div>
  </footer>
</aside>
