import type { Card, CardPort } from '../../cards';
import type { Folder, FolderPort } from '../../folders';
import { reviewItems } from '../domain/funcs';
import type { ReviewItem, ReviewPort } from '../domain/types';
export async function loadDueReview(
  cards: CardPort,
  folders: FolderPort,
  now: () => number,
): Promise<{ entries: ReviewItem[]; folders: Folder[] }> {
  const [savedCards, savedFolders] = await Promise.all([cards.ListCards(), folders.ListFolders()]);
  return { entries: reviewItems(savedCards).filter((entry) => entry.dueAt <= now()), folders: savedFolders };
}
export async function saveReview(port: ReviewPort, entry: ReviewItem, rating: number): Promise<Card> {
  return port.ReviewItem(entry.card.folderId, entry.card.id, entry.item.id, rating);
}
