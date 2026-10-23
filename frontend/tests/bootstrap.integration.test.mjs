import assert from 'node:assert/strict';
import { registerHooks } from 'node:module';
import { after, test } from 'node:test';

const entry = new URL('../src/main.ts', import.meta.url).href;
const hooks = registerHooks({
  resolve(specifier, context, next) {
    if (!context.parentURL?.startsWith(entry)) return next(specifier, context);
    const sources = new Map([
      ['svelte', 'export function mount(component, options) { globalThis.ravenBootstrap.mounts.push(options); }'],
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
  const target = {
    children: [],
    replaceChildren(child) {
      this.children = [child];
    },
  };
  let resolve, reject;
  const state = {
    mounts: [],
    runtimes: [],
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
    delete globalThis.ravenBootstrap;
  });
  return { state, target, resolve, reject };
}

test('each window waits for configuration before constructing its runtime and mounting', async (t) => {
  const { state, target, resolve } = environment(t);
  await import('../src/main.ts?bootstrap-success');
  assert.equal(state.mounts.length, 0);
  assert.equal(state.runtimes.length, 0);
  const settings = { preferencesStorageKey: 'custom.preferences', mediaRoutePrefix: '/saved/' };
  resolve(settings);
  await new Promise(setImmediate);
  assert.deepEqual(state.runtimes, [settings]);
  assert.equal(state.localizationStarted, true);
  assert.equal(globalThis.document.documentElement.dir, 'rtl');
  assert.equal(state.mounts[0].target, target);
  assert.equal(state.mounts[0].props.backend, settings);
});

test('configuration failure prevents partial initialization and shows a monochrome error', async (t) => {
  const { state, target, reject } = environment(t);
  await import('../src/main.ts?bootstrap-failure');
  reject(new Error('configuration unavailable'));
  await new Promise(setImmediate);
  assert.deepEqual(state.mounts, []);
  assert.deepEqual(state.runtimes, []);
  assert.match(target.children[0].textContent, /could not load its configuration/);
  assert.match(target.children[0].style.cssText, /color:#fff;background:#000/);
});
