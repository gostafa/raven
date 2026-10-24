import type { Card, CardSelection, ContentItem, ItemDraft, EditorItem, CardSelectionEntry } from './types';

function selectionEntry(card: Card): CardSelectionEntry {
  const items = card.items.filter((item) => item.hint !== true);
  const dates = items.map((item) => new Date(item.schedule.Due).getTime()).filter(Number.isFinite);
  return { card, nextReview: dates.length ? Math.min(...dates) : null, progress: cardProgress(card) };
}
function selectedEntry(entry: CardSelectionEntry, selection: CardSelection): boolean {
  const { minimum, maximum } = selection.progressRange ?? { minimum: 0, maximum: 100 };
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
function missingSchedule(
  left: CardSelectionEntry,
  right: CardSelectionEntry,
  sort: CardSelection['sort'],
): number | null {
  if (!sort.startsWith('review-')) return null;
  if (left.nextReview !== null && right.nextReview !== null) return null;
  return reviewDifference(left, right);
}
function directedDifference(difference: number, sort: CardSelection['sort']): number {
  return sort.endsWith('-desc') ? -difference : difference;
}
function compareEntries(left: CardSelectionEntry, right: CardSelectionEntry, sort: CardSelection['sort']): number {
  const missing = missingSchedule(left, right, sort);
  if (missing !== null) return missing || selectionTie(left.card, right.card);
  return directedDifference(selectionDifference(left, right, sort), sort) || selectionTie(left.card, right.card);
}
export function selectCards(cards: readonly Card[], selection: CardSelection): Card[] {
  return cards
    .map(selectionEntry)
    .filter((entry) => selectedEntry(entry, selection))
    .sort((left, right) => compareEntries(left, right, selection.sort))
    .map(({ card }) => card);
}

export function textDraft(text: string, key: string): EditorItem {
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

function hasValue(value: string | undefined): boolean {
  return value !== undefined && value.length > 0;
}
function itemHasContent(item: ItemDraft): boolean {
  if (item.kind === 'text') return hasValue(item.text?.trim());
  return hasValue(item.id) || hasValue(item.sourcePath);
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
