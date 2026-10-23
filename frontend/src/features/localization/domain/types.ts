export type LanguagePreference = 'system' | 'en' | 'ar';
export type Locale = 'en' | 'ar';
export interface LocalizationState {
  preference: LanguagePreference;
  locale: Locale;
  direction: 'ltr' | 'rtl';
  revision: number;
}
