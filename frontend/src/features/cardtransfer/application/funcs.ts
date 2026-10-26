import { createClipboard, reconcileClipboard } from '../domain/funcs';
import type { CardClipboard, TransferInput, TransferPort, TransferResult } from '../domain/types';

export async function transferCards(port: TransferPort, input: TransferInput): Promise<TransferResult> {
  const clipboard = createClipboard(input.operation, input.sources);
  if (!clipboard || !input.destinationFolderId || !['copy', 'move'].includes(input.operation)) {
    throw new Error('Choose cards, a destination folder, and copy or move.');
  }
  return await port.TransferCards({ ...clipboard, destinationFolderId: input.destinationFolderId });
}

export async function executeTransfer(
  port: TransferPort,
  input: { request: TransferInput; clipboard: CardClipboard | null },
): Promise<{ result: TransferResult; clipboard: CardClipboard | null }> {
  const result = await transferCards(port, input.request);
  const clipboard = input.request.operation === 'move' ? reconcileClipboard(input.clipboard, result) : input.clipboard;
  return { result, clipboard };
}
