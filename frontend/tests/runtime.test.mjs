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
        `export const RavenService = {
        async GetConfiguration() {
          if (globalThis.ravenRuntime.failure) throw globalThis.ravenRuntime.failure;
          return globalThis.ravenRuntime.configuration;
        }
      };`,
      ],
    ]);
    const source = sources.get(specifier);
    return source
      ? { url: `data:text/javascript,${encodeURIComponent(source)}`, shortCircuit: true }
      : next(specifier, context);
  },
});
after(() => hooks.deregister());

test('runtime consumes configured event names and media routes and releases subscriptions', async (t) => {
  globalThis.ravenRuntime = {
    configuration: {
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
  dispose.forEach((unsubscribe) => unsubscribe());
  assert.equal(globalThis.ravenRuntime.unsubscribed, 4);
  globalThis.ravenRuntime.failure = new Error('configuration unavailable');
  await assert.rejects(loadConfiguration(), /configuration unavailable/);
});
