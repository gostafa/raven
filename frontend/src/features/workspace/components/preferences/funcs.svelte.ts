import type { ViewPreferences } from '../../domain/types';
import type { PreferencesBackend, PreferencesState } from './types';
import { defaultViewPreferences } from '../../domain/funcs';
import { loadViewPreferences, saveViewPreferences } from '../../application/funcs';

function restore(backend: PreferencesBackend, state: PreferencesState): ViewPreferences {
  try {
    const port = backend();
    return loadViewPreferences(port.preferences, port.configuration.preferencesStorageKey);
  } catch {
    state.error = 'workspace.your_filters_and_sort_preferences_could_not_be_loaded';
    return defaultViewPreferences();
  }
}
function persist(backend: PreferencesBackend, state: PreferencesState): void {
  try {
    const port = backend();
    saveViewPreferences(port.preferences, port.configuration.preferencesStorageKey, state.view);
    state.error = '';
  } catch {
    state.error = 'workspace.your_filters_and_sort_preferences_could_not_be_saved_on_this_device';
  }
}
/** Owns the component's persisted filter state and its persistence lifecycle. */
export function createViewPreferences(backend: PreferencesBackend): PreferencesState {
  const state = $state<PreferencesState>({ view: defaultViewPreferences(), error: '' });
  state.view = restore(backend, state);
  $effect(() => {
    persist(backend, state);
  });
  return state;
}
