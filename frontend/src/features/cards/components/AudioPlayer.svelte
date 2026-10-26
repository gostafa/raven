<script lang="ts">
  import { useLocalization, type MessageKey } from '../../localization';
  const l = useLocalization();
  import { onDestroy, tick } from 'svelte';
  import { loadMedia } from './viewport/funcs';
  import { playMedia, seekMedia } from '../application/funcs';
  import type { PreviewStartPort } from '../ports/types';

  let {
    src,
    filename = '',
    compact = false,
    autoplay = false,
    hideFilename = false,
    hovered = false,
    focused = false,
    nativeStart,
    onerror,
  }: {
    src: string;
    filename?: string;
    compact?: boolean;
    autoplay?: boolean;
    hideFilename?: boolean;
    hovered?: boolean;
    focused?: boolean;
    nativeStart?: PreviewStartPort;
    onerror?: () => void;
  } = $props();
  let audio: HTMLAudioElement;
  let resolved = $state<string>();
  let playing = $state(false);
  let waiting = $state(false);
  let duration = $state(0);
  let currentTime = $state(0);
  let message = $state<MessageKey | ''>('');
  let attempt = 0;
  const seekable = $derived(Number.isFinite(duration) && duration > 0);

  function time(value: number): string {
    if (!Number.isFinite(value) || value < 0) return '--:--';
    return $l.duration(value);
  }
  async function start() {
    resolved = src;
    const request = ++attempt;
    message = '';
    waiting = true;
    try {
      await tick();
      if (request === attempt) await play(request);
    } catch {
      if (request === attempt) message = 'cards.playback_could_not_start_try_again';
    } finally {
      if (request === attempt) waiting = false;
    }
  }
  async function play(request: number) {
    try {
      await playMedia(audio);
    } catch (error) {
      if (!(error instanceof DOMException) || error.name !== 'NotAllowedError' || !nativeStart) throw error;
      await nativeStart.start(async () => {
        if (request === attempt) await playMedia(audio);
      });
    }
  }
  function playbackFailed() {
    message = 'cards.playback_could_not_start_try_again';
    waiting = false;
  }
  function toggle() {
    if (playing || waiting) {
      attempt++;
      audio.pause();
      waiting = false;
    } else start().catch(playbackFailed);
  }
  $effect(() => {
    // Reset component state when an editor supplies a different source.
    const source = src;
    if (source) {
      attempt++;
      playing = false;
      waiting = false;
      duration = 0;
      currentTime = 0;
      message = '';
    }
  });
  $effect(() => {
    if (resolved === undefined || resolved.length === 0 || (!autoplay && !hovered)) return;
    start().catch(playbackFailed);
    return () => {
      attempt++;
      audio.pause();
      waiting = false;
    };
  });
  onDestroy(() => {
    attempt++;
    audio.pause();
  });
</script>

<div
  class="audio-player"
  class:compact
  use:loadMedia={{
    source: src,
    deferred: compact && !autoplay,
    active: hovered || focused,
    resolve: (source) => {
      resolved = source;
    },
  }}
>
  <audio
    bind:this={audio}
    src={resolved}
    preload={autoplay ? 'auto' : 'metadata'}
    onloadedmetadata={() => {
      duration = audio.duration;
    }}
    ondurationchange={() => {
      duration = audio.duration;
    }}
    ontimeupdate={() => {
      currentTime = audio.currentTime;
    }}
    onplay={() => {
      playing = true;
    }}
    onplaying={() => {
      waiting = false;
    }}
    onwaiting={() => {
      waiting = true;
    }}
    onpause={() => {
      attempt++;
      playing = false;
      waiting = false;
    }}
    onended={() => {
      playing = false;
      waiting = false;
    }}
    onerror={() => {
      playing = false;
      waiting = false;
      message = 'cards.this_audio_could_not_be_loaded_or_played';
      onerror?.();
    }}
  ></audio>
  <div class="audio-heading">
    <button
      type="button"
      class="play"
      onclick={toggle}
      aria-label={$l.t(playing || waiting ? 'cards.pause' : 'cards.play', { name: filename || $l.t('cards.audio') })}
    >
      <svg viewBox="0 0 24 24" width="19" height="19" fill="currentColor" aria-hidden="true">
        {#if playing}<path d="M7 5h4v14H7zm6 0h4v14h-4z" />{:else}<path d="m8 5 11 7-11 7z" />{/if}
      </svg>
    </button>
    <div class="audio-info">
      {#if !hideFilename}<span class="filename" dir="auto" title={filename}>{filename || $l.t('cards.audio')}</span
        >{/if}
      <span class="status" role="status"
        >{(message ? $l.t(message) : '') ||
          (waiting
            ? $l.t('cards.buffering')
            : playing
              ? $l.t('cards.playing')
              : !seekable
                ? $l.t('cards.loading_audio')
                : $l.t('cards.audio'))}</span
      >
    </div>
  </div>
  <div class="timeline" dir="ltr">
    <input
      type="range"
      min="0"
      max={seekable ? duration : 0}
      step="0.1"
      value={currentTime}
      disabled={!seekable}
      aria-label={$l.t('cards.seek', { name: filename || $l.t('cards.audio') })}
      aria-valuetext={$l.t('cards.duration', {
        current: time(currentTime),
        total: seekable ? time(duration) : $l.t('cards.unknown_duration'),
      })}
      oninput={(event) => {
        seekMedia(audio, event.currentTarget.valueAsNumber);
        currentTime = audio.currentTime;
      }}
    />
    <div class="times"><span>{time(currentTime)}</span><span>{seekable ? time(duration) : '--:--'}</span></div>
  </div>
</div>

<style>
  .audio-player {
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 16px;
    min-width: 0;
    padding: 18px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: #000;
    color: #fff;
  }
  audio {
    display: none;
  }
  .audio-heading {
    display: flex;
    align-items: center;
    gap: 12px;
    min-width: 0;
  }
  .play {
    display: grid;
    place-items: center;
    flex: 0 0 40px;
    width: 40px;
    height: 40px;
    padding: 0;
    border: 0;
    border-radius: 50%;
    background: #fff;
    color: #000;
    cursor: pointer;
  }
  .play:hover {
    background: #d4d4d4;
  }
  .audio-info {
    min-width: 0;
  }
  .filename {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 12px;
  }
  .status {
    display: block;
    margin-top: 5px;
    color: var(--muted);
    font-size: 10px;
    overflow-wrap: anywhere;
  }
  .timeline {
    min-width: 0;
  }
  input {
    display: block;
    width: 100%;
    min-width: 0;
    height: 20px;
    margin: 0;
    accent-color: #fff;
    cursor: pointer;
  }
  input:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .times {
    display: flex;
    justify-content: space-between;
    gap: 4px;
    color: var(--muted);
    font-size: 10px;
    font-variant-numeric: tabular-nums;
  }
  button:focus-visible,
  input:focus-visible {
    outline: 2px solid #a3a3a3;
    outline-offset: 3px;
  }
  .compact {
    justify-content: safe center;
    box-sizing: border-box;
    height: 100%;
    gap: 7px;
    padding: 10px;
    border: 0;
    border-radius: 0;
    overflow: auto;
  }
  .compact .audio-heading {
    gap: 8px;
  }
  .compact .play {
    flex-basis: 32px;
    width: 32px;
    height: 32px;
  }
</style>
