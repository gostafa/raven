import type { ArchivePort, ArchiveResult, ImportMode } from '../domain/types';

export async function exportFolders(ports: ArchivePort, folderIds: string[]): Promise<ArchiveResult> {
  const selection = [...new Set(folderIds)];
  if (!selection.length || selection.some((id) => !id)) throw new Error('Select at least one folder to export.');
  return await ports.ExportFolders(selection);
}

export async function importFolders(
  ports: ArchivePort,
  mode: ImportMode | '',
  refresh: () => Promise<void>,
): Promise<ArchiveResult> {
  if (mode !== 'fresh' && mode !== 'resume') throw new Error('Choose how to start reviewing the imported cards.');
  const result = await ports.ImportFolders(mode);
  if (!result.canceled) await refresh();
  return result;
}
