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
  }: {
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
  const borderColor = $derived(
    progress <= 50
      ? '#ef4444'
      : progress <= 80
        ? `color-mix(in srgb, #ef4444, #eab308 ${((progress - 50) / 30) * 100}%)`
        : `color-mix(in srgb, #eab308, #22c55e ${Math.min(100, ((progress - 80) / 20) * 100)}%)`,
  );
</script>

<li class="workspace-control workspace-memory-tile">
  <div class="workspace-memory-card" style:--progress-fill="var(--ink)" style:--progress-border={borderColor}>
    <span
      class="workspace-control workspace-card-progress-percentage"
      aria-label={$l.t('workspace.recallSuccess', { percentage: $l.percent(progress) })}
      title={$l.t('workspace.recallExplanation')}>{$l.percent(progress)}</span
    >
    <div class="workspace-memory-card-preview">
      <CardContent
        {card}
        {mediaURL}
        {nativeStart}
        compact
        onopen={(itemId, opener) => openPart(card, itemId, opener)}
      />
    </div>
  </div>
  <span class="workspace-control workspace-tile-actions"
    ><ActionMenu
      label={$l.t('workspace.cardOptions', { name: cardPreview(card) })}
      {disabled}
      actions={[
        { label: $l.t('workspace.reviewCard'), onselect: (opener) => startReview(opener, card) },
        { label: $l.t('workspace.edit_card'), onselect: (opener) => editCard(opener, card) },
        {
          label: $l.t('workspace.delete_card'),
          onselect: (opener) => confirmDelete(opener, card.folderId, card),
        },
      ]}
    /></span
  >
</li>
