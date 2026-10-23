import type { LocalizationState } from '../domain/types';

/** Events can arrive before the initial request resolves, or before a setter returns. */
export function newerLocalization(current: LocalizationState, incoming: LocalizationState): LocalizationState {
  return incoming.revision > current.revision ? incoming : current;
}
