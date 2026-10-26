import assert from 'node:assert/strict';
import { test } from 'node:test';
import { loadWorkspace, saveCard, deleteSelection, reviewSelection } from '../src/features/workspace/index.ts';
import { saveReview, loadDueReview } from '../src/features/reviews/index.ts';

const folder = { id: 'folder', title: 'Ideas', description: '', tags: [] };
const workspace = { folders: [], selectedFolderId: null, sidebarOpen: true };
const card = {
  id: 'card',
  folderId: 'folder',
  createdAt: 100,
  dueAt: 100,
  lastReviewed: 0,
  items: [{ id: 'item', schedule: { Due: new Date(100).toISOString() } }],
};

test('review now includes future items only from selected cards and excludes hints', () => {
  const future = {
    ...card,
    folderId: 'other-folder',
    items: [
      { id: 'future', schedule: { Due: new Date(9_000_000_000_000).toISOString() } },
      { id: 'hint', hint: true, schedule: { Due: new Date(0).toISOString() } },
    ],
  };
  const selection = [{ folderId: future.folderId, cardId: future.id }];
  const before = structuredClone([card, future, selection]);
  const queue = reviewSelection([card, future], selection);
  assert.equal(queue.length, 1);
  assert.equal(queue[0].card, future);
  assert.equal(queue[0].item.id, 'future');
  assert.deepEqual(reviewSelection([card, future], []), []);
  assert.deepEqual(reviewSelection([card, future], [{ folderId: 'missing', cardId: card.id }]), []);
  assert.equal(reviewSelection([card, future], [...selection, { folderId: card.folderId, cardId: card.id }]).length, 2);
  assert.deepEqual([card, future, selection], before);
});

test('loading failures leave the caller snapshot unchanged and allow retry', async () => {
  let failed = true;
  const ports = {
    now: () => 200,
    ListCards: async () => [card],
    ListFolders: async () => {
      if (failed) throw new Error('offline');
      return [folder];
    },
  };
  await assert.rejects(loadWorkspace(ports, workspace), /offline/);
  assert.deepEqual(workspace.folders, []);
  failed = false;
  assert.deepEqual((await loadWorkspace(ports, workspace)).cards, [card]);
});

test('card save retries reuse an already-created folder after a failed card write', async () => {
  let creates = 0;
  const input = {
    folderId: '__new',
    newFolderTitle: 'Ideas',
    editing: null,
    items: [{ kind: 'text', text: 'Remember' }],
  };
  const ports = {
    CreateFolder: async () => {
      creates++;
      return folder;
    },
    CreateCard: async () => {
      throw new Error('disk full');
    },
  };
  await assert.rejects(
    saveCard(ports, input, (created) => {
      input.folderId = created.id;
    }),
    /disk full/,
  );
  ports.CreateCard = async (saved) => {
    assert.equal(saved.folderId, folder.id);
    return card;
  };
  assert.equal(await saveCard(ports, input, () => assert.fail('folder recreated')), card);
  assert.equal(creates, 1);
});

test('failed reviews do not mutate the session entry and can be retried', async () => {
  const entry = { card, item: card.items[0], dueAt: 100 };
  const before = structuredClone(entry);
  const port = {
    ReviewItem: async () => {
      throw new Error('unavailable');
    },
  };
  await assert.rejects(saveReview(port, entry, 3), /unavailable/);
  assert.deepEqual(entry, before);
  port.ReviewItem = async (...args) => {
    assert.deepEqual(args, [{ folderId: 'folder', cardId: 'card', itemId: 'item', rating: 3 }]);
    return card;
  };
  assert.equal(await saveReview(port, entry, 3), card);
});

test('review loading uses the supplied clock and deletion awaits persistence', async () => {
  const result = await loadDueReview(
    { ListCards: async () => [card] },
    { ListFolders: async () => [folder] },
    () => 100,
  );
  assert.equal(result.entries.length, 1);
  assert.equal(
    (await loadDueReview({ ListCards: async () => [card] }, { ListFolders: async () => [] }, () => 99)).entries.length,
    0,
  );
  await assert.rejects(
    deleteSelection(
      {
        DeleteCard: async () => {
          throw new Error('failed');
        },
      },
      { folderId: 'folder', cardId: 'card' },
    ),
    /failed/,
  );
});
