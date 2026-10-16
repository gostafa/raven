import type { Card, ContentItem, ItemDraft, EditorItem } from './types';

export function textDraft(text = '', key: string): EditorItem {
  return { key, kind: 'text', text };
}

export function editItems(card: Card, urlFor: (item: ContentItem) => string): EditorItem[] {
  return card.items.map((item) => ({
    key: item.id,
    id: item.id,
    kind: item.kind,
    ...(item.kind === 'text'
      ? { text: item.text ?? '' }
      : { originalName: item.originalName, previewURL: urlFor(item) }),
  }));
}

export function saveItems(items: EditorItem[]): ItemDraft[] {
  return items
    .filter((item) => item.kind !== 'text' || (item.text?.trim().length ?? 0) > 0)
    .map(({ id, kind, text, sourcePath }) => ({ id, kind, text, sourcePath }));
}

export function hasContent(items: ItemDraft[]): boolean {
  return items.some((item) =>
    item.kind === 'text'
      ? (item.text?.trim().length ?? 0) > 0
      : (item.id?.length ?? 0) > 0 || (item.sourcePath?.length ?? 0) > 0,
  );
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
