import type { ContentItem } from '../../cards';

export interface Folder {
  id: string;
  title: string;
  description: string;
  tags: string[];
}
export interface FolderPaper {
  key: string;
  kind: 'image' | 'video';
  item: ContentItem;
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
  UpdateFolder(input: { id: string; title: string; description: string; tags: string[] }): Promise<Folder>;
  DeleteFolder(folderId: string): Promise<void>;
}

export type FolderSort =
  'default' | 'tags-desc' | 'tags-asc' | 'cards-desc' | 'cards-asc' | 'review-asc' | 'review-desc';
export interface FolderMetrics {
  cardCount: number;
  nextReview: number | null;
}
export interface FolderSelection {
  tags: string[];
  sort: FolderSort;
  metrics: ReadonlyMap<string, FolderMetrics>;
  compare?: (left: string, right: string) => number;
}
