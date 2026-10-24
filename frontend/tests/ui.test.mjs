import assert from 'node:assert/strict';
import { test } from 'node:test';
import { menuFocusIndex, toggleSelection } from '../src/features/ui/domain/funcs.ts';

test('menu navigation wraps and enters from either direction', () => {
  assert.equal(menuFocusIndex('ArrowDown', -1, 3), 0);
  assert.equal(menuFocusIndex('ArrowUp', -1, 3), 2);
  assert.equal(menuFocusIndex('ArrowDown', 2, 3), 0);
  assert.equal(menuFocusIndex('ArrowUp', 0, 3), 2);
  assert.equal(menuFocusIndex('Home', 2, 3), 0);
  assert.equal(menuFocusIndex('End', 0, 3), 2);
});

test('empty menus and typing leave focus unchanged', () => {
  for (const key of ['ArrowDown', 'ArrowUp', 'Home', 'End']) {
    assert.equal(menuFocusIndex(key, -1, 0), null);
  }
  assert.equal(menuFocusIndex('a', 1, 3), null);
});

test('filter toggles preserve other selections without mutating the caller', () => {
  const selected = Object.freeze(['image', 'audio']);
  assert.deepEqual(toggleSelection(selected, 'image'), ['audio']);
  assert.deepEqual(toggleSelection(selected, 'text'), ['image', 'audio', 'text']);
  assert.deepEqual(selected, ['image', 'audio']);
});
