import type { Folder, FolderSelection, Preferences, PreferencesStorage, Workspace } from './types';

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
  const data = preferencesRecord(value);
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
function preferencesRecord(value: unknown): Record<string, unknown> {
  if (isRecord(value)) return value;
  throw new Error('Invalid saved preferences');
}
function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
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

export function selectFolders(folders: Folder[], selection: FolderSelection): Folder[] {
  const { tags, sort } = selection;
  const selected = folders.filter((folder) =>
    tags.every((tag) => folder.tags.some((value) => value.toLowerCase() === tag.toLowerCase())),
  );
  if (sort === 'default') return selected;
  return selected.sort((left, right) => compareFolders(left, right, selection));
}
function compareFolders(left: Folder, right: Folder, selection: FolderSelection): number {
  const compare = selection.compare ?? ((a: string, b: string) => a.localeCompare(b));
  const difference = folderDifference(left, right, selection);
  const leftReview = selection.metrics.get(left.id)?.nextReview ?? null;
  const rightReview = selection.metrics.get(right.id)?.nextReview ?? null;
  if (selection.sort.startsWith('review-') && (leftReview === null || rightReview === null))
    return missingReviewDifference(leftReview, rightReview) || compare(left.title, right.title);
  return (selection.sort.endsWith('-desc') ? -difference : difference) || compare(left.title, right.title);
}
function missingReviewDifference(left: number | null, right: number | null): number {
  if (left === right) return 0;
  return left === null ? 1 : -1;
}
function folderDifference(left: Folder, right: Folder, selection: FolderSelection): number {
  if (selection.sort.startsWith('tags-')) return left.tags.length - right.tags.length;
  const leftMetric = selection.metrics.get(left.id);
  const rightMetric = selection.metrics.get(right.id);
  if (selection.sort.startsWith('cards-')) return (leftMetric?.cardCount ?? 0) - (rightMetric?.cardCount ?? 0);
  return (leftMetric?.nextReview ?? 0) - (rightMetric?.nextReview ?? 0);
}
