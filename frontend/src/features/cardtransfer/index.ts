export type {
  CardReference,
  CardClipboard,
  TransferInput,
  TransferEntry,
  TransferResult,
  TransferPort,
  TransferOperation,
} from './domain/types';
export {
  reference,
  referenceKey,
  createClipboard,
  visibleSelection,
  applyTransfer,
  reconcileClipboard,
} from './domain/funcs';
export { transferCards, executeTransfer } from './application/funcs';
