<script context="module" lang="ts">
  // Re-export for any external consumers
  export { formatBmTime } from "./AudiobookDropdowns.svelte";
</script>

<script lang="ts">
  import {
    currentAudiobook,
    abPlaybackState,
    abDurationMs,
    abFormattedPosition,
    abFormattedDuration,
    abProgress,
    abCurrentChapter,
    toggleABPlayPause,
    seekAudiobook,
    skipForward,
    skipBackward,
  } from "$lib/stores/player/audiobookPlayer";
  import { getApiBase } from "$lib/api/base";
  import type { AudiobookChapter } from "$lib/types";
  import { expanded } from "./coverExpandStore";
  import AudiobookDropdowns from "./AudiobookDropdowns.svelte";

  let dropdowns: AudiobookDropdowns;

  function toggleExpand() {
    expanded.update((v) => !v);
  }

  function onSeekFull(e: Event) {
    const pct = parseFloat((e.target as HTMLInputElement).value);
    const secs = ($abDurationMs / 1000) * (pct / 100);
    seekAudiobook(secs);
  }

  function closeDropdowns() {
    dropdowns?.closeAll();
  }

  // Chapter position as percentage within seek bar
  function chapterPct(ch: AudiobookChapter): number {
    return $abDurationMs > 0 ? (ch.start_ms / $abDurationMs) * 100 : 0;
  }

  // Seek to a specific chapter
  function seekToChapter(ch: AudiobookChapter) {
    seekAudiobook(ch.start_ms / 1000);
  }

  // Handle keyboard navigation on chapter ticks
  function onChapterKeydown(e: KeyboardEvent, ch: AudiobookChapter) {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      seekToChapter(ch);
    }
  }

  // Match browser thumb-centering: thumb doesn't overflow track ends,
  // so center = thumbR + progress/100 × (trackWidth − 2×thumbR).
  const THUMB_R = 6;
  let seekWrapWidth = 0;
  $: abFillPx =
    seekWrapWidth > 2 * THUMB_R
      ? THUMB_R + ($abProgress / 100) * (seekWrapWidth - 2 * THUMB_R)
      : (seekWrapWidth * $abProgress) / 100;
</script>

<svelte:window on:click={closeDropdowns} />

<footer class="ab-bar bottom-bar">
  <!-- Left: cover + metadata — fixed sidebar width -->
  <div class="ab-info">
    {#if $currentAudiobook}
      {#if !$expanded}
        <div class="ab-cover-hover">
          {#if $currentAudiobook.cover_art_key}
            <img
              src="{getApiBase()}/covers/audiobook/{$currentAudiobook.id}"
              alt="cover"
              class="ab-cover"
            />
          {:else}
            <div class="ab-cover ab-cover-placeholder">
              <svg
                width="18"
                height="18"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" /><path
                  d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"
                />
              </svg>
            </div>
          {/if}
          <button
            class="ab-cover-expand-btn"
            on:click={toggleExpand}
            aria-label="Expand cover"
          >
            <svg width="16" height="16" viewBox="0 0 20 20"
              ><path
                d="M4 4h12v12H4V4zm2 2v8h8V6H6z"
                fill="currentColor"
              /></svg
            >
          </button>
        </div>
      {/if}
      <div class="ab-meta" class:full-width={$expanded}>
        <div class="ab-title">{$currentAudiobook.title}</div>
        <div class="ab-sub">
          {#if $currentAudiobook.author_name}
            <span class="ab-author">{$currentAudiobook.author_name}</span>
          {/if}
          {#if $abCurrentChapter}
            <span class="ab-chapter-name">· {$abCurrentChapter.title}</span>
          {/if}
        </div>
      </div>
    {:else}
      <div class="ab-cover ab-cover-placeholder">
        <svg
          width="18"
          height="18"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.5"
        >
          <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" /><path
            d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"
          />
        </svg>
      </div>
      <div class="ab-meta">
        <div class="skeleton-line sk-title"></div>
        <div class="skeleton-line sk-sub"></div>
      </div>
    {/if}
  </div>

  <!-- Playback: transport controls + seek + right -->
  <div class="ab-playback">
    <!-- Transport controls: skip back, play/pause, skip forward -->
    <div class="ab-controls">
      <button
        class="ctrl-btn"
        on:click={() => skipBackward(10)}
        title="Back 10 s"
        aria-label="Skip back 10 seconds"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
          <path
            d="M12 5V1L7 6l5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6H4c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z"
          />
          <text
            x="12"
            y="15.5"
            text-anchor="middle"
            font-size="5.5"
            font-family="sans-serif"
            font-weight="bold"
            fill="currentColor">10</text
          >
        </svg>
      </button>

      <button
        class="ctrl-btn play-btn"
        on:click={toggleABPlayPause}
        aria-label={$abPlaybackState === "playing" ? "Pause" : "Play"}
      >
        {#if $abPlaybackState === "playing"}
          <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor">
            <rect x="6" y="4" width="4" height="16" rx="1" /><rect
              x="14"
              y="4"
              width="4"
              height="16"
              rx="1"
            />
          </svg>
        {:else if $abPlaybackState === "loading"}
          <div class="spin-ring"></div>
        {:else}
          <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor">
            <polygon points="5,3 19,12 5,21" />
          </svg>
        {/if}
      </button>

      <button
        class="ctrl-btn"
        on:click={() => skipForward(30)}
        title="Forward 30 s"
        aria-label="Skip forward 30 seconds"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
          <path
            d="M18 13c0 3.31-2.69 6-6 6s-6-2.69-6-6 2.69-6 6-6v4l5-5-5-5v4c-4.42 0-8 3.58-8 8s3.58 8 8 8 8-3.58 8-8h-2z"
          />
          <text
            x="12"
            y="15.5"
            text-anchor="middle"
            font-size="5.5"
            font-family="sans-serif"
            font-weight="bold"
            fill="currentColor">30</text
          >
        </svg>
      </button>
    </div>

    <!-- Seek bar (chapter-aware) -->
    <div class="ab-center">
      <div class="ab-seek-row">
        <span class="time">{$abFormattedPosition}</span>
        <div class="ab-seek-wrap" bind:clientWidth={seekWrapWidth}>
          <!-- Chapter tick marks -->
          {#if $currentAudiobook?.chapters?.length}
            {#each $currentAudiobook.chapters.slice(1) as ch (ch.id)}
              <div
                class="chapter-tick"
                style="left: {chapterPct(ch)}%"
                on:click={() => seekToChapter(ch)}
                on:keydown={(e) => onChapterKeydown(e, ch)}
                role="button"
                tabindex="0"
                aria-label="Seek to {ch.title}"
              >
                <span class="chapter-tick-label">{ch.title}</span>
              </div>
            {/each}
          {/if}
          <div class="seek-track">
            <div class="seek-fill" style="width: {abFillPx}px"></div>
          </div>
          <input
            type="range"
            min="0"
            max="100"
            step="0.05"
            value={$abProgress}
            on:input={onSeekFull}
            class="seek-input"
            aria-label="Seek"
          />
        </div>
        <span class="time">{$abFormattedDuration}</span>
      </div>
    </div>

    <!-- Right: speed, sleep, chapters, bookmarks, volume -->
    <AudiobookDropdowns bind:this={dropdowns} />
  </div>
</footer>

<style>
  .ab-bar {
    display: flex;
    align-items: center;
    height: var(--bottom-h);
    background: var(--bg-elevated);
    border-top: 1px solid var(--border);
    flex-shrink: 0;
    gap: 0;
  }

  /* ── Left info ────────────────────────────────────────── */
  .ab-info {
    width: var(--sidebar-w);
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 8px 0 16px;
    min-width: 0;
  }

  /* ── Playback section (mirrors music bar .playback-section) ── */
  .ab-playback {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 16px 0 0;
    min-width: 0;
  }

  /* Expandable cover hover wrap */
  .ab-cover-hover {
    position: relative;
    flex-shrink: 0;
    width: 44px;
    height: 44px;
  }
  .ab-cover-hover .ab-cover-expand-btn {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.55);
    border: none;
    border-radius: 4px;
    color: #fff;
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.15s;
  }
  .ab-cover-hover:hover .ab-cover-expand-btn {
    opacity: 1;
  }

  .ab-meta.full-width {
    flex: 1;
    min-width: 0;
  }

  .ab-cover {
    width: 44px;
    height: 44px;
    border-radius: 4px;
    object-fit: cover;
    display: block;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
    background: var(--bg-hover);
  }
  .ab-cover-placeholder {
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
  }

  .ab-meta {
    min-width: 0;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .ab-title {
    font-size: 0.88rem;
    font-weight: 500;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .ab-sub {
    font-size: 0.75rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    display: flex;
    gap: 4px;
  }
  .ab-chapter-name {
    opacity: 0.75;
  }

  /* ── Center (seek bar, now inside ab-playback) ───────── */
  .ab-center {
    flex: 1;
    display: flex;
    align-items: center;
    min-width: 0;
  }

  /* Transport controls (now inside ab-playback) */
  .ab-controls {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 2px;
  }

  .ctrl-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 4px 6px;
    height: 30px;
    transition: color 0.15s;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 4px;
  }
  .ctrl-btn:hover {
    color: var(--text);
  }

  .play-btn {
    color: var(--text);
    width: 38px;
    height: 38px;
    border-radius: 50%;
    background: var(--bg-hover);
    transition:
      background 0.15s,
      color 0.15s;
  }
  .play-btn:hover {
    background: var(--accent);
    color: #fff;
  }

  /* Seek bar */
  .ab-seek-row {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
  }
  .time {
    font-size: 0.72rem;
    color: var(--text-muted);
    width: 38px;
    flex-shrink: 0;
  }
  .time:last-child {
    text-align: right;
  }

  .ab-seek-wrap {
    flex: 1;
    position: relative;
    height: 20px;
    display: flex;
    align-items: center;
  }
  .seek-track {
    position: absolute;
    left: 0;
    right: 0;
    height: 4px;
    background: var(--bg-hover);
    border-radius: 2px;
    overflow: hidden;
    pointer-events: none;
  }
  .seek-fill {
    height: 100%;
    background: var(--accent);
    pointer-events: none;
  }
  .seek-input {
    position: absolute;
    left: 0;
    right: 0;
    width: 100%;
    margin: 0;
    height: 20px;
    cursor: pointer;
    -webkit-appearance: none;
    appearance: none;
    background: transparent;
    z-index: 2;
  }
  .seek-input::-webkit-slider-runnable-track {
    background: transparent;
    height: 4px;
  }
  .seek-input::-moz-range-track {
    background: transparent;
    height: 4px;
    border: none;
  }
  .seek-input::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--accent);
    margin-top: -4px;
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.15s;
  }
  .seek-input::-moz-range-thumb {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--accent);
    border: none;
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.15s;
  }
  .ab-seek-wrap:hover .seek-track {
    height: 6px;
  }
  .ab-seek-wrap:hover .seek-input::-webkit-slider-thumb {
    opacity: 1;
  }
  .ab-seek-wrap:hover .seek-input::-moz-range-thumb {
    opacity: 1;
  }

  /* Chapter tick marks */
  .chapter-tick {
    position: absolute;
    top: 50%;
    transform: translateX(-50%) translateY(-50%);
    width: 14px;
    height: 20px;
    pointer-events: auto;
    cursor: pointer;
    z-index: 3;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .chapter-tick::before {
    content: "";
    width: 2px;
    height: 10px;
    background: var(--text-muted);
    opacity: 0.5;
    border-radius: 1px;
    transition:
      opacity 0.15s,
      background 0.15s;
  }
  .chapter-tick:hover::before {
    opacity: 1;
    background: var(--accent);
  }
  .chapter-tick-label {
    position: absolute;
    bottom: calc(100% + 6px);
    left: 50%;
    transform: translateX(-50%);
    white-space: nowrap;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 3px 8px;
    font-size: 0.72rem;
    color: var(--text);
    pointer-events: none;
    opacity: 0;
    transition: opacity 0.15s;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.25);
  }
  .chapter-tick:hover .chapter-tick-label {
    opacity: 1;
  }

  /* Loading spinner */
  .spin-ring {
    width: 20px;
    height: 20px;
    border: 2px solid var(--bg-hover);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  /* Skeleton */
  @keyframes pulse {
    0%,
    100% {
      opacity: 1;
    }
    50% {
      opacity: 0.4;
    }
  }
  .skeleton-line {
    background: var(--bg-hover);
    border-radius: 3px;
    animation: pulse 1.6s ease-in-out infinite;
  }
  .sk-title {
    width: 130px;
    height: 12px;
  }
  .sk-sub {
    width: 80px;
    height: 10px;
  }

  /* Hide on mobile — replaced by MobileAudiobookPlayer */
  @media (max-width: 640px) {
    .ab-bar {
      display: none;
    }
  }
</style>
