import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  selectFolders,
  emptyWorkspace,
  loadPreferences,
  parseTags,
  reconcileFolders,
  savePreferences,
} from '../src/features/folders/index.ts';

const preferencesStorageKey = 'raven.preferences.v1';

test('preferences use the injected key without writing the default key', () => {
  const storage = memoryStorage();
  const preferences = { selectedFolderId: 'chosen', sidebarOpen: false };
  savePreferences(storage, preferences, 'custom.preferences');
  assert.deepEqual(loadPreferences(storage, 'custom.preferences'), preferences);
  assert.equal(storage.getItem(preferencesStorageKey), null);
});

function memoryStorage(initial = null) {
  const values = new Map(initial === null ? [] : [[preferencesStorageKey, initial]]);
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
}

const folder = { id: 'first', title: 'Ideas', description: 'Things to explore', tags: ['Personal'] };

test('a first launch starts with an open sidebar and no folders', () => {
  assert.deepEqual(
    { ...emptyWorkspace(), ...loadPreferences(memoryStorage(), preferencesStorageKey) },
    {
      folders: [],
      selectedFolderId: null,
      sidebarOpen: true,
    },
  );
});

test('only selection and sidebar preference are saved in localStorage', () => {
  const storage = memoryStorage();
  const workspace = {
    folders: [folder, { id: 'second', title: 'Untitled ideas', description: '', tags: [] }],
    selectedFolderId: 'second',
    sidebarOpen: false,
  };
  savePreferences(storage, workspace, preferencesStorageKey);
  const preferences = { selectedFolderId: 'second', sidebarOpen: false };
  assert.deepEqual(loadPreferences(storage, preferencesStorageKey), preferences);
  assert.equal(storage.getItem(preferencesStorageKey), JSON.stringify(preferences));
  assert.equal(storage.getItem('raven.workspace.v1'), null);
  assert.deepEqual(
    reconcileFolders({ ...emptyWorkspace(), ...loadPreferences(storage, preferencesStorageKey) }, workspace.folders),
    workspace,
  );
});

test('comma-separated tags ignore whitespace, empty entries and case-insensitive duplicates', () => {
  assert.deepEqual(parseTags(' ideas, , Personal,ideas, personal , 新しい, '), ['ideas', 'Personal', '新しい']);
  assert.deepEqual(parseTags(' , , '), []);
});

test('Home stays selected after reloading a workspace that contains folders', () => {
  const storage = memoryStorage();
  const workspace = { folders: [folder], selectedFolderId: null, sidebarOpen: true };
  savePreferences(storage, workspace, preferencesStorageKey);
  assert.deepEqual(
    reconcileFolders({ ...emptyWorkspace(), ...loadPreferences(storage, preferencesStorageKey) }, [folder]),
    workspace,
  );
});

test('an outdated selection falls back to Home after folders are loaded', () => {
  for (const folders of [[folder], []]) {
    const storage = memoryStorage(JSON.stringify({ selectedFolderId: 'missing', sidebarOpen: true }));
    const workspace = { ...emptyWorkspace(), ...loadPreferences(storage, preferencesStorageKey) };
    assert.equal(workspace.selectedFolderId, 'missing');
    assert.equal(reconcileFolders(workspace, folders).selectedFolderId, null);
  }
});

test('malformed or invalid preferences are rejected without overwriting them', () => {
  const corruptValues = [
    '{invalid',
    'null',
    '[]',
    '{}',
    JSON.stringify({ selectedFolderId: 123, sidebarOpen: true }),
    JSON.stringify({ selectedFolderId: null, sidebarOpen: 'false' }),
  ];
  for (const raw of corruptValues) {
    const storage = memoryStorage(raw);
    assert.throws(() => loadPreferences(storage, preferencesStorageKey));
    assert.equal(storage.getItem(preferencesStorageKey), raw);
  }
});

test('read and write failures reach the caller so the UI can report them', () => {
  assert.throws(
    () =>
      loadPreferences(
        {
          getItem() {
            throw new Error('Storage unavailable');
          },
        },
        preferencesStorageKey,
      ),
    /Storage unavailable/,
  );
  assert.throws(
    () =>
      savePreferences(
        {
          setItem() {
            throw new Error('Quota exceeded');
          },
        },
        emptyWorkspace(),
        preferencesStorageKey,
      ),
    /Quota exceeded/,
  );
});

test('legacy workspace folders and preferences are ignored and left untouched', () => {
  const storage = memoryStorage();
  const legacy = JSON.stringify({ folders: [folder], selectedFolderId: folder.id, sidebarOpen: false });
  storage.setItem('raven.workspace.v1', legacy);
  assert.deepEqual(loadPreferences(storage, preferencesStorageKey), { selectedFolderId: null, sidebarOpen: true });
  const workspace = reconcileFolders({ ...emptyWorkspace(), ...loadPreferences(storage, preferencesStorageKey) }, []);
  savePreferences(storage, workspace, preferencesStorageKey);
  assert.deepEqual(workspace, emptyWorkspace());
  assert.equal(storage.getItem('raven.workspace.v1'), legacy);
});

test('folder filters match every selected tag without case sensitivity', () => {
  const folders = [folder, { ...folder, id: 'both', tags: ['personal', 'Work'] }];
  assert.deepEqual(
    selectFolders(folders, ['PERSONAL', 'work'], 'default', new Map()).map((item) => item.id),
    ['both'],
  );
  assert.equal(selectFolders(folders, ['missing'], 'default', new Map()).length, 0);
  assert.equal(folders.length, 2);
});

test('folder sorting supports counts and review times, keeping unscheduled folders last', () => {
  const folders = [
    { ...folder, id: 'a', title: 'A', tags: ['one'] },
    { ...folder, id: 'b', title: 'B', tags: ['one', 'two'] },
    { ...folder, id: 'c', title: 'C', tags: [] },
  ];
  const metrics = new Map([
    ['a', { cardCount: 3, nextReview: 200 }],
    ['b', { cardCount: 1, nextReview: 100 }],
    ['c', { cardCount: 0, nextReview: null }],
  ]);
  for (const [sort, expected] of [
    ['tags-desc', ['b', 'a', 'c']],
    ['tags-asc', ['c', 'a', 'b']],
    ['cards-desc', ['a', 'b', 'c']],
    ['cards-asc', ['c', 'b', 'a']],
    ['review-asc', ['b', 'a', 'c']],
    ['review-desc', ['a', 'b', 'c']],
  ])
    assert.deepEqual(
      selectFolders(folders, [], sort, metrics).map((item) => item.id),
      expected,
    );
  assert.deepEqual(
    folders.map((item) => item.id),
    ['a', 'b', 'c'],
  );
});
