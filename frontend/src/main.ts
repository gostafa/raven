import { mount, unmount } from 'svelte';
import App from './App.svelte';
import { WorkspaceLoading } from './features/workspace';
import { createLocalization, configurationFailure } from './features/localization';
import { createRuntime, loadConfiguration, notify } from './features/workspace/adapters';

const target = document.getElementById('app');
if (!target) throw new Error('Raven mount element is missing');
const view = new URLSearchParams(window.location.search).get('view');
const loading = view !== 'widget' && view !== 'review' ? mount(WorkspaceLoading, { target }) : null;

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
  if (loading) await unmount(loading);
  mount(App, { target: container, props: { backend, localization } });
}

void start(target).catch(async () => {
  if (loading) await unmount(loading);
  const failure = configurationFailure(navigator.languages);
  document.documentElement.lang = failure.locale;
  document.documentElement.dir = failure.direction;
  target.replaceChildren();
  await notify('Raven', failure.message).catch(() => {
    // Native delivery is optional; keep startup failures out of the workspace.
  });
});
