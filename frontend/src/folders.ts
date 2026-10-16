import type { Folder as StoredFolder } from '../bindings/raven/internal/storage/models';

// Wails marks all Go slices nullable; the workspace always uses arrays.
export interface Folder extends Omit<StoredFolder, 'tags'> {
  tags: string[];
}

export interface Preferences {
  selectedFolderId: string | null;
  sidebarOpen: boolean;
}

export interface Workspace extends Preferences {
  folders: Folder[];
}

export const preferencesStorageKey = 'raven.preferences.v1';

export function emptyWorkspace(): Workspace {
  return { folders: [], selectedFolderId: null, sidebarOpen: true };
}

export function parseTags(value: string): string[] {
  const seen = new Set<string>();
  return value.split(',').map((tag) => tag.trim()).filter((tag) => {
    const key = tag.toLowerCase();
    if (!tag || seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export function loadPreferences(storage: Pick<Storage, 'getItem'>): Preferences {
  const saved = storage.getItem(preferencesStorageKey);
  if (saved === null) return { selectedFolderId: null, sidebarOpen: true };

  const value: unknown = JSON.parse(saved);
  if (!value || typeof value !== 'object') throw new Error('Invalid saved preferences');
  const data = value as Record<string, unknown>;
  if (typeof data.sidebarOpen !== 'boolean' ||
      (data.selectedFolderId !== null && typeof data.selectedFolderId !== 'string')) {
    throw new Error('Invalid saved preferences');
  }

  return {
    selectedFolderId: data.selectedFolderId as string | null,
    sidebarOpen: data.sidebarOpen,
  };
}

export function savePreferences(storage: Pick<Storage, 'setItem'>, preferences: Preferences): void {
  const { selectedFolderId, sidebarOpen } = preferences;
  storage.setItem(preferencesStorageKey, JSON.stringify({ selectedFolderId, sidebarOpen }));
}

export function reconcileFolders(workspace: Workspace, folders: StoredFolder[]): Workspace {
  return {
    ...workspace,
    folders: folders.map((folder) => ({ ...folder, tags: folder.tags ?? [] })),
    selectedFolderId: folders.some((folder) => folder.id === workspace.selectedFolderId)
      ? workspace.selectedFolderId : null,
  };
}
