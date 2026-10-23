import assert from 'node:assert/strict';
import { test } from 'node:test';
import { matchesCard, selectCards } from '../src/features/cards/index.ts';

const now = 1000;
const review = (automatic = false, Rating = 3) => ({ automatic, log: { Rating } });
const item = (kind, due, reviews = [], hint = false) => ({
  kind,
  hint,
  reviews,
  schedule: { Due: due === null ? 'invalid' : new Date(due).toISOString() },
  text: kind === 'text' ? 'Biology notes' : undefined,
});
const card = (id, createdAt, items, folderId = 'folder') => ({ id, createdAt, items, folderId });
const ids = (cards) => cards.map((entry) => entry.id);
const select = (cards, options = {}) =>
  selectCards(cards, {
    sort: 'created-desc',
    kinds: [],
    ...options,
  });

test('review sorting uses earliest valid non-hint schedules and places missing schedules last', () => {
  const cards = [
    card('due', 1, [item('text', now), item('audio', now + 100)]),
    card('upcoming', 2, [item('text', now + 1), item('image', 0, [], true)]),
    card('invalid', 3, [item('text', null)]),
    card('hints', 4, [item('image', 0, [], true)]),
    card('empty', 5, []),
  ];
  assert.deepEqual(ids(select(cards, { sort: 'review-asc' })), ['due', 'upcoming', 'empty', 'hints', 'invalid']);
  assert.deepEqual(ids(select(cards, { sort: 'review-desc' })), ['upcoming', 'due', 'empty', 'hints', 'invalid']);
});

test('content types match any selection including hints, combined with search and folder scope', () => {
  const cards = [
    card('mixed', 1, [item('text', 0), item('image', 0, [], true)]),
    card('audio', 2, [item('audio', 0)]),
    card('video', 3, [item('video', 2000)]),
    card('other-folder', 4, [item('text', 0), item('image', 0)], 'other'),
  ];
  assert.deepEqual(ids(select(cards, { kinds: ['image', 'audio'] })), ['other-folder', 'audio', 'mixed']);
  assert.deepEqual(ids(select(cards, { kinds: ['video'] })), ['video']);
  const matching = cards.filter((entry) => entry.folderId === 'folder' && matchesCard(entry, 'Science', 'biology'));
  assert.deepEqual(ids(select(matching, { kinds: ['image'] })), ['mixed']);
  assert.deepEqual(select(matching, { kinds: ['audio'] }), []);
  assert.equal(select(cards).length, cards.length);
});

test('all sort directions use deterministic ties and put missing schedules last', () => {
  const cards = [
    card('a', 10, [item('text', 100, [review(false, 1)])]),
    card('b', 20, [item('text', 200, [review()])]),
    card('c', 30, [item('text', null, [review(false, 1), review()])]),
  ];
  assert.deepEqual(ids(select(cards)), ['c', 'b', 'a']);
  assert.deepEqual(ids(select(cards, { sort: 'created-asc' })), ['a', 'b', 'c']);
  assert.deepEqual(ids(select(cards, { sort: 'review-asc' })), ['a', 'b', 'c']);
  assert.deepEqual(ids(select(cards, { sort: 'review-desc' })), ['b', 'a', 'c']);
  assert.deepEqual(ids(select(cards, { sort: 'progress-asc' })), ['a', 'c', 'b']);
  assert.deepEqual(ids(select(cards, { sort: 'progress-desc' })), ['b', 'c', 'a']);
  const ties = [card('b', 1, []), card('a', 1, []), card('new', 2, []), card('first-folder', 1, [], 'a')];
  for (const sort of ['created-desc', 'review-asc', 'review-desc', 'progress-asc', 'progress-desc']) {
    assert.deepEqual(ids(select(ties, { sort })), ['new', 'first-folder', 'a', 'b']);
  }
});

test('selection leaves input cards, items, and selection unchanged', () => {
  const cards = [card('b', 2, [item('text', 2)]), card('a', 1, [item('image', 1)])];
  const options = { sort: 'review-asc', kinds: ['image', 'text'] };
  const original = structuredClone({ cards, options });
  selectCards(cards, options);
  assert.deepEqual({ cards, options }, original);
  assert.deepEqual(selectCards([], options), []);
});

test('custom recall ranges include both boundaries, support exact values, and combine with content filters', () => {
  const percentages = [0, 19, 20, 37, 63, 64, 80, 100];
  const cards = percentages.map((percentage) =>
    card(String(percentage), percentage, [
      item(
        'text',
        now,
        Array.from({ length: 100 }, (_, index) => review(false, index < percentage ? 3 : 1)),
      ),
    ]),
  );
  for (const [minimum, maximum, expected] of [
    [20, 63, ['63', '37', '20']],
    [0, 0, ['0']],
    [37, 37, ['37']],
    [80, 100, ['100', '80']],
    [100, 100, ['100']],
    [21, 36, []],
  ]) {
    assert.deepEqual(ids(select(cards, { progressRange: { minimum, maximum } })), expected);
  }
  assert.deepEqual(ids(select(cards, { progressRange: { minimum: 20, maximum: 63 }, sort: 'progress-asc' })), [
    '20',
    '37',
    '63',
  ]);
  assert.deepEqual(select(cards, { progressRange: { minimum: 20, maximum: 63 }, kinds: ['audio'] }), []);
  assert.deepEqual(select(cards, { progressRange: { minimum: 0, maximum: 100 } }), select(cards));
});

test('percentage filtering uses displayed recall including unreviewed items and excluding hints and automatic reviews', () => {
  const cards = [
    card('unreviewed', 1, [item('text', now)]),
    card('mixed', 2, [
      item('text', now, [review(), review(true, 1)]),
      item('text', now),
      item('image', now, [review(false, 1)], true),
    ]),
    card('hints-only', 3, [item('image', now, [review()], true)]),
  ];
  assert.deepEqual(ids(select(cards, { progressRange: { minimum: 0, maximum: 0 } })), ['hints-only', 'unreviewed']);
  assert.deepEqual(ids(select(cards, { progressRange: { minimum: 50, maximum: 50 }, kinds: ['image'] })), ['mixed']);
});
