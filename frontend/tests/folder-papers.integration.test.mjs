import assert from 'node:assert/strict';
import { test } from 'node:test';
import { selectFolderPapers } from '../src/features/folders/index.ts';

const item = (id, kind, due, position = 0, hint = false) => ({
  id,
  kind,
  position,
  hint,
  schedule: { Due: due },
});
const card = (id, items, folderId = 'chosen', createdAt = 0) => ({ id, items, folderId, createdAt });
const ids = (cards) => selectFolderPapers(cards, 'chosen').map((paper) => paper.item.id);

test('visual previews use earliest item review dates, including overdue items, rather than card creation dates', () => {
  const cards = [
    card('new', [item('later', 'image', '2026-12-01')], 'chosen', 100),
    card('old', [item('soon', 'video', '2026-11-01'), item('overdue', 'image', '2026-01-01')], 'chosen', 1),
    card('elsewhere', [item('other', 'image', '2025-01-01')], 'other'),
  ];
  const before = structuredClone(cards);
  assert.deepEqual(ids(cards), ['overdue', 'soon', 'later']);
  const [paper] = selectFolderPapers(cards, 'chosen');
  assert.equal(paper.item, cards[1].items[1]);
  assert.equal(paper.kind, 'image');
  assert.deepEqual(cards, before);
});

test('hints and nonvisual items do not consume the five preview slots', () => {
  const cards = [
    card('one', [
      item('hint', 'image', '2026-01-01', 0, true),
      item('text', 'text', '2026-01-01'),
      item('audio', 'audio', '2026-01-01'),
      ...['video', 'image', 'video', 'image', 'video', 'image'].map((kind, i) =>
        item(String(i), kind, '2026-01-02', i),
      ),
    ]),
  ];
  assert.deepEqual(ids(cards), ['0', '1', '2', '3', '4']);
});

test('five separate video cards appear in review order rather than only the first video', () => {
  const cards = Array.from({ length: 6 }, (_, index) =>
    card(`card-${index}`, [item(`video-${index}`, 'video', `2026-01-0${6 - index}`)]),
  );
  const papers = selectFolderPapers(cards, 'chosen');
  assert.deepEqual(
    papers.map((paper) => paper.item.id),
    ['video-5', 'video-4', 'video-3', 'video-2', 'video-1'],
  );
  assert.equal(
    papers.every((paper) => paper.kind === 'video'),
    true,
  );
  assert.equal(new Set(papers.map((paper) => paper.key)).size, 5);
});

test('invalid schedules sort after valid review dates with deterministic ties', () => {
  const cards = [
    card('b', [item('b', 'video', 'invalid')]),
    card('a', [item('a2', 'image', '', 2), item('a1', 'video', '', 1)]),
    card('future', [item('valid', 'image', '2099-01-01')]),
  ];
  assert.deepEqual(ids(cards), ['valid', 'a1', 'a2', 'b']);
});

test('equal review dates use card id, item position, and item id as stable tiebreakers', () => {
  const due = '2026-01-01';
  const cards = [
    card('b', [item('b', 'image', due)]),
    card('a', [item('z', 'image', due, 1), item('y', 'video', due, 1), item('x', 'image', due)]),
  ];
  assert.deepEqual(ids(cards), ['x', 'y', 'z', 'b']);
});

test('preview keys remain unique when different cards reuse an item identifier', () => {
  const papers = selectFolderPapers(
    [card('a', [item('same', 'image', '2026-01-01')]), card('b', [item('same', 'video', '2026-01-01')])],
    'chosen',
  );
  assert.equal(new Set(papers.map((paper) => paper.key)).size, 2);
});

test('empty, missing, and nonvisual folders have no placeholder papers', () => {
  assert.deepEqual(selectFolderPapers([], 'chosen'), []);
  assert.deepEqual(selectFolderPapers([card('one', [item('image', 'image', '2026-01-01')])], 'missing'), []);
  assert.deepEqual(
    ids([
      card('one', [
        item('text', 'text', '2026-01-01'),
        item('audio', 'audio', '2026-01-01'),
        item('hint', 'image', '2026-01-01', 0, true),
      ]),
    ]),
    [],
  );
});
