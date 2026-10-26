<script lang="ts">
  import { useLocalization } from '../../localization';
  import {
    CardContent,
    cardPreview,
    cardProgress,
    type Card,
    type MediaURLPort,
    type PreviewStartPort,
  } from '../../cards';
  import { ActionMenu } from '../../ui';
  import { reference, referenceKey } from '../../cardtransfer';
  import WorkspaceIcon from './WorkspaceIcon.svelte';
  const l = useLocalization();
  let {
    card,
    mediaURL,
    nativeStart,
    disabled,
    openPart,
    startReview,
    editCard,
    confirmDelete,
    selecting,
    selected,
    toggleSelected,
    moveCard,
    copyCard,
  }: {
    selecting: boolean;
    selected: boolean;
    toggleSelected: (card: Card) => void;
    moveCard: (opener: HTMLElement, card: Card) => void;
    copyCard: (opener: HTMLElement, card: Card) => void;
    card: Card;
    mediaURL: MediaURLPort;
    nativeStart: PreviewStartPort;
    disabled: boolean;
    openPart: (card: Card, itemId: string, opener: HTMLElement) => void;
    startReview: (opener: HTMLElement, card: Card) => void;
    editCard: (opener: HTMLElement, card: Card) => void;
    confirmDelete: (opener: HTMLElement, folderId: string, card: Card) => void;
  } = $props();
  const progress = $derived(cardProgress(card));
  const progressColor = $derived(
    progress <= 50
      ? '#f87171'
      : progress <= 80
        ? `color-mix(in oklab, #f87171 ${String(((80 - progress) / 30) * 100)}%, #facc15)`
        : `color-mix(in oklab, #facc15 ${String(((100 - progress) / 20) * 100)}%, #4ade80)`,
  );
</script>

<li
  class="workspace-control workspace-memory-tile"
  class:workspace-card-selecting={selecting}
  class:workspace-card-selected={selected}
  data-card-key={referenceKey(reference(card))}
>
  {#if selecting}<label class="workspace-card-selection">
      <input
        type="checkbox"
        checked={selected}
        {disabled}
        aria-label={$l.t('transfer.select_card', { name: cardPreview(card) })}
        onchange={() => {
          toggleSelected(card);
        }}
      />
      <span class="workspace-selection-check"><WorkspaceIcon name="check" size={14} /></span>
      <span class="workspace-selection-label">{$l.t(selected ? 'transfer.selected' : 'transfer.select')}</span>
    </label>{/if}
  <div
    class="workspace-memory-card"
    style:--progress-fill={progressColor}
    style:--progress-border="var(--progress-fill)"
  >
    <span
      class="workspace-control workspace-card-progress-percentage"
      aria-label={$l.t('workspace.recallSuccess', { percentage: $l.percent(progress) })}
      title={$l.t('workspace.recallExplanation')}>{$l.percent(progress)}</span
    >
    <div class="workspace-memory-card-preview" inert={selecting}>
      <CardContent
        {card}
        {mediaURL}
        {nativeStart}
        compact
        onopen={(itemId: string, opener: HTMLElement) => {
          openPart(card, itemId, opener);
        }}
      />
    </div>
  </div>
  {#if !selecting}<span class="workspace-control workspace-tile-actions"
      ><ActionMenu
        label={$l.t('workspace.cardOptions', { name: cardPreview(card) })}
        {disabled}
        actions={[
          {
            label: $l.t('workspace.edit_card'),
            onselect: (opener: HTMLButtonElement) => {
              editCard(opener, card);
            },
          },
          {
            label: $l.t('transfer.move_to'),
            onselect: (opener: HTMLButtonElement) => {
              moveCard(opener, card);
            },
          },
          {
            label: $l.t('transfer.copy'),
            onselect: (opener: HTMLButtonElement) => {
              copyCard(opener, card);
            },
          },
          {
            label: $l.t('workspace.review_now'),
            onselect: (opener: HTMLButtonElement) => {
              startReview(opener, card);
            },
          },
          {
            label: $l.t('workspace.delete_card'),
            onselect: (opener: HTMLButtonElement) => {
              confirmDelete(opener, card.folderId, card);
            },
          },
        ]}
      /></span
    >{/if}
</li>
