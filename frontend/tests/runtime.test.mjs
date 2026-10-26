import assert from 'node:assert/strict';
import { registerHooks } from 'node:module';
import { after, test } from 'node:test';

const adapter = new URL('../src/features/workspace/adapters/funcs.ts', import.meta.url).href;
const hooks = registerHooks({
  resolve(specifier, context, next) {
    if (context.parentURL !== adapter) return next(specifier, context);
    const sources = new Map([
      [
        '@wailsio/runtime',
        `export const Events = {
        On(name, callback) {
          globalThis.ravenRuntime.events.push({ name, callback });
          return () => { globalThis.ravenRuntime.unsubscribed++; };
        }
      };`,
      ],
      [
        '../../../../bindings/raven',
        `function transport(name, args = []) {
          globalThis.ravenRuntime.calls.push({ name, args });
          return globalThis.ravenRuntime.responses[name];
        }
        export const RavenService = {
        async GetConfiguration() {
          if (globalThis.ravenRuntime.failure) throw globalThis.ravenRuntime.failure;
          return JSON.stringify(globalThis.ravenRuntime.configuration);
        },
        async ListFolders() { return transport('ListFolders'); },
        async CreateFolder(input) { return transport('CreateFolder', [input]); },
        async UpdateFolder(input) { return transport('UpdateFolder', [input]); },
        async ExportFolders(input) { return transport('ExportFolders', [input]); },
        async ImportFolders(mode) { return transport('ImportFolders', [mode]); },
        async ListCards() { return transport('ListCards'); },
        async CreateCard(input) { return transport('CreateCard', [input]); },
        async UpdateCard(input) { return transport('UpdateCard', [input]); },
        async TransferCards(input) { return transport('TransferCards', [input]); },
        async ReviewItem(input) { return transport('ReviewItem', [input]); },
        async Notify(title, body) { return transport('Notify', [title, body]); },
        async SyncFloatingReview() { return transport('SyncFloatingReview'); },
        async SelectMediaFiles(kind) { return transport('SelectMediaFiles', [kind]); }
      };`,
      ],
    ]);
    const source = sources.get(specifier);
    return source
      ? { url: `data:text/javascript,${encodeURIComponent(source)}`, shortCircuit: true }
      : next(specifier, context);
  },
});

test('runtime JSON transport preserves requests, records, nullable lists, and parse failures', async (t) => {
  const folder = { id: 'folder', title: 'Ideas', description: '', tags: [] };
  const card = { id: 'card', folder_id: 'folder', created_at: 1, due_at: 2, last_reviewed: 0, items: [] };
  globalThis.ravenRuntime = {
    calls: [],
    responses: {
      ListFolders: JSON.stringify([folder]),
      CreateFolder: JSON.stringify(folder),
      UpdateFolder: JSON.stringify(folder),
      ListCards: JSON.stringify([card]),
      CreateCard: JSON.stringify(card),
      UpdateCard: JSON.stringify(card),
      ReviewItem: JSON.stringify(card),
      SelectMediaFiles: JSON.stringify([
        { source_path: '/test.png', original_name: 'test.png', byte_size: 8, preview_url: '/preview/test' },
      ]),
    },
  };
  t.after(() => {
    delete globalThis.ravenRuntime;
  });
  const { createRuntime } = await import('../src/features/workspace/adapters/funcs.ts');
  const runtime = createRuntime({ mediaRoutePrefix: '/media/', textKind: 'plain' });
  await runtime.Notify('Raven', 'تم نسخ بطاقتين');
  assert.deepEqual(globalThis.ravenRuntime.calls[0], { name: 'Notify', args: ['Raven', 'تم نسخ بطاقتين'] });
  await runtime.SyncFloatingReview();
  assert.deepEqual(globalThis.ravenRuntime.calls.at(-1), { name: 'SyncFloatingReview', args: [] });
  assert.deepEqual(await runtime.ListFolders(), [folder]);
  assert.deepEqual(await runtime.CreateFolder({ title: 'Ideas', description: '', tags: [] }), folder);
  const folderEdit = { ...folder, title: 'Updated ideas', description: 'Context', tags: ['edited'] };
  await runtime.UpdateFolder(folderEdit);
  assert.deepEqual(
    JSON.parse(globalThis.ravenRuntime.calls.find((call) => call.name === 'UpdateFolder').args[0]),
    folderEdit,
  );
  assert.equal((await runtime.ListCards())[0].folderId, 'folder');
  const input = { folderId: 'folder', items: [{ kind: 'text', text: 'Remember' }] };
  assert.equal((await runtime.CreateCard(input)).id, 'card');
  await runtime.UpdateCard({ ...input, cardId: 'card' });
  await runtime.ReviewItem({ folderId: 'folder', cardId: 'card', itemId: 'item', rating: 3 });
  assert.equal((await runtime.SelectMediaFiles('image'))[0].sourcePath, '/test.png');
  const create = globalThis.ravenRuntime.calls.find((call) => call.name === 'CreateCard');
  assert.deepEqual(JSON.parse(create.args[0]), { folder_id: 'folder', items: [{ kind: 'plain', text: 'Remember' }] });
  const update = globalThis.ravenRuntime.calls.find((call) => call.name === 'UpdateCard');
  assert.equal(JSON.parse(update.args[0]).card_id, 'card');
  const review = globalThis.ravenRuntime.calls.find((call) => call.name === 'ReviewItem');
  assert.deepEqual(JSON.parse(review.args[0]), { folder_id: 'folder', card_id: 'card', item_id: 'item', rating: 3 });
  globalThis.ravenRuntime.responses.ListCards = 'null';
  assert.deepEqual(await runtime.ListCards(), []);
  globalThis.ravenRuntime.responses.ListFolders = 'null';
  assert.deepEqual(await runtime.ListFolders(), []);
  globalThis.ravenRuntime.responses.SelectMediaFiles = 'null';
  assert.deepEqual(await runtime.SelectMediaFiles('image'), []);
  globalThis.ravenRuntime.responses.ListCards = '{invalid';
  await assert.rejects(runtime.ListCards(), /action_failed/);
  globalThis.ravenRuntime.responses.ListFolders = JSON.stringify([{ ...folder, id: 7 }]);
  await assert.rejects(runtime.ListFolders(), /action_failed/);
  globalThis.ravenRuntime.responses.ListCards = '{}';
  await assert.rejects(runtime.ListCards(), /action_failed/);
  globalThis.ravenRuntime.responses.ListCards = JSON.stringify([
    { ...card, items: [{ id: 'item', schedule: { due: 7 } }] },
  ]);
  await assert.rejects(runtime.ListCards(), /action_failed/);
  globalThis.ravenRuntime.responses.SelectMediaFiles = JSON.stringify([
    { source_path: false, original_name: 'test.png', byte_size: 8, preview_url: '/preview' },
  ]);
  await assert.rejects(runtime.SelectMediaFiles('image'), /action_failed/);
});
after(() => hooks.deregister());

test('folder archive transport sends complete selections and preserves import modes and cancellation', async (t) => {
  const response = {
    canceled: false,
    filename: 'folders.raven.zip',
    folder_ids: ['new'],
    folder_count: 2,
    card_count: 4,
    media_count: 3,
  };
  globalThis.ravenRuntime = {
    calls: [],
    responses: { ExportFolders: JSON.stringify(response), ImportFolders: JSON.stringify(response) },
  };
  t.after(() => {
    delete globalThis.ravenRuntime;
  });
  const { createRuntime } = await import('../src/features/workspace/adapters/funcs.ts');
  const runtime = createRuntime({ mediaRoutePrefix: '/media/', textKind: 'plain' });
  const exported = await runtime.ExportFolders(['first', 'second']);
  assert.equal(exported.folderCount, 2);
  assert.deepEqual(JSON.parse(globalThis.ravenRuntime.calls[0].args[0]), { folder_ids: ['first', 'second'] });
  for (const mode of ['fresh', 'resume']) assert.deepEqual((await runtime.ImportFolders(mode)).folderIds, ['new']);
  assert.deepEqual(
    globalThis.ravenRuntime.calls.slice(1).map((call) => call.args),
    [['fresh'], ['resume']],
  );
  globalThis.ravenRuntime.responses.ImportFolders = JSON.stringify({ ...response, canceled: true, folder_ids: null });
  assert.equal((await runtime.ImportFolders('fresh')).canceled, true);
});

test('runtime consumes configured event names and media routes and releases subscriptions', async (t) => {
  globalThis.ravenRuntime = {
    configuration: {
      localization_changed: 'custom:language',
      preferences_storage_key: 'custom.preferences',
      media_route_prefix: '/saved/',
      text_kind: 'plain',
      schedules_changed: 'custom:changed',
      show_reviews: 'custom:show',
      open_floating_review: 'custom:open',
      close_floating_review: 'custom:close',
    },
    events: [],
    unsubscribed: 0,
  };
  t.after(() => {
    delete globalThis.ravenRuntime;
  });
  const { createRuntime, loadConfiguration } = await import('../src/features/workspace/adapters/funcs.ts');
  const runtime = createRuntime(await loadConfiguration());
  assert.equal(runtime.configuration.preferencesStorageKey, 'custom.preferences');
  assert.equal(
    runtime.mediaURL({ folderId: 'folder space', cardId: 'card', id: 'item' }),
    '/saved/folder%20space/card/item',
  );
  const dispose = [
    runtime.events.onSchedulesChanged(() => {}),
    runtime.events.onShowReviews(() => {}),
    runtime.events.onOpenFloatingReview(() => {}),
    runtime.events.onCloseFloatingReview(() => {}),
  ];
  assert.deepEqual(
    globalThis.ravenRuntime.events.map((event) => event.name),
    ['custom:changed', 'custom:show', 'custom:open', 'custom:close'],
  );
  dispose.forEach((unsubscribe) => {
    unsubscribe();
  });
  assert.equal(globalThis.ravenRuntime.unsubscribed, 4);
  globalThis.ravenRuntime.failure = new Error('configuration unavailable');
  await assert.rejects(loadConfiguration(), /action_failed/);
});

test('card transfers serialize references and validate destination mappings', async (t) => {
  const card = { id: 'new-card', folder_id: 'destination', created_at: 1, due_at: 2, last_reviewed: 0, items: [] };
  globalThis.ravenRuntime = {
    calls: [],
    responses: {
      TransferCards: JSON.stringify({ entries: [{ source: { folder_id: 'source', card_id: 'card' }, card }] }),
    },
  };
  t.after(() => {
    delete globalThis.ravenRuntime;
  });
  const { createRuntime } = await import('../src/features/workspace/adapters/funcs.ts');
  const runtime = createRuntime({ mediaRoutePrefix: '/media/', textKind: 'text' });
  const result = await runtime.TransferCards({
    operation: 'move',
    sources: [{ folderId: 'source', cardId: 'card' }],
    destinationFolderId: 'destination',
  });
  assert.deepEqual(JSON.parse(globalThis.ravenRuntime.calls[0].args[0]), {
    operation: 'move',
    sources: [{ folder_id: 'source', card_id: 'card' }],
    destination_folder_id: 'destination',
  });
  assert.deepEqual(result.entries[0].source, { folderId: 'source', cardId: 'card' });
  assert.equal(result.entries[0].card.folderId, 'destination');
  for (const invalid of [
    { entries: [] },
    { entries: [{ source: {}, card }] },
    { entries: [{ source: { folder_id: 'source', card_id: 'card' }, card: {} }] },
  ]) {
    globalThis.ravenRuntime.responses.TransferCards = JSON.stringify(invalid);
    await assert.rejects(
      runtime.TransferCards({ operation: 'copy', sources: [], destinationFolderId: 'destination' }),
      /action_failed/,
    );
  }
});
