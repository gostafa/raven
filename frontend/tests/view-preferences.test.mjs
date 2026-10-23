import assert from 'node:assert/strict';
import { test } from 'node:test';
import { loadViewPreferences, saveViewPreferences } from '../src/features/workspace/application/funcs.ts';

function memoryStorage() {
  const values = new Map();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
}

test('all view choices survive reload without overwriting navigation preferences', () => {
  const storage = memoryStorage();
  storage.setItem('custom', 'navigation');
  const preferences = {
    search: 'my memory',
    foldersOpen: false,
    folderTags: ['Work', 'Personal'],
    folderSort: 'review-desc',
    cardSort: 'progress-asc',
    cardKinds: ['image', 'audio'],
    cardProgressRange: { minimum: 25, maximum: 75 },
  };
  saveViewPreferences(storage, 'custom', preferences);
  assert.deepEqual(loadViewPreferences(storage, 'custom'), preferences);
  assert.equal(storage.getItem('custom'), 'navigation');
  assert.equal(storage.getItem('raven.preferences.v1.view'), null);
});

test('clearing filters and search persists across the next launch', () => {
  const storage = memoryStorage();
  const defaults = loadViewPreferences(storage, 'custom');
  saveViewPreferences(storage, 'custom', {
    ...defaults,
    search: 'old',
    folderTags: ['Work'],
    cardKinds: ['video'],
    cardProgressRange: { minimum: 50, maximum: 90 },
  });
  saveViewPreferences(storage, 'custom', defaults);
  assert.deepEqual(loadViewPreferences(storage, 'custom'), defaults);
});

test('older or invalid saved fields fall back independently while preserving valid choices', () => {
  const storage = memoryStorage();
  const defaults = loadViewPreferences(storage, 'custom');
  for (const range of [
    null,
    {},
    { minimum: -1, maximum: 80 },
    { minimum: 80, maximum: 20 },
    { minimum: 0, maximum: 101 },
  ]) {
    storage.setItem(
      'custom.view',
      JSON.stringify({
        search: 'keep',
        foldersOpen: false,
        folderSort: 'removed',
        cardSort: 123,
        folderTags: ['Work', null, 'Work', ''],
        cardKinds: ['video', 'unknown', 'video', null],
        cardProgressRange: range,
      }),
    );
    assert.deepEqual(loadViewPreferences(storage, 'custom'), {
      ...defaults,
      search: 'keep',
      foldersOpen: false,
      folderTags: ['Work'],
      cardKinds: ['video'],
    });
  }
  for (const value of [null, [], {}, { folderSort: 'tags-asc' }]) {
    storage.setItem('custom.view', JSON.stringify(value));
    assert.deepEqual(loadViewPreferences(storage, 'custom'), {
      ...defaults,
      ...(value?.folderSort ? { folderSort: value.folderSort } : {}),
    });
  }
});

test('storage and malformed JSON failures reach the UI caller', () => {
  const storage = memoryStorage();
  storage.setItem('custom.view', '{bad');
  assert.throws(() => loadViewPreferences(storage, 'custom'));
  const failure = new Error('storage unavailable');
  const unavailable = {
    getItem() {
      throw failure;
    },
    setItem() {
      throw failure;
    },
  };
  assert.throws(() => loadViewPreferences(unavailable, 'custom'), failure);
  assert.throws(() => saveViewPreferences(unavailable, 'custom', {}), failure);
});
