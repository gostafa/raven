import type { EditorItem, MediaKind, MediaPort } from '../domain/types';
export async function selectMedia(port: MediaPort, kind: MediaKind, items: EditorItem[]): Promise<EditorItem[]> {
  const files = await port.SelectMediaFiles(kind);
  return [
    ...items,
    ...files.map((file) => ({
      key: port.newKey(),
      kind,
      sourcePath: file.sourcePath,
      originalName: file.originalName,
      previewURL: file.previewURL,
    })),
  ];
}
