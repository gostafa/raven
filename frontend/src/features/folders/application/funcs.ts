import type { FolderPaper } from '../domain/types';
import type { Card } from '../../cards';

export function selectFolderPapers(cards: readonly Card[], folderId: string): FolderPaper[] {
  return cards
    .filter((card) => card.folderId === folderId)
    .toSorted((left, right) => right.createdAt - left.createdAt || left.id.localeCompare(right.id))
    .flatMap((card) => card.items.toSorted((left, right) => left.position - right.position))
    .filter((item) => item.hint !== true && (item.kind === 'image' || item.kind === 'video'))
    .slice(0, 4)
    .map((item) => ({ kind: item.kind === 'video' ? 'video' : 'image' }));
}
