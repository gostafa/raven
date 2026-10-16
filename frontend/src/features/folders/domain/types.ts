export interface Folder {
  id: string;
  title: string;
  description: string;
  tags: string[];
}
export interface Preferences {
  selectedFolderId: string | null;
  sidebarOpen: boolean;
}
export interface Workspace extends Preferences {
  folders: Folder[];
}
export interface PreferencesStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}
export interface FolderPort {
  ListFolders(): Promise<Folder[]>;
  CreateFolder(input: { title: string; description: string; tags: string[] }): Promise<Folder>;
  DeleteFolder(folderId: string): Promise<void>;
}
