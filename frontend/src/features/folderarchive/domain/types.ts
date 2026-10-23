export type ImportMode = 'fresh' | 'resume';

export interface ArchiveResult {
  canceled: boolean;
  filename: string;
  folderIds: string[];
  folderCount: number;
  cardCount: number;
  mediaCount: number;
}

export interface ArchivePort {
  ExportFolders(folderIds: string[]): Promise<ArchiveResult>;
  ImportFolders(mode: ImportMode): Promise<ArchiveResult>;
}
