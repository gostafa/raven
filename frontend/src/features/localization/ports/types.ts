import type { LanguagePreference, LocalizationState } from '../domain/types';

export interface LocalizationPort {
  GetLocalization(): Promise<LocalizationState>;
  SetLanguagePreference(preference: LanguagePreference): Promise<LocalizationState>;
  onLocalizationChanged(callback: (state: LocalizationState) => void): () => void;
}

export interface PresentationEnvironment {
  apply(state: LocalizationState): void;
}
