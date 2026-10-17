import { mount } from 'svelte';
import App from './App.svelte';
import { createRuntime, loadConfiguration } from './features/workspace/adapters';

const target = document.getElementById('app');
if (!target) throw new Error('Raven mount element is missing');

async function start(target: HTMLElement) {
  const configuration = await loadConfiguration();
  mount(App, { target, props: { backend: createRuntime(configuration) } });
}

void start(target).catch(() => {
  const message = document.createElement('p');
  message.textContent = 'Raven could not load its configuration. Please restart the app.';
  message.style.cssText = 'color:#fff;background:#000;padding:24px';
  target.replaceChildren(message);
});
