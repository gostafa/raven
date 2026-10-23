import { getContext, setContext } from 'svelte';
import type { Localization } from '../i18next/types';
import { contextKey } from './consts';

export function provideLocalization(localization: Localization): void {
  setContext(contextKey, localization);
}

export function useLocalization(): Localization {
  return getContext<Localization>(contextKey);
}
