import type { Card } from '../../cards';

export interface CardReference {
  folderId: string;
  cardId: string;
}
export type TransferOperation = 'copy' | 'move';
export interface CardClipboard {
  operation: TransferOperation;
  sources: CardReference[];
}
export interface TransferInput extends CardClipboard {
  destinationFolderId: string;
}
export interface TransferEntry {
  source: CardReference;
  card: Card;
}
export interface TransferResult {
  entries: TransferEntry[];
}
export interface TransferPort {
  TransferCards(input: TransferInput): Promise<TransferResult>;
}
