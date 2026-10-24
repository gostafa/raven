import type { FolderPaper } from '../domain/types';
import type { Card } from '../../cards';

export function selectFolderPapers(cards: readonly Card[], folderId: string): FolderPaper[] {
  return cards
    .filter((card) => card.folderId === folderId)
    .flatMap((card) =>
      card.items
        .filter((item) => item.hint !== true && (item.kind === 'image' || item.kind === 'video'))
        .map((item) => ({ card, item, due: reviewTime(item.schedule.Due) })),
    )
    .toSorted(
      (left, right) =>
        left.due - right.due ||
        left.card.id.localeCompare(right.card.id) ||
        left.item.position - right.item.position ||
        left.item.id.localeCompare(right.item.id),
    )
    .slice(0, 5)
    .map(({ card, item }) => ({
      key: JSON.stringify([card.id, item.id]),
      kind: item.kind === 'video' ? 'video' : 'image',
      item,
    }));
}

function reviewTime(due: string): number {
  const time = new Date(due).getTime();
  return Number.isFinite(time) ? time : Infinity;
}
