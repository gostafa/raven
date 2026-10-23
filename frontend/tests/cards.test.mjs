/* eslint-disable security/detect-non-literal-fs-filename -- Paths are fixed repository assets or local module-resolution candidates. */
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { compile } from 'svelte/compiler';
import {
  cardPreview,
  cardProgress,
  editItems,
  hasContent,
  matchesCard,
  createMediaURL,
  moveItem,
  saveItems,
  orderedItems,
  textDraft,
} from '../src/features/cards/index.ts';

import { stopMedia } from '../src/features/ui/index.ts';
import { reviewItems, shuffledReviewItems, lastUserReview } from '../src/features/reviews/index.ts';
import { mergeCardSnapshot } from '../src/features/cards/index.ts';

const card = {
  id: 'card',
  folderId: 'folder',
  createdAt: 100,
  dueAt: 200,
  lastReviewed: 0,
  stage: 0,
  items: [
    { id: 't1', folderId: 'folder', cardId: 'card', position: 0, kind: 'text', text: 'Useful information' },
    { id: 'i1', folderId: 'folder', cardId: 'card', position: 1, kind: 'image', originalName: 'diagram.png' },
    { id: 't2', folderId: 'folder', cardId: 'card', position: 2, kind: 'text', text: 'More context' },
    { id: 'a1', folderId: 'folder', cardId: 'card', position: 3, kind: 'audio', originalName: 'Pronunciation.wav' },
  ],
};

test('card progress measures manual recall and excludes hints and automatic reviews', () => {
  const review = (Rating, automatic = false) => ({ automatic, log: { Rating } });
  assert.equal(cardProgress({ ...card, items: [] }), 0);
  const items = [
    { reviews: [review(1), review(2), review(3), review(4), review(1, true)] },
    { hint: true, reviews: [review(1)] },
  ];
  assert.equal(cardProgress({ ...card, items }), 75);
  assert.equal(cardProgress({ ...card, items: [{ reviews: [review(1)] }] }), 0);
  assert.equal(cardProgress({ ...card, items: [{ reviews: [review(4)] }] }), 100);
  assert.equal(cardProgress({ ...card, items: [{ reviews: [review(1, true)] }] }), 0);
});

test('card progress weights items equally and includes unreviewed items', () => {
  const review = (Rating) => ({ automatic: false, log: { Rating } });
  const successful = { reviews: Array.from({ length: 20 }, () => review(3)) };
  const failed = { reviews: [review(1)] };
  assert.equal(cardProgress({ ...card, items: [successful, failed] }), 50);
  assert.equal(cardProgress({ ...card, items: [successful, { reviews: [] }] }), 50);
  assert.equal(cardProgress({ ...card, items: [successful, { reviews: [] }, { reviews: [] }, { reviews: [] }] }), 25);
  assert.equal(cardProgress({ ...card, items: [{ ...successful, hint: true }] }), 0);
});

test('card progress ignores invalid ratings and rounds only after averaging', () => {
  const reviews = (ratings) => ratings.map((Rating) => ({ automatic: false, log: { Rating } }));
  assert.equal(cardProgress({ ...card, items: [{ reviews: reviews([0, 5, 1.5, NaN, Infinity, 3]) }] }), 100);
  assert.equal(cardProgress({ ...card, items: [{ reviews: reviews([1, 1, 3]) }, { reviews: reviews([1, 3]) }] }), 42);
});

test('a refresh preceding the save response never duplicates a saved card', () => {
  const sibling = { ...card, folderId: 'another-folder' };
  const refreshed = [{ ...card }, sibling];
  const result = mergeCardSnapshot(refreshed, card);
  assert.equal(result.length, 2);
  assert.equal(result[0], card);
  assert.equal(result[1], sibling);
  assert.notEqual(refreshed[0], card);
  assert.deepEqual(mergeCardSnapshot([sibling], card), [sibling, card]);
});

test('editing keeps IDs and does not mutate saved cards', () => {
  const drafts = editItems(card, createMediaURL('/media/'));
  drafts[0].text = 'Changed';
  assert.equal(card.items[0].text, 'Useful information');
  assert.equal(drafts[1].id, 'i1');
  assert.equal(drafts[1].originalName, 'diagram.png');
  assert.equal(drafts[1].previewURL, '/media/folder/card/i1');
  assert.equal('previewURL' in saveItems(drafts)[1], false);
  assert.equal(saveItems(drafts)[1].position, undefined);
  assert.equal(saveItems(drafts)[1].relativePath, undefined);
  assert.equal('side' in saveItems(drafts)[1], false);
});

test('all mixed items reorder together and preserve their identities', () => {
  const drafts = editItems(card, createMediaURL('/media/'));
  const moved = moveItem(drafts, 'a1', -1);
  assert.deepEqual(
    moved.map((item) => item.id),
    ['t1', 'i1', 'a1', 't2'],
  );
  assert.deepEqual(moveItem(moved, 'a1', 1), drafts);
  assert.deepEqual(moveItem(drafts, 't1', -1), drafts);
  assert.deepEqual(moveItem(drafts, 'a1', 1), drafts);
});

test('one text block or one media file is enough; empty cards are rejected', () => {
  const image = { key: 'image', kind: 'image', sourcePath: '/selected/image.png' };
  const audio = { key: 'audio', id: 'a1', kind: 'audio' };
  assert.equal(hasContent(saveItems([textDraft('A useful fact', crypto.randomUUID())])), true);
  assert.equal(hasContent(saveItems([image])), true);
  assert.equal(hasContent(saveItems([audio])), true);
  assert.equal(saveItems([image, textDraft('  ', crypto.randomUUID())]).length, 1);
  assert.equal(hasContent(saveItems([textDraft('  ', crypto.randomUUID())])), false);
  assert.equal(hasContent([]), false);
  assert.equal(hasContent([{ kind: 'audio' }]), false);
});

test('search and previews include the entire content collection', () => {
  for (const query of ['MORE CONTEXT', 'diagram', 'pronunciation', 'useful', 'biology']) {
    assert.equal(matchesCard(card, 'Biology', query), true, query);
  }
  assert.equal(matchesCard(card, 'Biology', 'unrelated'), false);
  assert.equal(cardPreview(card), 'Useful information · diagram.png · More context · Pronunciation.wav');
});

test('content renders in one stored order', () => {
  const shuffled = { ...card, items: [...card.items].reverse() };
  assert.deepEqual(
    orderedItems(shuffled).map((item) => item.id),
    ['t1', 'i1', 't2', 'a1'],
  );
  assert.equal(createMediaURL('/media/')(card.items[1]), '/media/folder/card/i1');
});

test('leaving a card pauses all mounted audio and video', () => {
  let paused = 0;
  stopMedia({
    querySelectorAll(selector) {
      assert.equal(selector, 'audio, video');
      return [
        {
          pause() {
            paused++;
          },
        },
        {
          pause() {
            paused++;
          },
        },
      ];
    },
  });
  assert.equal(paused, 2);
  assert.doesNotThrow(() => stopMedia(undefined));
});

test('the editor has one information fieldset and both review views share one item renderer', async () => {
  const editor = await readFile(new URL('../src/features/cards/components/CardEditor.svelte', import.meta.url), 'utf8');
  const source = await readFile(
    new URL('../src/features/workspace/components/Workspace.svelte', import.meta.url),
    'utf8',
  );
  const review = await readFile(
    new URL('../src/features/reviews/components/ReviewSession.svelte', import.meta.url),
    'utf8',
  );
  const floating = await readFile(
    new URL('../src/features/reviews/components/FloatingReview.svelte', import.meta.url),
    'utf8',
  );
  const { ast } = compile(editor, { modernAst: true, generate: false });
  const fieldsets = [];
  function visit(node) {
    if (!node || typeof node !== 'object') return;
    if (node.type === 'RegularElement' && node.name === 'fieldset') fieldsets.push(node);
    for (const value of Object.values(node)) {
      if (Array.isArray(value)) value.forEach(visit);
      else if (value && typeof value === 'object') visit(value);
    }
  }
  visit(ast.fragment);
  assert.equal(fieldsets.length, 1);
  assert.match(editor, /<legend>\{\$l.t\('cards.what_you_want_to_remember'\)\}<\/legend>/);
  assert.doesNotMatch(editor, /Question or prompt|Front of card|Back of card/);
  assert.equal((review.match(/<CardContent\b/g) ?? []).length, 2);
  assert.match(source, /<ReviewSession\b/);
  assert.match(floating, /<ReviewSession\b/);
  assert.doesNotMatch(review, /revealed|Reveal answer|side="/);
});

test('reviews flatten content into independent schedules with stable due order', () => {
  const scheduled = {
    ...card,
    items: card.items.map((item, index) => ({
      ...item,
      schedule: { Due: new Date(index < 2 ? 1000 : 2000).toISOString() },
      reviews: [],
    })),
  };
  const entries = reviewItems([scheduled]);
  assert.equal(entries.length, 4);
  assert.deepEqual(
    entries.map(({ item }) => item.id),
    ['i1', 't1', 'a1', 't2'],
  );
  assert.equal(entries[0].card, scheduled);
  assert.equal(entries[0].dueAt, 1000);
});

test('hint roles survive editing and saving and are excluded from review queues', () => {
  const mixed = {
    ...card,
    items: card.items.map((item, index) => ({
      ...item,
      hint: index > 0,
      schedule: { Due: new Date(1000).toISOString() },
      reviews: [],
    })),
  };
  const drafts = editItems(mixed, () => '/media');
  assert.deepEqual(
    saveItems(drafts).map((item) => item.hint),
    [false, true, true, true],
  );
  assert.deepEqual(
    reviewItems([mixed]).map(({ item }) => item.id),
    ['t1'],
  );
  assert.equal(reviewItems([{ ...mixed, items: mixed.items.map((item) => ({ ...item, hint: true })) }]).length, 0);
});

test('full review shuffles individual parts across cards and excludes hints without changing stored order', () => {
  const makeCard = (id) => ({
    ...card,
    id,
    items: ['a', 'b', 'hint'].map((part) => ({
      id: part,
      hint: part === 'hint',
      schedule: { Due: new Date(1000).toISOString() },
    })),
  });
  const cards = [makeCard('first'), makeCard('second')];
  const original = structuredClone(cards);
  const entries = shuffledReviewItems(cards, () => 0);
  assert.deepEqual(
    entries.map(({ card, item }) => `${card.id}/${item.id}`),
    ['first/b', 'second/a', 'second/b', 'first/a'],
  );
  assert.deepEqual(cards, original);
  assert.deepEqual(shuffledReviewItems([]), []);
});

test('automatic Again initialization does not count as a user review', () => {
  const item = { ...card.items[0], reviews: [{ automatic: true, log: { Review: new Date(1000).toISOString() } }] };
  assert.equal(lastUserReview(item), 0);
  item.reviews.push({ automatic: false, log: { Review: new Date(2000).toISOString() } });
  assert.equal(lastUserReview(item), 2000);
});
