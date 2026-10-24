import type { Card, CardSelection, ContentItem, ItemDraft, EditorItem, CardSelectionEntry } from './types';

function selectionEntry(card: Card): CardSelectionEntry {
  const items = card.items.filter((item) => item.hint !== true);
  const dates = items.map((item) => new Date(item.schedule.Due).getTime()).filter(Number.isFinite);
  return { card, nextReview: dates.length ? Math.min(...dates) : null, progress: cardProgress(card) };
}
function selectedEntry(entry: CardSelectionEntry, selection: CardSelection): boolean {
  const { minimum = 0, maximum = 100 } = selection.progressRange ?? {};
  const kindsMatch =
    !selection.kinds.length || entry.card.items.some((item) => selection.kinds.some((kind) => kind === item.kind));
  return kindsMatch && entry.progress >= minimum && entry.progress <= maximum;
}
function reviewDifference(left: CardSelectionEntry, right: CardSelectionEntry): number {
  if (left.nextReview === right.nextReview) return 0;
  if (left.nextReview === null) return 1;
  if (right.nextReview === null) return -1;
  return left.nextReview - right.nextReview;
}
function selectionDifference(left: CardSelectionEntry, right: CardSelectionEntry, sort: CardSelection['sort']): number {
  if (sort.startsWith('review-')) return reviewDifference(left, right);
  if (sort.startsWith('progress-')) return left.progress - right.progress;
  return left.card.createdAt - right.card.createdAt;
}
function selectionTie(left: Card, right: Card): number {
  return (
    right.createdAt - left.createdAt || left.folderId.localeCompare(right.folderId) || left.id.localeCompare(right.id)
  );
}
export function selectCards(cards: readonly Card[], selection: CardSelection): Card[] {
  return cards
    .map(selectionEntry)
    .filter((entry) => selectedEntry(entry, selection))
    .sort((left, right) => {
      const difference = selectionDifference(left, right, selection.sort);
      return (selection.sort.endsWith('-desc') ? -difference : difference) || selectionTie(left.card, right.card);
    })
    .map(({ card }) => card);
}

export function textDraft(text = '', key: string): EditorItem {
  return { key, kind: 'text', text };
}

export function editItems(card: Card, urlFor: (item: ContentItem) => string): EditorItem[] {
  return card.items.map((item) => ({
    key: item.id,
    id: item.id,
    kind: item.kind,
    hint: item.hint ?? false,
    ...(item.kind === 'text'
      ? { text: item.text ?? '' }
      : { originalName: item.originalName, previewURL: urlFor(item) }),
  }));
}

export function saveItems(items: EditorItem[]): ItemDraft[] {
  return items
    .filter((item) => item.kind !== 'text' || (item.text?.trim().length ?? 0) > 0)
    .map(({ id, kind, text, sourcePath, hint }) => ({ id, kind, text, sourcePath, hint }));
}

export function hasContent(items: ItemDraft[]): boolean {
  return items.some(itemHasContent);
}

function itemHasContent(item: ItemDraft): boolean {
  if (item.kind === 'text') return (item.text?.trim().length ?? 0) > 0;
  return (item.id?.length ?? 0) > 0 || (item.sourcePath?.length ?? 0) > 0;
}

export function moveItem(items: EditorItem[], key: string, direction: -1 | 1): EditorItem[] {
  const index = items.findIndex((item) => item.key === key);
  if (index < 0) return items;
  const next = index + direction;
  if (next < 0 || next >= items.length) return items;
  const item = items.at(index);
  if (!item) return items;
  const moved = items.slice();
  moved.splice(index, 1);
  moved.splice(next, 0, item);
  return moved;
}

export function orderedItems(card: Card): ContentItem[] {
  return card.items.toSorted((a, b) => a.position - b.position);
}

export function cardProgress(card: Card): number {
  const items = card.items.filter((item) => item.hint !== true);
  if (!items.length) return 0;
  const total = items.reduce((sum, item) => {
    const reviews = item.reviews.filter(
      (review) =>
        !review.automatic && Number.isInteger(review.log.Rating) && review.log.Rating >= 1 && review.log.Rating <= 4,
    );
    if (!reviews.length) return sum;
    return sum + reviews.filter((review) => review.log.Rating > 1).length / reviews.length;
  }, 0);
  return Math.round((total / items.length) * 100);
}

export function cardPreview(card: Card): string {
  return orderedItems(card)
    .map((item) => itemPreview(item))
    .join(' · ');
}

export function matchesCard(card: Card, folder: string, query: string): boolean {
  return [folder, ...card.items.flatMap((item) => [item.text ?? '', item.originalName ?? ''])].some((value) =>
    value.toLowerCase().includes(query.trim().toLowerCase()),
  );
}

export function mergeCardSnapshot(cards: Card[], saved: Card): Card[] {
  const matches = (card: Card) => card.folderId === saved.folderId && card.id === saved.id;
  return cards.some(matches) ? cards.map((card) => (matches(card) ? saved : card)) : [...cards, saved];
}

function itemPreview(item: ContentItem): string {
  if (item.text !== undefined && item.text.length > 0) return item.text;
  return item.originalName !== undefined && item.originalName.length > 0 ? item.originalName : item.kind;
}
