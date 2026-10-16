import assert from 'node:assert/strict';
import { test } from 'node:test';
import { selectMedia } from '../src/features/cards/application/funcs.ts';

test('media selection appends frontend drafts without mutating existing items', async () => {
  const items = [{ key: 'text', kind: 'text', text: 'Remember' }];
  const port = {
    newKey: () => 'selected',
    SelectMediaFiles: async (kind) => {
      assert.equal(kind, 'image');
      return [
        {
          sourcePath: '/selected/image.png',
          originalName: 'image.png',
          previewURL: '/media-preview/token',
          byteSize: 10,
        },
      ];
    },
  };
  const result = await selectMedia(port, 'image', items);
  assert.equal(items.length, 1);
  assert.deepEqual(result[1], {
    key: 'selected',
    kind: 'image',
    sourcePath: '/selected/image.png',
    originalName: 'image.png',
    previewURL: '/media-preview/token',
  });
});

test('picker failures and cancellation leave the current draft intact', async () => {
  const items = [{ key: 'text', kind: 'text', text: 'Remember' }];
  const before = structuredClone(items);
  await assert.rejects(
    selectMedia(
      {
        SelectMediaFiles: async () => {
          throw new Error('picker unavailable');
        },
      },
      'audio',
      items,
    ),
    /picker unavailable/,
  );
  assert.deepEqual(items, before);
  const result = await selectMedia(
    { SelectMediaFiles: async () => [], newKey: () => assert.fail('cancelled selection creates no keys') },
    'video',
    items,
  );
  assert.deepEqual(result, items);
});
