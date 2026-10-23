import type { Folder, FolderMetrics, FolderSort, Preferences, PreferencesStorage, Workspace } from './types';

export function emptyWorkspace(): Workspace {
  return { folders: [], selectedFolderId: null, sidebarOpen: true };
}

export function parseTags(value: string): string[] {
  const seen = new Set<string>();
  return value
    .split(',')
    .map((tag) => tag.trim())
    .filter((tag) => {
      const key = tag.toLowerCase();
      if (!tag || seen.has(key)) return false;
      seen.add(key);
      return true;
    });
}

export function loadPreferences(
  storage: Pick<PreferencesStorage, 'getItem'>,
  preferencesStorageKey: string,
): Preferences {
  const saved = storage.getItem(preferencesStorageKey);
  if (saved === null) return { selectedFolderId: null, sidebarOpen: true };

  const value: unknown = JSON.parse(saved);
  if (value === null || typeof value !== 'object') throw new Error('Invalid saved preferences');
  const data = value as Record<string, unknown>;
  if (
    typeof data.sidebarOpen !== 'boolean' ||
    (data.selectedFolderId !== null && typeof data.selectedFolderId !== 'string')
  ) {
    throw new Error('Invalid saved preferences');
  }

  return {
    selectedFolderId: data.selectedFolderId,
    sidebarOpen: data.sidebarOpen,
  };
}

export function savePreferences(
  storage: Pick<PreferencesStorage, 'setItem'>,
  preferences: Preferences,
  preferencesStorageKey: string,
): void {
  const { selectedFolderId, sidebarOpen } = preferences;
  storage.setItem(preferencesStorageKey, JSON.stringify({ selectedFolderId, sidebarOpen }));
}

export function reconcileFolders(workspace: Workspace, folders: Folder[]): Workspace {
  return {
    ...workspace,
    folders: folders.map((folder) => ({ ...folder, tags: [...folder.tags] })),
    selectedFolderId: folders.some((folder) => folder.id === workspace.selectedFolderId)
      ? workspace.selectedFolderId
      : null,
  };
}

export function selectFolders(
  folders: Folder[],
  tags: string[],
  sort: FolderSort,
  metrics: ReadonlyMap<string, FolderMetrics>,
  compare: (left: string, right: string) => number = (left, right) => left.localeCompare(right),
): Folder[] {
  const selected = folders.filter((folder) =>
    tags.every((tag) => folder.tags.some((value) => value.toLowerCase() === tag.toLowerCase())),
  );
  if (sort === 'default') return selected;
  return selected.sort((a, b) => {
    const left = metrics.get(a.id);
    const right = metrics.get(b.id);
    let difference: number;
    if (sort.startsWith('tags-')) difference = a.tags.length - b.tags.length;
    else if (sort.startsWith('cards-')) difference = (left?.cardCount ?? 0) - (right?.cardCount ?? 0);
    else {
      const leftReview = left?.nextReview ?? null;
      const rightReview = right?.nextReview ?? null;
      if (leftReview === null || rightReview === null) {
        return leftReview === null && rightReview === null ? compare(a.title, b.title) : leftReview === null ? 1 : -1;
      }
      difference = leftReview - rightReview;
    }
    return (sort.endsWith('-desc') ? -difference : difference) || compare(a.title, b.title);
  });
}
