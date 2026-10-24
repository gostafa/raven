<script lang="ts">
  import { cardProgress, type Card } from '../../cards';
  import { useLocalization } from '../../localization';

  let { cards }: { cards: readonly Card[] } = $props();
  const l = useLocalization();
  const completed = $derived(cards.filter((card) => cardProgress(card) === 100).length);
  const progress = $derived(cards.length ? completed / cards.length : 0);
  const percentage = $derived(progress * 100);
  const progressColor = $derived(
    percentage <= 50
      ? '#f87171'
      : percentage <= 80
        ? `color-mix(in oklab, #f87171 ${String(((80 - percentage) / 30) * 100)}%, #facc15)`
        : `color-mix(in oklab, #facc15 ${String(((100 - percentage) / 20) * 100)}%, #4ade80)`,
  );
</script>

<span
  class="folder-progress"
  style:--progress-color={progressColor}
  class:complete={cards.length > 0 && completed === cards.length}
  role="img"
  aria-label={`${$l.t('workspace.recallSuccess', { percentage: $l.percent(100) })}: ${$l.number(completed)} / ${$l.number(cards.length)}`}
>
  <svg viewBox="0 0 48 48" aria-hidden="true">
    <circle class="track" cx="24" cy="24" r="20" />
    <circle
      class="fill"
      cx="24"
      cy="24"
      r="20"
      pathLength="100"
      stroke-dasharray={`${String(progress * 100)} 100`}
      opacity={completed ? 1 : 0}
    />
  </svg>
  <span aria-hidden="true">{completed}/{cards.length}</span>
</span>

<style>
  .folder-progress {
    position: absolute;
    bottom: 12px;
    inset-inline-end: 12px;
    display: grid;
    place-items: center;
    width: 48px;
    height: 48px;
    color: var(--muted);
    font-size: 8px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  svg {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    transform: rotate(-90deg);
  }
  circle {
    fill: none;
    stroke-width: 4;
  }
  .track {
    stroke: var(--progress-color);
    opacity: 0.25;
  }
  .fill {
    stroke: var(--progress-color);
    stroke-linecap: round;
  }
  .complete {
    color: #fff;
  }
  .complete .fill {
    stroke: var(--progress-color);
  }
</style>
