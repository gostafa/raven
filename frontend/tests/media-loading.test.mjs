import assert from 'node:assert/strict';
import { test } from 'node:test';
import { loadMedia } from '../src/features/cards/components/viewport/funcs.ts';

test('media waits for the preload margin and remains activated when it leaves the viewport', (t) => {
  const observers = [];
  mockObserver(
    t,
    class {
      constructor(callback, options) {
        this.callback = callback;
        this.options = options;
        this.disconnected = 0;
        observers.push(this);
      }
      observe(node) {
        this.node = node;
      }
      disconnect() {
        this.disconnected++;
      }
    },
  );
  const node = new EventTarget();
  const sources = [];
  const options = { source: '/media/first', deferred: true, active: false, resolve: (source) => sources.push(source) };
  const handle = loadMedia(node, options);
  const observer = observers[0];
  assert.equal(observer.node, node);
  assert.deepEqual(observer.options, { rootMargin: '300px' });
  observer.callback([{ isIntersecting: false }]);
  assert.deepEqual(sources, []);
  observer.callback([{ isIntersecting: true }]);
  observer.callback([{ isIntersecting: false }]);
  handle.update({ ...options, active: false });
  node.dispatchEvent(new Event('pointerenter'));
  assert.deepEqual(sources, ['/media/first']);
  assert.ok(observer.disconnected > 0);
  handle.destroy();
  observer.callback([{ isIntersecting: true }]);
  node.dispatchEvent(new Event('focusin'));
  assert.deepEqual(sources, ['/media/first']);
});

test('source replacement resets loading and hover or keyboard focus activates it immediately', (t) => {
  const observers = [];
  mockObserver(
    t,
    class {
      constructor(callback) {
        this.callback = callback;
        observers.push(this);
      }
      observe() {}
      disconnect() {}
    },
  );
  const sources = [];
  const node = new EventTarget();
  const options = { source: 'one', deferred: true, active: false, resolve: (source) => sources.push(source) };
  const handle = loadMedia(node, options);
  node.dispatchEvent(new Event('pointerenter'));
  handle.update({ ...options, source: 'two' });
  observers[0].callback([{ isIntersecting: true }]);
  assert.deepEqual(sources, ['one', undefined]);
  node.dispatchEvent(new Event('focusin'));
  assert.deepEqual(sources, ['one', undefined, 'two']);
  handle.update({ ...options, source: 'three', active: true });
  assert.deepEqual(sources, ['one', undefined, 'two', undefined, 'three']);
  handle.destroy();
  observers.at(-1).callback([{ isIntersecting: true }]);
  assert.equal(sources.at(-1), 'three');
});

test('reviews, editor media and platforms without observers activate immediately', (t) => {
  mockObserver(
    t,
    class {
      constructor() {
        assert.fail('immediate media should not allocate an observer');
      }
    },
  );
  for (const options of [
    { deferred: false, active: false },
    { deferred: true, active: true },
  ]) {
    const sources = [];
    const handle = loadMedia(new EventTarget(), {
      ...options,
      source: 'immediate',
      resolve: (source) => sources.push(source),
    });
    assert.deepEqual(sources, ['immediate']);
    handle.destroy();
  }
  Object.defineProperty(globalThis, 'IntersectionObserver', { configurable: true, writable: true, value: undefined });
  const sources = [];
  const handle = loadMedia(new EventTarget(), {
    source: 'fallback',
    deferred: true,
    active: false,
    resolve: (source) => sources.push(source),
  });
  assert.deepEqual(sources, ['fallback']);
  handle.destroy();
});

function mockObserver(t, implementation) {
  const original = Object.getOwnPropertyDescriptor(globalThis, 'IntersectionObserver');
  Object.defineProperty(globalThis, 'IntersectionObserver', {
    configurable: true,
    writable: true,
    value: implementation,
  });
  t.after(() => {
    if (original) Object.defineProperty(globalThis, 'IntersectionObserver', original);
    else delete globalThis.IntersectionObserver;
  });
}
