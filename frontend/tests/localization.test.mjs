/* eslint-disable security/detect-object-injection -- Keys are enumerated from bundled catalogs under test. */
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { get } from 'svelte/store';
import { createLocalization } from '../src/features/localization/index.ts';

function languagePort() {
  let revision = 1;
  return {
    async GetLocalization() {
      return { preference: 'en', locale: 'en', direction: 'ltr', revision };
    },
    async SetLanguagePreference(locale) {
      return { preference: locale, locale, direction: locale === 'ar' ? 'rtl' : 'ltr', revision: ++revision };
    },
    onLocalizationChanged() {
      return () => {};
    },
  };
}

test('Arabic plural categories and both catalogs render complete messages without raw placeholders', async () => {
  const port = languagePort();
  const locale = createLocalization(port, { apply() {} });
  await locale.start();
  await locale.change('ar');
  const forms = [0, 1, 2, 3, 11, 100].map((count) => get(locale).t('common.items', { count }));
  assert.deepEqual(forms, ['لا عناصر', 'عنصر واحد', 'عنصران', '٣ عناصر', '١١ عنصرًا', '١٠٠ عنصر']);
  const en = (await import('../src/features/localization/adapters/i18next/en.json', { with: { type: 'json' } }))
    .default;
  const ar = (await import('../src/features/localization/adapters/i18next/ar.json', { with: { type: 'json' } }))
    .default;
  for (const language of ['en', 'ar']) {
    await locale.change(language);
    for (const key of Object.keys(en)) {
      assert.ok(ar[key], `Missing Arabic message: ${key}`);
      const parameters = Object.fromEntries(
        [...en[key].matchAll(/\{\{\s*(\w+)/g)].map((match) => [match[1], match[1] === 'count' ? 3 : 'test']),
      );
      const result = get(locale).t(key, parameters);
      assert.ok(result && result !== key, `${language}: ${key}`);
      assert.doesNotMatch(result, /\{\{|undefined|NaN/);
    }
  }
  locale.dispose();
});
