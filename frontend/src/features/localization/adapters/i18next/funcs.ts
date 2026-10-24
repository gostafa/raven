import { createInstance } from 'i18next';
import { writable } from 'svelte/store';
import type { LocalizationState } from '../../domain/types';
import type { LocalizationPort, PresentationEnvironment } from '../../ports/types';
import { newerLocalization } from '../../application/funcs';
import type { Localization, MessageKey, Presentation } from './types';
import en from './en.json' with { type: 'json' };
import ar from './ar.json' with { type: 'json' };
import { preferredLocale } from '../../domain/funcs';

export function configurationFailure(locales: readonly string[]): {
  locale: string;
  direction: string;
  message: string;
} {
  const locale = preferredLocale(locales);
  return {
    locale,
    direction: locale === 'ar' ? 'rtl' : 'ltr',
    message: (locale === 'ar' ? ar : en)['common.raven_could_not_load_its_configuration_please_restart_the_app'],
  };
}
function initializeEngine(engine: ReturnType<typeof createInstance>): Promise<unknown> {
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
  return ready;
}
function interpolation(values: Record<string, string | number>): Record<string, string | number> {
  return Object.fromEntries(
    Object.entries(values).map(([name, value]) => [
      name,
      typeof value === 'string' && ['name', 'query', 'shortcut'].includes(name) ? `\u2068${value}\u2069` : value,
    ]),
  );
}
function duration(value: number, number: Presentation['number']): string {
  if (!Number.isFinite(value) || value < 0) return '--:--';
  const minutes = number(Math.floor(value / 60), { useGrouping: false });
  const seconds = number(Math.floor(value % 60), { minimumIntegerDigits: 2, useGrouping: false });
  return `${minutes}:${seconds}`;
}
function formatting(locale: string): Pick<Presentation, 'number' | 'percent' | 'date' | 'duration' | 'compare'> {
  const numericLocale = locale === 'ar' ? 'ar-u-ca-gregory-nu-arab' : 'en-u-ca-gregory-nu-latn';
  const number: Presentation['number'] = (value, options) =>
    new Intl.NumberFormat(numericLocale, options).format(value);
  return {
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
    duration: (value) => duration(value, number),
    compare: (left, right) => new Intl.Collator(locale).compare(left, right),
  };
}
function presentation(state: LocalizationState, engine: ReturnType<typeof createInstance>): Presentation {
  const locale = state.locale;
  return {
    ...state,
    ...formatting(locale),
    t: (key: MessageKey, values = {}) => engine.t(key, { ...interpolation(values), lng: locale }),
  };
}
class I18nLocalization implements Localization {
  private readonly engine = createInstance();
  private readonly ready = initializeEngine(this.engine);
  private state: LocalizationState = { preference: 'system', locale: 'en', direction: 'ltr', revision: -1 };
  private readonly store = writable(presentation(this.state, this.engine));
  private cancel = (): void => {
    /* No subscription exists before startup. */
  };
  private disposed = false;
  public constructor(
    private readonly port: LocalizationPort,
    private readonly environment: PresentationEnvironment,
  ) {}
  public subscribe: Localization['subscribe'] = (run, invalidate) => this.store.subscribe(run, invalidate);
  private accept(incoming: LocalizationState): void {
    if (this.disposed) return;
    const next = newerLocalization(this.state, incoming);
    if (next === this.state) return;
    this.state = next;
    this.environment.apply(next);
    this.store.set(presentation(next, this.engine));
  }
  public async start(): Promise<void> {
    await this.ready;
    this.cancel = this.port.onLocalizationChanged((incoming) => {
      this.accept(incoming);
    });
    try {
      this.accept(await this.port.GetLocalization());
    } catch (error) {
      this.cancel();
      throw error;
    }
  }
  public change: Localization['change'] = async (preference) => {
    this.accept(await this.port.SetLanguagePreference(preference));
  };
  public dispose(): void {
    this.disposed = true;
    this.cancel();
  }
}
export function createLocalization(port: LocalizationPort, environment: PresentationEnvironment): Localization {
  return new I18nLocalization(port, environment);
}
