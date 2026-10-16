import assert from 'node:assert/strict';
import { test } from 'node:test';
import { emptyWorkspace, loadPreferences, parseTags, reconcileFolders, savePreferences, preferencesStorageKey } from '../src/folders.ts';

function memoryStorage(initial = null) {
  const values = new Map(initial === null ? [] : [[preferencesStorageKey, initial]]);
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
}

const folder = { id: 'first', title: 'Ideas', description: 'Things to explore', tags: ['Personal'] };

test('a first launch starts with an open sidebar and no folders', () => {
  assert.deepEqual({ ...emptyWorkspace(), ...loadPreferences(memoryStorage()) }, {
    folders: [], selectedFolderId: null, sidebarOpen: true,
  });
});

test('only selection and sidebar preference are saved in localStorage', () => {
  const storage = memoryStorage();
  const workspace = {
    folders: [folder, { id: 'second', title: 'Untitled ideas', description: '', tags: [] }],
    selectedFolderId: 'second', sidebarOpen: false,
  };
  savePreferences(storage, workspace);
  const preferences = { selectedFolderId: 'second', sidebarOpen: false };
  assert.deepEqual(loadPreferences(storage), preferences);
  assert.equal(storage.getItem(preferencesStorageKey), JSON.stringify(preferences));
  assert.equal(storage.getItem('raven.workspace.v1'), null);
  assert.deepEqual(reconcileFolders({ ...emptyWorkspace(), ...loadPreferences(storage) }, workspace.folders), workspace);
});

test('comma-separated tags ignore whitespace, empty entries and case-insensitive duplicates', () => {
  assert.deepEqual(parseTags(' ideas, , Personal,ideas, personal , 新しい, '), ['ideas', 'Personal', '新しい']);
  assert.deepEqual(parseTags(' , , '), []);
});

test('Home stays selected after reloading a workspace that contains folders', () => {
  const storage = memoryStorage();
  const workspace = { folders: [folder], selectedFolderId: null, sidebarOpen: true };
  savePreferences(storage, workspace);
  assert.deepEqual(reconcileFolders({ ...emptyWorkspace(), ...loadPreferences(storage) }, [folder]), workspace);
});

test('an outdated selection falls back to Home after folders are loaded', () => {
  for (const folders of [[folder], []]) {
    const storage = memoryStorage(JSON.stringify({ selectedFolderId: 'missing', sidebarOpen: true }));
    const workspace = { ...emptyWorkspace(), ...loadPreferences(storage) };
    assert.equal(workspace.selectedFolderId, 'missing');
    assert.equal(reconcileFolders(workspace, folders).selectedFolderId, null);
  }
});

test('malformed or invalid preferences are rejected without overwriting them', () => {
  const corruptValues = [
    '{invalid', 'null', '[]', '{}',
    JSON.stringify({ selectedFolderId: 123, sidebarOpen: true }),
    JSON.stringify({ selectedFolderId: null, sidebarOpen: 'false' }),
  ];
  for (const raw of corruptValues) {
    const storage = memoryStorage(raw);
    assert.throws(() => loadPreferences(storage));
    assert.equal(storage.getItem(preferencesStorageKey), raw);
  }
});

test('read and write failures reach the caller so the UI can report them', () => {
  assert.throws(() => loadPreferences({ getItem() { throw new Error('Storage unavailable'); } }), /Storage unavailable/);
  assert.throws(() => savePreferences({ setItem() { throw new Error('Quota exceeded'); } }, emptyWorkspace()), /Quota exceeded/);
});

test('legacy workspace folders and preferences are ignored and left untouched', () => {
  const storage = memoryStorage();
  const legacy = JSON.stringify({ folders: [folder], selectedFolderId: folder.id, sidebarOpen: false });
  storage.setItem('raven.workspace.v1', legacy);
  assert.deepEqual(loadPreferences(storage), { selectedFolderId: null, sidebarOpen: true });
  const workspace = reconcileFolders({ ...emptyWorkspace(), ...loadPreferences(storage) }, []);
  savePreferences(storage, workspace);
  assert.deepEqual(workspace, emptyWorkspace());
  assert.equal(storage.getItem('raven.workspace.v1'), legacy);
});
