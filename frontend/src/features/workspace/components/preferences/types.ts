import type { MessageKey } from '../../../localization';
import type { AppPorts, ViewPreferences } from '../../domain/types';

export type PreferencesBackend = () => Pick<AppPorts, 'preferences' | 'configuration'>;
export interface PreferencesState {
  view: ViewPreferences;
  error: MessageKey | '';
}
