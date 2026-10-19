import type { Card, ContentItem } from '../../cards';
import type { ReviewItem } from './types';

export function reviewItems(cards: Card[]): ReviewItem[] {
  return cards
    .flatMap((card) =>
      card.items
        .filter((item) => item.hint !== true)
        .map((item) => ({ card, item, dueAt: new Date(item.schedule.Due).getTime() })),
    )
    .sort(
      (a, b) =>
        a.dueAt - b.dueAt ||
        `${a.card.folderId}/${a.card.id}/${a.item.id}`.localeCompare(`${b.card.folderId}/${b.card.id}/${b.item.id}`),
    );
}
export function lastUserReview(item: ContentItem): number {
  return Math.max(
    0,
    ...item.reviews.filter((review) => !review.automatic).map((review) => new Date(review.log.Review).getTime()),
  );
}
