import type { ArchivePort, ImportMode } from '../domain/types';

export async function exportFolders(ports: ArchivePort, folderIds: string[]) {
  const selection = [...new Set(folderIds)];
  if (!selection.length || selection.some((id) => !id)) throw new Error('Select at least one folder to export.');
  return ports.ExportFolders(selection);
}

export async function importFolders(ports: ArchivePort, mode: ImportMode | '', refresh: () => Promise<void>) {
  if (mode !== 'fresh' && mode !== 'resume') throw new Error('Choose how to start reviewing the imported cards.');
  const result = await ports.ImportFolders(mode);
  if (!result.canceled) await refresh();
  return result;
}
