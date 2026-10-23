import { createInstance } from 'i18next';
import { writable } from 'svelte/store';
import type { LocalizationState } from '../../domain/types';
import type { LocalizationPort, PresentationEnvironment } from '../../ports/types';
import { newerLocalization } from '../../application/funcs';
import type { Localization, MessageKey, Presentation } from './types';
import en from './en.json' with { type: 'json' };
import ar from './ar.json' with { type: 'json' };
import { preferredLocale } from '../../domain/funcs';

export function configurationFailure(locales: readonly string[]) {
  const locale = preferredLocale(locales);
  return {
    locale,
    direction: locale === 'ar' ? 'rtl' : 'ltr',
    message: (locale === 'ar' ? ar : en)['common.raven_could_not_load_its_configuration_please_restart_the_app'],
  };
}

export function createLocalization(port: LocalizationPort, environment: PresentationEnvironment): Localization {
  const engine = createInstance();
  let state: LocalizationState = { preference: 'system', locale: 'en', direction: 'ltr', revision: -1 };
  let cancel = () => {
    /* No subscription exists before startup. */
  };
  let disposed = false;
  const ready = engine.init({
    lng: 'en',
    fallbackLng: 'en',
    supportedLngs: ['en', 'ar'],
    resources: { en: { translation: en }, ar: { translation: ar } },
    keySeparator: false,
    nsSeparator: false,
    initAsync: false,
    interpolation: { escapeValue: false },
  });
  engine.services.formatter?.addCached('number', (locale, options: Intl.NumberFormatOptions) => {
    const formatter = new Intl.NumberFormat(locale, {
      ...options,
      numberingSystem: locale?.startsWith('ar') === true ? 'arab' : 'latn',
    });
    return (value: number) => formatter.format(value);
  });

  function presentation(): Presentation {
    const locale = state.locale;
    const numericLocale = locale === 'ar' ? 'ar-u-ca-gregory-nu-arab' : 'en-u-ca-gregory-nu-latn';
    const number = (value: number, options?: Intl.NumberFormatOptions) =>
      new Intl.NumberFormat(numericLocale, options).format(value);
    return {
      ...state,
      t: (key: MessageKey, values = {}) =>
        engine.t(key, {
          ...Object.fromEntries(
            Object.entries(values).map(([name, value]) => [
              name,
              typeof value === 'string' && ['name', 'query', 'shortcut'].includes(name)
                ? `\u2068${value}\u2069`
                : value,
            ]),
          ),
          lng: locale,
        }),
      number,
      percent: (value) => number(value / 100, { style: 'percent', maximumFractionDigits: 0 }),
      date: (value) =>
        new Intl.DateTimeFormat(numericLocale, {
          calendar: 'gregory',
          month: 'short',
          day: 'numeric',
          hour: 'numeric',
          minute: '2-digit',
        }).format(value),
      duration: (value) =>
        !Number.isFinite(value) || value < 0
          ? '--:--'
          : `${number(Math.floor(value / 60), { useGrouping: false })}:${number(Math.floor(value % 60), { minimumIntegerDigits: 2, useGrouping: false })}`,
      compare: (left, right) => new Intl.Collator(locale).compare(left, right),
    };
  }
  const store = writable(presentation());
  function accept(incoming: LocalizationState) {
    if (disposed) return;
    const next = newerLocalization(state, incoming);
    if (next === state) return;
    state = next;
    environment.apply(state);
    store.set(presentation());
  }
  return {
    subscribe: store.subscribe,
    async start() {
      await ready;
      cancel = port.onLocalizationChanged(accept);
      try {
        accept(await port.GetLocalization());
      } catch (error) {
        cancel();
        throw error;
      }
    },
    async change(preference) {
      accept(await port.SetLanguagePreference(preference));
    },
    dispose() {
      disposed = true;
      cancel();
    },
  } satisfies Localization;
}
