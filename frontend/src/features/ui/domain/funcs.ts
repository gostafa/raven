export function menuFocusIndex(key: string, current: number, count: number): number | null {
  if (count === 0) return null;
  if (key === 'Home') return 0;
  if (key === 'End') return count - 1;
  return arrowFocusIndex(key, current, count);
}
function arrowFocusIndex(key: string, current: number, count: number): number | null {
  if (key !== 'ArrowDown' && key !== 'ArrowUp') return null;
  if (current < 0) return initialFocusIndex(key, count);
  return (current + (key === 'ArrowDown' ? 1 : -1) + count) % count;
}
function initialFocusIndex(key: string, count: number): number {
  return key === 'ArrowUp' ? count - 1 : 0;
}

export function toggleSelection<Value>(selected: readonly Value[], value: Value): Value[] {
  return selected.includes(value) ? selected.filter((item) => item !== value) : [...selected, value];
}
