import assert from 'node:assert/strict';
import { registerHooks } from 'node:module';
import { after, test } from 'node:test';

const entry = new URL('../src/main.ts', import.meta.url).href;
const hooks = registerHooks({
  resolve(specifier, context, next) {
    if (!context.parentURL?.startsWith(entry)) return next(specifier, context);
    const sources = new Map([
      [
        'svelte',
        `export function mount(component, options) { globalThis.ravenBootstrap.mounts.push(options); return options; }
         export async function unmount(instance) { globalThis.ravenBootstrap.unmounts.push(instance); }`,
      ],
      ['./features/workspace', 'export const WorkspaceLoading = {};'],
      ['./App.svelte', 'export default {};'],
      [
        './features/localization',
        `export function configurationFailure() { return {locale:'en',direction:'ltr',message:'Raven could not load its configuration. Please restart the app.'}; }
 export function createLocalization(port, environment) {
        return { async start() { globalThis.ravenBootstrap.localizationStarted = true; environment.apply({locale:'ar',direction:'rtl'}); }, dispose() {} };
      }`,
      ],
      [
        './features/workspace/adapters',
        `
        export function loadConfiguration() { return globalThis.ravenBootstrap.loading; }
        export async function notify(title, body) {
          globalThis.ravenBootstrap.notifications.push({title, body});
          if (globalThis.ravenBootstrap.notificationFailure) throw new Error('native notifications unavailable');
        }
        export function createRuntime(configuration) {
          globalThis.ravenBootstrap.runtimes.push(configuration);
          return configuration;
        }`,
      ],
    ]);
    const source = sources.get(specifier);
    return source
      ? { url: `data:text/javascript,${encodeURIComponent(source)}`, shortCircuit: true }
      : next(specifier, context);
  },
});
after(() => hooks.deregister());

function environment(t) {
  const previousDocument = globalThis.document;
  const previousWindow = globalThis.window;
  globalThis.window = { location: { search: '' } };
  const target = {
    children: [],
    replaceChildren(...children) {
      this.children = children;
    },
  };
  let resolve, reject;
  const state = {
    mounts: [],
    unmounts: [],
    runtimes: [],
    notifications: [],
    loading: new Promise((accept, fail) => {
      resolve = accept;
      reject = fail;
    }),
  };
  globalThis.ravenBootstrap = state;
  globalThis.document = {
    documentElement: {},
    getElementById: () => target,
    createElement: () => ({ textContent: '', style: {} }),
  };
  t.after(() => {
    globalThis.document = previousDocument;
    globalThis.window = previousWindow;
    delete globalThis.ravenBootstrap;
  });
  return { state, target, resolve, reject };
}

test('the library shows a skeleton immediately while configuration loads', async (t) => {
  const { state, target, resolve } = environment(t);
  await import('../src/main.ts?bootstrap-success');
  assert.equal(state.mounts.length, 1);
  assert.equal(state.mounts[0].target, target);
  assert.equal(state.runtimes.length, 0);
  const settings = { preferencesStorageKey: 'custom.preferences', mediaRoutePrefix: '/saved/' };
  resolve(settings);
  await new Promise(setImmediate);
  assert.deepEqual(state.runtimes, [settings]);
  assert.equal(state.localizationStarted, true);
  assert.equal(globalThis.document.documentElement.dir, 'rtl');
  assert.equal(state.mounts[0].target, target);
  assert.deepEqual(state.unmounts, [state.mounts[0]]);
  assert.equal(state.mounts[1].props.backend, settings);
});

test('configuration failure uses native notifications without an in-app message', async (t) => {
  const { state, target, reject } = environment(t);
  await import('../src/main.ts?bootstrap-failure');
  reject(new Error('configuration unavailable'));
  await new Promise(setImmediate);
  assert.equal(state.mounts.length, 1);
  assert.deepEqual(state.unmounts, [state.mounts[0]]);
  assert.deepEqual(state.runtimes, []);
  assert.deepEqual(target.children, []);
  assert.equal(state.notifications[0].title, 'Raven');
  assert.match(state.notifications[0].body, /could not load its configuration/);
});

test('unavailable native notifications do not create a startup banner', async (t) => {
  const { state, target, reject } = environment(t);
  state.notificationFailure = true;
  await import('../src/main.ts?bootstrap-notifications-unavailable');
  reject(new Error('configuration unavailable'));
  await new Promise(setImmediate);
  assert.deepEqual(target.children, []);
  assert.equal(state.notifications.length, 1);
  assert.deepEqual(state.runtimes, []);
});
