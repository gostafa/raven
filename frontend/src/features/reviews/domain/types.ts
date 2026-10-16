import type { Card, ContentItem } from '../../cards';
export interface ReviewItem {
  card: Card;
  item: ContentItem;
  dueAt: number;
}
export interface ReviewPort {
  ReviewItem(folderId: string, cardId: string, itemId: string, rating: number): Promise<Card>;
}
export interface ReviewEvents {
  onOpenFloatingReview(callback: () => void): () => void;
  onCloseFloatingReview(callback: () => void): () => void;
}
