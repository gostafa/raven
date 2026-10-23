export type { LanguagePreference, Locale, LocalizationState } from './domain/types';
export type { LocalizationPort, PresentationEnvironment } from './ports/types';
export type { Localization, MessageKey, Presentation } from './adapters/i18next/types';
export { parseLocalizedNumber } from './domain/funcs';
export { newerLocalization } from './application/funcs';
export { createLocalization, configurationFailure } from './adapters/i18next/funcs';
export { provideLocalization, useLocalization } from './adapters/svelte/funcs';
export { default as LanguageSelector } from './components/LanguageSelector.svelte';
