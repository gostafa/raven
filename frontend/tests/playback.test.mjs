import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createPreviewPlayback, playMedia, seekMedia } from '../src/features/cards/application/funcs.ts';
import { createNativePreviewStarter } from '../src/features/cards/adapters/preview/funcs.ts';

function nativeEvents() {
  const listeners = new Map();
  const emitted = [];
  let key = 0;
  let insideNativeCallback = false;
  const starter = createNativePreviewStarter({
    once(name, callback) {
      listeners.set(name, callback);
      return () => listeners.delete(name);
    },
    async emit(name) {
      emitted.push(name);
    },
    newKey: () => String(++key),
  });
  return {
    starter,
    emitted,
    listeners,
    allowed: () => insideNativeCallback,
    deliver() {
      insideNativeCallback = true;
      try {
        listeners.get(emitted.shift())?.();
      } finally {
        insideNativeCallback = false;
      }
    },
  };
}

test('a WebKit permission rejection starts through the native event callback without a click', async () => {
  const native = nativeEvents();
  const video = media({
    async play() {
      if (!native.allowed()) throw Object.assign(new Error('native context required'), { name: 'NotAllowedError' });
      this.paused = false;
    },
  });
  const preview = createPreviewPlayback(video, native.starter);
  const pending = preview.start();
  await new Promise((resolve) => queueMicrotask(resolve));
  assert.equal(video.paused, true);
  assert.equal(native.emitted.length, 1);
  native.deliver();
  await pending;
  assert.equal(video.paused, false);
  assert.equal(native.listeners.size, 0);
  preview.stop();
});

test('a queued native start cannot play after leaving, even after another hover begins', async () => {
  const native = nativeEvents();
  let plays = 0;
  const video = media({
    async play() {
      if (!native.allowed()) throw Object.assign(new Error('native context required'), { name: 'NotAllowedError' });
      plays++;
      this.paused = false;
    },
  });
  const preview = createPreviewPlayback(video, native.starter);
  const first = preview.start();
  await new Promise((resolve) => queueMicrotask(resolve));
  preview.stop();
  const second = preview.start();
  await new Promise((resolve) => queueMicrotask(resolve));
  native.deliver();
  await first;
  assert.equal(plays, 0);
  native.deliver();
  await second;
  assert.equal(plays, 1);
  preview.stop();
});

test('a failed native event request removes its listener and rejects the start', async () => {
  let removed = false;
  const starter = createNativePreviewStarter({
    once: () => () => (removed = true),
    emit: async () => {
      throw new Error('native window unavailable');
    },
    newKey: () => 'failed',
  });
  await assert.rejects(
    starter.start(async () => assert.fail('must not play')),
    /native window unavailable/,
  );
  assert.equal(removed, true);
});

function media(overrides = {}) {
  return {
    muted: false,
    defaultMuted: false,
    currentTime: 12,
    duration: 30,
    ended: false,
    paused: true,
    async play() {
      this.paused = false;
    },
    pause() {
      this.paused = true;
    },
    ...overrides,
  };
}

test('a fresh video starts silently without a click granting playback permission', async () => {
  const video = media({
    async play() {
      if (!this.defaultMuted || !this.muted) throw new Error('NotAllowedError: click required');
      this.paused = false;
    },
  });
  const preview = createPreviewPlayback(video);
  await preview.start();
  assert.equal(video.defaultMuted, true);
  assert.equal(video.muted, true);
  assert.equal(video.paused, false);
});

test('video hover playback starts, then pauses and resets on leaving or cleanup', async () => {
  const video = media();
  const preview = createPreviewPlayback(video);
  await preview.start();
  assert.equal(video.paused, false);
  preview.stop();
  assert.equal(video.paused, true);
  assert.equal(video.currentTime, 0);
  preview.stop();
  assert.equal(video.currentTime, 0);
});

test('a delayed playback request cannot restart a preview after leaving', async () => {
  let complete;
  const video = media({
    play() {
      return new Promise((resolve) => {
        complete = () => {
          this.paused = false;
          resolve();
        };
      });
    },
  });
  const preview = createPreviewPlayback(video);
  const pending = preview.start();
  preview.stop();
  complete();
  await pending;
  assert.equal(video.paused, true);
  assert.equal(video.currentTime, 0);
});

test('leaving a video retains a decoded idle frame even if a pending start completes', async () => {
  let complete;
  const video = media({
    play() {
      return new Promise((resolve) => {
        complete = resolve;
      });
    },
  });
  const preview = createPreviewPlayback(video);
  const pending = preview.start();
  preview.stop(0.1);
  assert.equal(video.currentTime, 0.1);
  complete();
  await pending;
  assert.equal(video.paused, true);
  assert.equal(video.currentTime, 0.1);
  await preview.retry();
  assert.equal(video.paused, true);
});

test('blocked preview playback is handled and can be retried', async () => {
  const video = media({
    async play() {
      throw new Error('autoplay denied');
    },
  });
  const preview = createPreviewPlayback(video);
  await assert.doesNotReject(preview.start());
  assert.equal(video.paused, true);
  video.play = async function () {
    this.paused = false;
  };
  await preview.start();
  assert.equal(video.paused, false);
});

test('old playback completions do not stop a newly requested preview', async () => {
  const completions = [];
  const video = media({
    play() {
      return new Promise((resolve) => completions.push(resolve));
    },
  });
  const preview = createPreviewPlayback(video);
  const first = preview.start();
  preview.stop();
  video.currentTime = 5;
  const second = preview.start();
  completions[0]();
  await first;
  assert.equal(video.currentTime, 5);
  completions[1]();
  await second;
});

test('video readiness retries a failed hover start without requiring another hover', async () => {
  const video = media({
    async play() {
      throw new Error('not ready');
    },
  });
  const preview = createPreviewPlayback(video);
  await preview.start();
  video.play = async function () {
    this.paused = false;
  };
  await preview.retry();
  assert.equal(video.paused, false);
  video.ended = true;
  video.currentTime = video.duration;
  await preview.retry();
  assert.equal(video.currentTime, 0);
  assert.equal(video.paused, false);
  preview.stop();
  await preview.retry();
  assert.equal(video.paused, true);
  assert.equal(video.currentTime, 0);
});

test('audio seeking clamps values and ignores unavailable duration or invalid input', () => {
  const audio = media();
  seekMedia(audio, 17);
  assert.equal(audio.currentTime, 17);
  seekMedia(audio, 40);
  assert.equal(audio.currentTime, 30);
  seekMedia(audio, -2);
  assert.equal(audio.currentTime, 0);
  for (const duration of [0, NaN, Infinity]) {
    audio.duration = duration;
    seekMedia(audio, 4);
    assert.equal(audio.currentTime, 0);
  }
  audio.duration = 30;
  seekMedia(audio, NaN);
  assert.equal(audio.currentTime, 0);
});

test('audio resumes from its position and restarts after ending; failures reach the UI', async () => {
  const audio = media();
  await playMedia(audio);
  assert.equal(audio.currentTime, 12);
  audio.ended = true;
  await playMedia(audio);
  assert.equal(audio.currentTime, 0);
  audio.play = async () => {
    throw new Error('unsupported media');
  };
  await assert.rejects(playMedia(audio), /unsupported media/);
});
