<script lang="ts">
  import { useLocalization } from '../../localization';
  import type { EditorItem } from '../domain/types';
  import AudioPlayer from './AudioPlayer.svelte';
  const l = useLocalization();
  let { item }: { item: EditorItem } = $props();
  let failed = $state(false);
</script>

{#if failed}<p class="hint" role="status">
    {$l.t(
      'cards.this_file_could_not_be_loaded_or_played_check_that_it_is_available_and_its_format_is_supported_on_this_device',
    )}
  </p>
{:else if item.previewURL}
  {#if item.kind === 'image'}<img
      src={item.previewURL}
      alt={item.originalName || $l.t('cards.card_image')}
      onerror={() => (failed = true)}
    />
  {:else if item.kind === 'audio'}<AudioPlayer
      src={item.previewURL}
      filename={item.originalName}
      onerror={() => (failed = true)}
    />
  {:else if item.kind === 'video'}
    <!-- svelte-ignore a11y_media_has_caption (Imported videos do not include separate caption files.) -->
    <video
      dir="ltr"
      controls
      playsinline
      preload="metadata"
      src={item.previewURL}
      aria-label={item.originalName}
      onerror={() => (failed = true)}
    ></video>
  {/if}
{/if}
<p class="filename">
  <bdi>{item.originalName}</bdi><span
    >{item.id ? $l.t('cards.saved_file') : $l.t('cards.will_be_copied_when_you_save')}</span
  >
</p>

<style>
  .filename {
    overflow-wrap: anywhere;
    margin: 0;
    color: var(--ink);
    font-size: 12px;
    line-height: 1.6;
  }
  img,
  video {
    display: block;
    width: 100%;
    max-height: 320px;
    object-fit: contain;
    border-radius: 6px;
    background: #000;
    margin-bottom: 8px;
  }
  .filename span {
    display: block;
    font-size: 10px;
    color: var(--muted);
    margin-top: 3px;
  }
  .hint {
    font-size: 10px;
    color: var(--muted);
    line-height: 1.6;
  }
</style>
