export function menuFocusIndex(key: string, current: number, count: number): number | null {
  if (count === 0) return null;
  if (key === 'Home') return 0;
  if (key === 'End') return count - 1;
  if (key !== 'ArrowDown' && key !== 'ArrowUp') return null;
  if (current < 0) return key === 'ArrowUp' ? count - 1 : 0;
  return (current + (key === 'ArrowDown' ? 1 : -1) + count) % count;
}

export function toggleSelection<Value>(selected: readonly Value[], value: Value): Value[] {
  return selected.includes(value) ? selected.filter((item) => item !== value) : [...selected, value];
}
