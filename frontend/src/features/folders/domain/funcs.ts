import type { Folder, FolderSelection, FolderMetrics, Preferences, PreferencesStorage, Workspace } from './types';

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
  return parsePreferences(data);
}
function parsePreferences(data: Record<string, unknown>): Preferences {
  if (
    typeof data.sidebarOpen !== 'boolean' ||
    (data.selectedFolderId !== null && typeof data.selectedFolderId !== 'string')
  )
    throw new Error('Invalid saved preferences');
  return { selectedFolderId: data.selectedFolderId, sidebarOpen: data.sidebarOpen };
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
  return sortedFolderDifference(left, right, selection) || compare(left.title, right.title);
}
function directedDifference(difference: number, sort: string): number {
  return sort.endsWith('-desc') ? -difference : difference;
}
function sortedFolderDifference(left: Folder, right: Folder, selection: FolderSelection): number {
  const leftMetric = selection.metrics.get(left.id);
  const rightMetric = selection.metrics.get(right.id);
  const reviews = reviewOrdering(leftMetric, rightMetric);
  if (selection.sort.startsWith('review-') && reviews !== null) return reviews;
  return directedDifference(folderDifference(left, right, selection), selection.sort);
}
function reviewTime(metric: FolderMetrics | undefined): number | null {
  return metric?.nextReview ?? null;
}
function cardCount(metric: FolderMetrics | undefined): number {
  return metric?.cardCount ?? 0;
}
function reviewOrdering(leftMetric: FolderMetrics | undefined, rightMetric: FolderMetrics | undefined): number | null {
  const left = reviewTime(leftMetric);
  const right = reviewTime(rightMetric);
  if (left !== null && right !== null) return null;
  if (left === right) return 0;
  return left === null ? 1 : -1;
}
function folderDifference(left: Folder, right: Folder, selection: FolderSelection): number {
  if (selection.sort.startsWith('tags-')) return left.tags.length - right.tags.length;
  const leftMetric = selection.metrics.get(left.id);
  const rightMetric = selection.metrics.get(right.id);
  if (selection.sort.startsWith('cards-')) return cardCount(leftMetric) - cardCount(rightMetric);
  return (reviewTime(leftMetric) ?? 0) - (reviewTime(rightMetric) ?? 0);
}
