import type { Card } from '../../cards';
import type { CardClipboard, CardReference, TransferOperation, TransferResult } from './types';

export function reference(card: Card): CardReference {
  return { folderId: card.folderId, cardId: card.id };
}
export function referenceKey(source: CardReference): string {
  return JSON.stringify([source.folderId, source.cardId]);
}
export function createClipboard(operation: TransferOperation, sources: readonly CardReference[]): CardClipboard | null {
  const unique = new Map(sources.map((source) => [referenceKey(source), { ...source }]));
  return unique.size ? { operation, sources: [...unique.values()] } : null;
}
export function visibleSelection(selection: readonly CardReference[], cards: readonly Card[]): CardReference[] {
  const visible = new Set(cards.map((card) => referenceKey(reference(card))));
  return selection.filter((source) => visible.has(referenceKey(source)));
}
export function applyTransfer(cards: Card[], operation: TransferOperation, result: TransferResult): Card[] {
  const removed = new Set(operation === 'move' ? result.entries.map((entry) => referenceKey(entry.source)) : []);
  const destinations = new Set(result.entries.map((entry) => referenceKey(reference(entry.card))));
  return [
    ...cards.filter(
      (card) => !removed.has(referenceKey(reference(card))) && !destinations.has(referenceKey(reference(card))),
    ),
    ...result.entries.map((entry) => entry.card),
  ];
}
export function reconcileClipboard(clipboard: CardClipboard | null, result: TransferResult): CardClipboard | null {
  if (!clipboard) return null;
  const replacements = new Map(result.entries.map((entry) => [referenceKey(entry.source), reference(entry.card)]));
  return createClipboard(
    clipboard.operation,
    clipboard.sources.map((source) => replacements.get(referenceKey(source)) ?? source),
  );
}
