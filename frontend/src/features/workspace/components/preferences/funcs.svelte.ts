import type { MessageKey } from '../../../localization';
import type { AppPorts, ViewPreferences } from '../../domain/types';
import { defaultViewPreferences } from '../../domain/funcs';
import { loadViewPreferences, saveViewPreferences } from '../../application/funcs';

/** Owns the component's persisted filter state and its persistence lifecycle. */
export function createViewPreferences(backend: () => Pick<AppPorts, 'preferences' | 'configuration'>) {
  let error = $state<MessageKey | ''>('');
  const view = $state<ViewPreferences>(restore());

  function restore(): ViewPreferences {
    try {
      const port = backend();
      return loadViewPreferences(port.preferences, port.configuration.preferencesStorageKey);
    } catch {
      error = 'workspace.your_filters_and_sort_preferences_could_not_be_loaded';
      return defaultViewPreferences();
    }
  }

  $effect(() => {
    try {
      const port = backend();
      saveViewPreferences(port.preferences, port.configuration.preferencesStorageKey, view);
      error = '';
    } catch {
      error = 'workspace.your_filters_and_sort_preferences_could_not_be_saved_on_this_device';
    }
  });

  return {
    view,
    get error() {
      return error;
    },
  };
}
