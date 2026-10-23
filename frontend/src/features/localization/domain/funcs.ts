/** Normalize human-entered digits without changing persisted numeric values. */
export function parseLocalizedNumber(value: string): number {
  const normalized = value
    .trim()
    .replace(/[٠-٩۰-۹]/g, (digit) => String(digit.charCodeAt(0) - (digit >= '۰' ? 0x6f0 : 0x660)))
    .replace(/٫/g, '.');
  if (!/^[+-]?[\d.]+$/.test(normalized) || normalized.split('.').length > 2) return Number.NaN;
  return Number(normalized);
}

/** Used only when the backend cannot provide its authoritative preference. */
export function preferredLocale(locales: readonly string[]): 'ar' | 'en' {
  for (const locale of locales) {
    const language = locale.toLowerCase().split(/[-_]/)[0];
    if (language === 'ar' || language === 'en') return language;
  }
  return 'en';
}
