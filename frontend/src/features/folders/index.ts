export type { Folder, Workspace, PreferencesStorage, FolderPort } from './domain/types';
export { preferencesStorageKey } from './domain/consts';
export { emptyWorkspace, parseTags, loadPreferences, savePreferences, reconcileFolders } from './domain/funcs';
