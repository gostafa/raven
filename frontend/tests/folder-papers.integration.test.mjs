import assert from 'node:assert/strict';
import { test } from 'node:test';
import { selectFolderPapers } from '../src/features/folders/index.ts';

const item = (kind, position, hint = false) => ({ kind, position, hint });
const card = (id, createdAt, items, folderId = 'chosen') => ({ id, createdAt, items, folderId });

test('folder papers select actual visual items in newest-card and item-position order', () => {
  const cards = [
    card('older', 1, [item('image', 0)]),
    card('newer', 2, [item('video', 2), item('text', 0), item('image', 1), item('audio', 3)]),
    card('elsewhere', 3, [item('video', 0)], 'other'),
  ];
  const before = structuredClone(cards);
  assert.deepEqual(selectFolderPapers(cards, 'chosen'), [{ kind: 'image' }, { kind: 'video' }, { kind: 'image' }]);
  assert.deepEqual(cards, before);
});

test('hints do not consume paper slots, and at most four eligible papers appear', () => {
  const cards = [
    card('one', 1, [
      item('image', 0, true),
      item('text', 1),
      item('video', 2),
      item('image', 3),
      item('video', 4),
      item('image', 5),
      item('video', 6),
    ]),
  ];
  assert.deepEqual(selectFolderPapers(cards, 'chosen'), [
    { kind: 'video' },
    { kind: 'image' },
    { kind: 'video' },
    { kind: 'image' },
  ]);
});

test('empty folders, missing folders, and folders without visual media have no papers', () => {
  assert.deepEqual(selectFolderPapers([], 'chosen'), []);
  assert.deepEqual(selectFolderPapers([card('one', 1, [item('image', 0)])], 'missing'), []);
  assert.deepEqual(
    selectFolderPapers([card('one', 1, [item('text', 0), item('audio', 1), item('image', 2, true)])], 'chosen'),
    [],
  );
});

test('cards with the same creation time use a stable identifier order', () => {
  assert.deepEqual(selectFolderPapers([card('b', 1, [item('video', 0)]), card('a', 1, [item('image', 0)])], 'chosen'), [
    { kind: 'image' },
    { kind: 'video' },
  ]);
});
