import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  reference,
  referenceKey,
  createClipboard,
  visibleSelection,
  applyTransfer,
  reconcileClipboard,
  transferCards,
  executeTransfer,
} from '../src/features/cardtransfer/index.ts';

const first = { id: '1', folderId: 'first', items: [{ text: 'Original' }] };
const second = { id: '1', folderId: 'second', items: [] };

test('clipboard owns deduplicated references, and selection stays within visible cards', () => {
  const source = reference(first);
  const clipboard = createClipboard('copy', [source, source, reference(second)]);
  assert.equal(clipboard.sources.length, 2);
  source.cardId = 'changed';
  assert.equal(clipboard.sources[0].cardId, '1');
  assert.notEqual(referenceKey(reference(first)), referenceKey(reference(second)));
  assert.deepEqual(visibleSelection(clipboard.sources, [second]), [reference(second)]);
  assert.deepEqual(visibleSelection(clipboard.sources, []), []);
  assert.equal(createClipboard('copy', []), null);
});

test('paste resolves current cards on every call without consuming copy references', async () => {
  const clipboard = createClipboard('copy', [reference(first)]);
  let latest = first;
  let calls = 0;
  const port = {
    TransferCards: async (input) => {
      calls++;
      assert.deepEqual(input.sources, [reference(first)]);
      return {
        entries: [
          { source: input.sources[0], card: { ...latest, id: `copy-${calls}`, folderId: input.destinationFolderId } },
        ],
      };
    },
  };
  const pasted = await transferCards(port, { ...clipboard, destinationFolderId: 'second' });
  latest = { ...first, items: [{ text: 'Edited after copy' }] };
  const repeated = await transferCards(port, { ...clipboard, destinationFolderId: 'second' });
  assert.equal(repeated.entries[0].card.items[0].text, 'Edited after copy');
  assert.equal(applyTransfer([first, second], 'copy', pasted).length, 3);
  assert.deepEqual(clipboard.sources, [reference(first)]);
});

test('moves replace original identities, preserve neighboring cards, and reconcile clipboard references', () => {
  const moved = { ...first, id: 'moved', folderId: 'second' };
  const result = { entries: [{ source: reference(first), card: moved }] };
  assert.deepEqual(applyTransfer([first, second], 'move', result), [second, moved]);
  const clipboard = createClipboard('copy', [reference(first)]);
  assert.deepEqual(reconcileClipboard(clipboard, result).sources, [reference(moved)]);
  assert.equal(reconcileClipboard(null, result), null);
  assert.deepEqual(applyTransfer([first, second], 'move', { entries: [{ source: reference(second), card: second }] }), [
    first,
    second,
  ]);
});

test('failed or invalid pastes leave clipboard and card snapshots available for retry or cancellation', async () => {
  const clipboard = createClipboard('copy', [reference(first)]);
  const before = structuredClone(clipboard);
  const failure = new Error('source or folder no longer exists');
  const port = {
    TransferCards: async () => {
      throw failure;
    },
  };
  await assert.rejects(transferCards(port, { ...clipboard, destinationFolderId: 'second' }), failure);
  assert.deepEqual(clipboard, before);
  await assert.rejects(transferCards({}, { ...clipboard, destinationFolderId: '' }), /Choose cards/);
  await assert.rejects(
    transferCards({}, { operation: 'copy', sources: [], destinationFolderId: 'second' }),
    /Choose cards/,
  );
});

test('moves reconcile copied references only after success, while copies retain the clipboard', async () => {
  const clipboard = createClipboard('copy', [reference(first)]);
  const before = structuredClone(clipboard);
  const request = { ...clipboard, operation: 'move', destinationFolderId: 'second' };
  const result = { entries: [{ source: reference(first), card: { ...first, id: 'moved', folderId: 'second' } }] };
  const port = {
    TransferCards: async () => {
      throw new Error('failed');
    },
  };
  await assert.rejects(executeTransfer(port, { request, clipboard }), /failed/);
  assert.deepEqual(clipboard, before);
  port.TransferCards = async () => result;
  assert.deepEqual((await executeTransfer(port, { request, clipboard })).clipboard.sources, [
    reference(result.entries[0].card),
  ]);
  const copied = createClipboard('copy', clipboard.sources);
  assert.equal(
    (await executeTransfer(port, { request: { ...request, operation: 'copy' }, clipboard: copied })).clipboard,
    copied,
  );
});
