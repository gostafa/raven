import type { Folder, Preferences, PreferencesStorage, Workspace } from './types';
import { preferencesStorageKey } from './consts';

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

export function loadPreferences(storage: Pick<PreferencesStorage, 'getItem'>): Preferences {
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

export function savePreferences(storage: Pick<PreferencesStorage, 'setItem'>, preferences: Preferences): void {
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
