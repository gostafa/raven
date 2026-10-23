import type { Readable } from 'svelte/store';
import type { LanguagePreference, LocalizationState } from '../../domain/types';
import type en from './en.json';

export type MessageKey = keyof typeof en;
export interface Presentation extends LocalizationState {
  t(key: MessageKey, values?: Record<string, string | number>): string;
  number(value: number, options?: Intl.NumberFormatOptions): string;
  percent(value: number): string;
  date(value: Date | number): string;
  duration(value: number): string;
  compare(left: string, right: string): number;
}

export interface Localization extends Readable<Presentation> {
  start(): Promise<void>;
  change(preference: LanguagePreference): Promise<void>;
  dispose(): void;
}
