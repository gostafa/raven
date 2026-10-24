import { mount } from 'svelte';
import App from './App.svelte';
import { createLocalization, configurationFailure } from './features/localization';
import { createRuntime, loadConfiguration } from './features/workspace/adapters';

const target = document.getElementById('app');
if (!target) throw new Error('Raven mount element is missing');

async function start(container: HTMLElement) {
  const configuration = await loadConfiguration();
  const backend = createRuntime(configuration);
  const localization = createLocalization(backend, {
    apply: (state) => {
      document.documentElement.lang = state.locale;
      document.documentElement.dir = state.direction;
    },
  });
  await localization.start();
  mount(App, { target: container, props: { backend, localization } });
}

void start(target).catch(() => {
  const failure = configurationFailure(navigator.languages);
  document.documentElement.lang = failure.locale;
  document.documentElement.dir = failure.direction;
  const message = document.createElement('p');
  message.textContent = failure.message;
  message.style.cssText = 'color:#fff;background:#000;padding:24px';
  target.replaceChildren(message);
});
