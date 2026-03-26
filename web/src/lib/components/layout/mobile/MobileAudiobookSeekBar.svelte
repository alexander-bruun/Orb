<script lang="ts">
  import {
    currentAudiobook,
    abProgress,
    abDurationMs,
    abFormattedPosition,
    abFormattedDuration,
    abCurrentChapter,
    abChapterProgress,
    abPreviousChapter,
    abNextChapter,
    jumpToChapter,
    seekAudiobook,
    seekAudiobookMs,
  } from '$lib/stores/player/audiobookPlayer';

  function onSeek(e: Event) {
    const pct = parseFloat((e.target as HTMLInputElement).value);
    if ($abCurrentChapter) {
      const nextChapter = $currentAudiobook?.chapters?.find(ch => ch.start_ms > $abCurrentChapter!.start_ms);
      const chDurationMs = nextChapter
        ? nextChapter.start_ms - $abCurrentChapter.start_ms
        : ($currentAudiobook?.duration_ms ?? 0) - $abCurrentChapter.start_ms;
      seekAudiobookMs($abCurrentChapter.start_ms + (pct / 100) * chDurationMs);
    } else {
      seekAudiobook(($abDurationMs / 1000) * (pct / 100));
    }
  }
</script>

<div class="fs-seek">
  {#if $abCurrentChapter}
    <!-- Chapter info header -->
    <div class="chapter-nav-info">
      {#if $abPreviousChapter}
        
        <button class="chapter-nav prev-nav" on:click|stopPropagation={() => jumpToChapter($abPreviousChapter)} title={$abPreviousChapter.title} aria-label="Previous chapter: {$abPreviousChapter.title}">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="15 18 9 12 15 6"></polyline>
          </svg>
          <span>{$abPreviousChapter.title}</span>
        </button>
      {:else}
        <div class="chapter-nav-spacer"></div>
      {/if}
      <div class="current-chapter">{$abCurrentChapter.title}</div>
      {#if $abNextChapter}
        
        <button class="chapter-nav next-nav" on:click|stopPropagation={() => jumpToChapter($abNextChapter)} title={$abNextChapter.title} aria-label="Next chapter: {$abNextChapter.title}">
          <span>{$abNextChapter.title}</span>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="9 18 15 12 9 6"></polyline>
          </svg>
        </button>
      {:else}
        <div class="chapter-nav-spacer"></div>
      {/if}
    </div>

    <!-- Seek bar for current chapter -->
    <div class="seek-bar-wrap">
      <div class="seek-track">
        <div class="seek-fill" style="width: {$abChapterProgress}%"></div>
      </div>
      <input
        type="range"
        min="0"
        max="100"
        step="0.05"
        value={$abChapterProgress}
        on:input={onSeek}
        on:touchstart|stopPropagation
        on:touchmove|stopPropagation
        on:touchend|stopPropagation
        class="seek-input"
        aria-label="Seek in chapter"
      />
    </div>
  {:else}
    <!-- Fallback to overall progress -->
    <div class="seek-bar-wrap">
      <div class="seek-track">
        <div class="seek-fill" style="width: {$abProgress}%"></div>
      </div>
      <input
        type="range"
        min="0"
        max="100"
        step="0.05"
        value={$abProgress}
        on:input={onSeek}
        on:touchstart|stopPropagation
        on:touchmove|stopPropagation
        on:touchend|stopPropagation
        class="seek-input"
        aria-label="Seek"
      />
    </div>
  {/if}

  <div class="seek-times">
    <span>{$abFormattedPosition}</span>
    <span>{$abFormattedDuration}</span>
  </div>
</div>

<style>
  @media (max-width: 640px) {
    .fs-seek {
      flex-shrink: 0;
      padding: 4px 0 12px;
    }

    .chapter-nav-info {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      margin-bottom: 8px;
      font-size: 0.7rem;
      color: rgba(255, 255, 255, 0.55);
    }

    .current-chapter {
      flex: 1;
      text-align: center;
      color: rgba(255, 255, 255, 0.75);
      font-weight: 500;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .chapter-nav {
      flex: 0 0 35%;
      display: flex;
      align-items: center;
      gap: 4px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      opacity: 0.6;
      background: none;
      border: none;
      color: inherit;
      font: inherit;
      cursor: pointer;
      padding: 0;
      -webkit-tap-highlight-color: transparent;
    }

    .chapter-nav:active {
      opacity: 0.9;
    }

    .chapter-nav-spacer {
      flex: 0 0 35%;
    }

    .prev-nav {
      justify-content: flex-start;
      text-align: right;
      flex-direction: row-reverse;
    }

    .next-nav {
      justify-content: flex-end;
    }

    .seek-bar-wrap {
      position: relative;
      height: 4px;
      display: flex;
      align-items: center;
      margin-bottom: 8px;
    }

    .seek-track {
      position: absolute;
      left: 0; right: 0;
      height: 4px;
      background: rgba(255, 255, 255, 0.2);
      border-radius: 2px;
    }

    .seek-fill {
      position: absolute;
      height: 100%;
      background: #fff;
      border-radius: 2px;
      transition: width 0.22s linear;
    }

    .seek-input {
      position: absolute;
      left: -8px; right: -8px;
      width: calc(100% + 16px);
      height: 28px;
      margin: 0;
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
      width: 16px; height: 16px; border-radius: 50%; background: #fff; margin-top: -6px;
    }
    .seek-input::-moz-range-thumb {
      width: 16px; height: 16px; border-radius: 50%; background: #fff; border: none;
    }

    .seek-times {
      display: flex;
      justify-content: space-between;
      font-size: 0.72rem;
      color: rgba(255, 255, 255, 0.55);
      font-variant-numeric: tabular-nums;
    }
  }
</style>
