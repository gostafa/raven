import assert from 'node:assert/strict';
import { test } from 'node:test';
import { get } from 'svelte/store';
import { createLocalization, parseLocalizedNumber } from '../src/features/localization/index.ts';

function languagePort() {
  let state = { preference: 'system', locale: 'en', direction: 'ltr', revision: 1 };
  const listeners = new Set();
  let failure;
  return {
    async GetLocalization() {
      return state;
    },
    async SetLanguagePreference(preference) {
      if (failure) throw failure;
      state = {
        preference,
        locale: preference === 'ar' ? 'ar' : 'en',
        direction: preference === 'ar' ? 'rtl' : 'ltr',
        revision: state.revision + 1,
      };
      for (const listener of listeners) listener(state);
      return state;
    },
    onLocalizationChanged(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    emit(value) {
      for (const listener of listeners) listener(value);
    },
    fail() {
      failure = new Error('disk unavailable');
    },
    get subscriptions() {
      return listeners.size;
    },
  };
}

test('language changes update every window, format Arabic values, and release subscriptions', async () => {
  const port = languagePort();
  const documents = [];
  const windows = [0, 1, 2].map(() => createLocalization(port, { apply: (state) => documents.push(state) }));
  await Promise.all(windows.map((window) => window.start()));
  await windows[0].change('ar');
  for (const window of windows) {
    const view = get(window);
    assert.equal(view.direction, 'rtl');
    assert.equal(view.locale, 'ar');
    assert.equal(view.number(123), '١٢٣');
    assert.match(view.percent(75), /٧٥/);
    assert.equal(view.duration(65), '١:٠٥');
    assert.match(view.date(new Date('2026-10-23T12:00:00Z')), /٢٣/);
    assert.equal(view.t('common.language'), 'اللغة');
    assert.equal(view.t('common.items', { count: 2 }), 'عنصران');
  }
  port.emit({ preference: 'en', locale: 'en', direction: 'ltr', revision: 1 });
  assert.equal(get(windows[0]).locale, 'ar');
  await windows[1].change('en');
  assert.equal(get(windows[2]).t('common.language'), 'Language');
  assert.equal(get(windows[2]).number(123), '123');
  assert.equal(documents.length, 9);
  windows.forEach((window) => {
    window.dispose();
  });
  assert.equal(port.subscriptions, 0);
});

test('a newer event arriving before the initial response wins, and failed writes preserve language', async () => {
  const port = languagePort();
  let respond;
  port.GetLocalization = () =>
    new Promise((resolve) => {
      respond = resolve;
    });
  const view = createLocalization(port, { apply() {} });
  const starting = view.start();
  await new Promise(setImmediate);
  port.emit({ preference: 'ar', locale: 'ar', direction: 'rtl', revision: 2 });
  respond({ preference: 'system', locale: 'en', direction: 'ltr', revision: 1 });
  await starting;
  assert.equal(get(view).locale, 'ar');
  port.fail();
  await assert.rejects(view.change('en'), /disk unavailable/);
  assert.equal(get(view).locale, 'ar');
  view.dispose();
});

test('numeric input accepts Western, Arabic-Indic, and Persian digits and rejects malformed input', () => {
  for (const digits of ['75', '٧٥', '۷۵']) assert.equal(parseLocalizedNumber(digits), 75);
  assert.equal(parseLocalizedNumber(' ٢٥٫٥ '), 25.5);
  for (const digits of ['', 'hello', '١٢abc', '١.٢.٣']) assert.ok(Number.isNaN(parseLocalizedNumber(digits)));
});
