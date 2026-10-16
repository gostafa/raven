import type { Card, ContentItem, ItemDraft } from '../bindings/raven/internal/storage/models';

export type MediaKind = 'image' | 'audio' | 'video';
export type EditorItem = ItemDraft & { key: string; originalName?: string; previewURL?: string };

export function textDraft(text = ''): EditorItem {
  return { key: crypto.randomUUID(), kind: 'text', text };
}

export function editItems(card: Card): EditorItem[] {
  return (card.items ?? []).map((item) => ({
    key: item.id, id: item.id, kind: item.kind,
    ...(item.kind === 'text' ? { text: item.text ?? '' } : { originalName: item.originalName, previewURL: mediaURL(item) }),
  }));
}

export function saveItems(items: EditorItem[]): ItemDraft[] {
  return items.filter((item) => item.kind !== 'text' || item.text?.trim()).map(({ id, kind, text, sourcePath }) => ({ id, kind, text, sourcePath }));
}

export function hasContent(items: ItemDraft[]): boolean {
  return items.some((item) => item.kind === 'text' ? !!item.text?.trim() : !!(item.id || item.sourcePath));
}

export function moveItem(items: EditorItem[], key: string, direction: -1 | 1): EditorItem[] {
  const index = items.findIndex((item) => item.key === key);
  if (index < 0) return items;
  const next = index + direction;
  if (next < 0 || next >= items.length) return items;
  const moved = [...items];
  [moved[index], moved[next]] = [moved[next], moved[index]];
  return moved;
}

export function orderedItems(card: Card): ContentItem[] {
  return (card.items ?? []).toSorted((a, b) => a.position - b.position);
}

export function cardPreview(card: Card): string {
  return orderedItems(card).map((item) => item.text || item.originalName || item.kind).join(' · ');
}

export function matchesCard(card: Card, folder: string, query: string): boolean {
  return [folder, ...(card.items ?? []).flatMap((item) => [item.text ?? '', item.originalName ?? ''])].some((value) => value.toLowerCase().includes(query.trim().toLowerCase()));
}

export function mediaURL(item: ContentItem): string {
  return `/media/${[item.folderId, item.cardId, item.id].map(encodeURIComponent).join('/')}`;
}

export function stopMedia(root: ParentNode | undefined): void {
  root?.querySelectorAll<HTMLMediaElement>('audio, video').forEach((media) => media.pause());
}

export type ReviewItem = { card: Card; item: ContentItem; dueAt: number };
export function reviewItems(cards: Card[]): ReviewItem[] {
  return cards.flatMap((card) => (card.items ?? []).map((item) => ({ card, item, dueAt: new Date(item.schedule.Due).getTime() })))
    .sort((a, b) => a.dueAt - b.dueAt || `${a.card.folderId}/${a.card.id}/${a.item.id}`.localeCompare(`${b.card.folderId}/${b.card.id}/${b.item.id}`));
}
export function lastUserReview(item: ContentItem): number {
  return Math.max(0, ...(item.reviews ?? []).filter((review) => !review.automatic).map((review) => new Date(review.log.Review).getTime()));
}
