<script context="module" lang="ts">
  function formatBmTime(ms: number): string {
    const s = Math.floor(ms / 1000);
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = s % 60;
    if (h > 0)
      return `${h}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
    return `${m}:${String(sec).padStart(2, "0")}`;
  }
</script>

<script lang="ts">
  import {
    currentAudiobook,
    abPlaybackState,
    abPositionMs,
    abDurationMs,
    abFormattedPosition,
    abFormattedDuration,
    abProgress,
    abCurrentChapter,
    abSpeed,
    abVolume,
    abBookmarks,
    sleepTimerMins,
    toggleABPlayPause,
    seekAudiobook,
    skipForward,
    skipBackward,
    setABSpeed,
    setABVolume,
    setSleepTimer,
    jumpToChapter,
    createBookmark,
    deleteBookmark,
    AB_SPEEDS,
    SLEEP_PRESETS,
  } from "$lib/stores/audiobookPlayer";
  import { getApiBase } from "$lib/api/base";
  import type { AudiobookChapter } from "$lib/types";
  import { expanded } from "./coverExpandStore";

  function toggleExpand() {
    expanded.update((v) => !v);
  }

  function onSeek(e: Event) {
    const pct = parseFloat((e.target as HTMLInputElement).value);
    const secs = ($abDurationMs / 1000) * (pct / 100);
    seekAudiobook(secs);
  }

  function onVolume(e: Event) {
    setABVolume(parseFloat((e.target as HTMLInputElement).value));
  }

  let speedOpen = false;
  let sleepOpen = false;
  let chapterOpen = false;
  let bookmarkOpen = false;

  function closeDropdowns() {
    speedOpen = false;
    sleepOpen = false;
    chapterOpen = false;
    bookmarkOpen = false;
  }

  // Chapter position as percentage within seek bar
  function chapterPct(ch: AudiobookChapter): number {
    return $abDurationMs > 0 ? (ch.start_ms / $abDurationMs) * 100 : 0;
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
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
        <svg
          width="18"
          height="18"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <polyline points="1 4 1 10 7 10" /><path
            d="M3.51 15a9 9 0 1 0 .49-3.5"
          />
          <text
            x="7"
            y="14"
            font-size="6"
            fill="currentColor"
            stroke="none"
            font-family="sans-serif"
            font-weight="bold">10</text
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
        <svg
          width="18"
          height="18"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <polyline points="23 4 23 10 17 10" /><path
            d="M20.49 15a9 9 0 1 1-.49-3.5"
          />
          <text
            x="7"
            y="14"
            font-size="6"
            fill="currentColor"
            stroke="none"
            font-family="sans-serif"
            font-weight="bold">30</text
          >
        </svg>
      </button>
    </div>

    <!-- Seek bar -->
    <div class="ab-center">
      <div class="ab-seek-row">
        <span class="time">{$abFormattedPosition}</span>
        <div class="ab-seek-wrap">
          <!-- Chapter tick marks -->
          {#if $currentAudiobook?.chapters?.length}
            {#each $currentAudiobook.chapters.slice(1) as ch (ch.id)}
              <div class="chapter-tick" style="left: {chapterPct(ch)}%">
                <span class="chapter-tick-label">{ch.title}</span>
              </div>
            {/each}
          {/if}
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
            class="seek-input"
            aria-label="Seek"
          />
        </div>
        <span class="time">{$abFormattedDuration}</span>
      </div>
    </div>

    <!-- Right: speed, sleep, chapters, bookmarks, volume -->
    <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
    <div class="ab-right" on:click|stopPropagation>
      <!-- Speed picker -->
      <div class="picker-wrap">
        <button
          class="ctrl-btn speed-btn"
          class:active={$abSpeed !== 1.0}
          on:click|stopPropagation={() => {
            speedOpen = !speedOpen;
            sleepOpen = false;
            chapterOpen = false;
            bookmarkOpen = false;
          }}
          title="Playback speed"
          aria-label="Playback speed"
        >
          {$abSpeed === 1.0 ? "1×" : `${$abSpeed}×`}
        </button>
        {#if speedOpen}
          <div class="picker-popup speed-popup">
            <div class="picker-header">Speed</div>
            {#each AB_SPEEDS as s (s)}
              <button
                class="picker-item"
                class:is-active={$abSpeed === s}
                on:click={() => {
                  setABSpeed(s);
                  speedOpen = false;
                }}>{s}×</button
              >
            {/each}
          </div>
        {/if}
      </div>

      <!-- Sleep timer -->
      <div class="picker-wrap">
        <button
          class="ctrl-btn icon-btn"
          class:active={$sleepTimerMins > 0}
          on:click|stopPropagation={() => {
            sleepOpen = !sleepOpen;
            speedOpen = false;
            chapterOpen = false;
            bookmarkOpen = false;
          }}
          title={$sleepTimerMins > 0
            ? `Sleep in ${$sleepTimerMins} min`
            : "Sleep timer"}
          aria-label="Sleep timer"
        >
          <svg
            width="15"
            height="15"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
          </svg>
          {#if $sleepTimerMins > 0}
            <span class="sleep-badge">{$sleepTimerMins}</span>
          {/if}
        </button>
        {#if sleepOpen}
          <div class="picker-popup">
            <div class="picker-header">Sleep timer</div>
            <button
              class="picker-item"
              class:is-active={$sleepTimerMins === 0}
              on:click={() => {
                setSleepTimer(0);
                sleepOpen = false;
              }}>Off</button
            >
            {#each SLEEP_PRESETS as mins (mins)}
              <button
                class="picker-item"
                class:is-active={$sleepTimerMins === mins}
                on:click={() => {
                  setSleepTimer(mins);
                  sleepOpen = false;
                }}>{mins} min</button
              >
            {/each}
          </div>
        {/if}
      </div>

      <!-- Volume -->
      <input
        type="range"
        min="0"
        max="1"
        step="0.01"
        value={$abVolume}
        on:input={onVolume}
        class="volume-bar"
        aria-label="Volume"
      />

      <!-- Chapter list -->
      {#if $currentAudiobook?.chapters?.length}
        <div class="picker-wrap">
          <button
            class="ctrl-btn icon-btn"
            class:active={chapterOpen}
            on:click|stopPropagation={() => {
              chapterOpen = !chapterOpen;
              speedOpen = false;
              sleepOpen = false;
              bookmarkOpen = false;
            }}
            title="Chapters"
            aria-label="Chapters"
          >
            <svg
              width="15"
              height="15"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <line x1="8" y1="6" x2="21" y2="6" /><line
                x1="8"
                y1="12"
                x2="21"
                y2="12"
              />
              <line x1="8" y1="18" x2="21" y2="18" /><line
                x1="3"
                y1="6"
                x2="3.01"
                y2="6"
              />
              <line x1="3" y1="12" x2="3.01" y2="12" /><line
                x1="3"
                y1="18"
                x2="3.01"
                y2="18"
              />
            </svg>
          </button>
          {#if chapterOpen}
            <div class="picker-popup chapter-popup">
              <div class="picker-header">Chapters</div>
              <div class="chapter-list">
                {#each $currentAudiobook.chapters as ch (ch.id)}
                  {@const isActive = $abCurrentChapter?.id === ch.id}
                  <button
                    class="chapter-item"
                    class:is-active={isActive}
                    on:click={() => {
                      jumpToChapter(ch);
                      chapterOpen = false;
                    }}
                  >
                    <span class="chapter-num">{ch.chapter_num + 1}</span>
                    <span class="chapter-title">{ch.title}</span>
                    {#if isActive}
                      <span class="chapter-dot"></span>
                    {/if}
                  </button>
                {/each}
              </div>
            </div>
          {/if}
        </div>
      {/if}

      <!-- Bookmarks -->
      <div class="picker-wrap">
        <button
          class="ctrl-btn icon-btn"
          class:active={$abBookmarks.length > 0}
          on:click|stopPropagation={() => {
            bookmarkOpen = !bookmarkOpen;
            speedOpen = false;
            sleepOpen = false;
            chapterOpen = false;
          }}
          title="Bookmarks"
          aria-label="Bookmarks"
        >
          <svg
            width="15"
            height="15"
            viewBox="0 0 24 24"
            fill={$abBookmarks.length > 0 ? "currentColor" : "none"}
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z" />
          </svg>
        </button>
        {#if bookmarkOpen}
          <div class="picker-popup bookmark-popup">
            <div class="picker-header">Bookmarks</div>
            <button
              class="picker-item add-bm"
              on:click={() => {
                createBookmark();
                bookmarkOpen = false;
              }}
            >
              <svg
                width="12"
                height="12"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2.5"
                ><line x1="12" y1="5" x2="12" y2="19" /><line
                  x1="5"
                  y1="12"
                  x2="19"
                  y2="12"
                /></svg
              >
              Bookmark here
            </button>
            {#if $abBookmarks.length === 0}
              <p class="picker-empty">No bookmarks yet</p>
            {:else}
              {#each $abBookmarks as bm (bm.id)}
                <div class="bm-item">
                  <button
                    class="bm-jump"
                    on:click={() => {
                      seekAudiobook(bm.position_ms / 1000);
                      bookmarkOpen = false;
                    }}
                  >
                    <span class="bm-time">{formatBmTime(bm.position_ms)}</span>
                    {#if bm.note}<span class="bm-note">{bm.note}</span>{/if}
                  </button>
                  <button
                    class="bm-del"
                    on:click={() => deleteBookmark(bm.id)}
                    aria-label="Delete bookmark"
                  >
                    <svg
                      width="11"
                      height="11"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2.5"
                      ><line x1="18" y1="6" x2="6" y2="18" /><line
                        x1="6"
                        y1="6"
                        x2="18"
                        y2="18"
                      /></svg
                    >
                  </button>
                </div>
              {/each}
            {/if}
          </div>
        {/if}
      </div>
    </div>
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

  .icon-btn {
    font-size: 0;
    padding: 6px;
    position: relative;
  }
  .icon-btn.active {
    color: var(--accent);
  }
  .icon-btn.active::after {
    content: "";
    position: absolute;
    bottom: 2px;
    left: 50%;
    transform: translateX(-50%);
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: var(--accent);
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
    cursor: default;
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

  /* ── Right controls ───────────────────────────────────── */
  .ab-right {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .speed-btn {
    font-size: 0.78rem;
    font-weight: 600;
    letter-spacing: -0.02em;
    color: var(--text-muted);
    padding: 4px 6px;
    border-radius: 4px;
    height: 26px;
    min-width: 30px;
  }
  .speed-btn.active {
    color: var(--accent);
  }

  .sleep-badge {
    position: absolute;
    top: 1px;
    right: 0;
    font-size: 8px;
    font-weight: 700;
    line-height: 1;
    color: var(--accent);
    pointer-events: none;
  }

  .volume-bar {
    width: 76px;
    height: 4px;
    accent-color: var(--accent);
    cursor: pointer;
  }

  /* ── Pickers (shared) ─────────────────────────────────── */
  .picker-wrap {
    position: relative;
  }

  .picker-popup {
    position: absolute;
    bottom: calc(100% + 10px);
    right: 0;
    min-width: 140px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.35);
    z-index: 1000;
    overflow: hidden;
  }
  .picker-header {
    font-size: 0.68rem;
    font-weight: 600;
    letter-spacing: 0.07em;
    text-transform: uppercase;
    color: var(--text-muted);
    padding: 8px 12px 4px;
  }
  .picker-item {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 7px 12px;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    font-size: 0.83rem;
    color: var(--text-muted);
    transition:
      background 0.1s,
      color 0.1s;
  }
  .picker-item:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .picker-item.is-active {
    color: var(--accent);
    font-weight: 600;
  }
  .picker-empty {
    font-size: 0.78rem;
    color: var(--text-muted);
    padding: 6px 12px 10px;
    margin: 0;
  }

  /* Speed popup: center-aligned numbers */
  .speed-popup {
    min-width: 90px;
  }
  .speed-popup .picker-item {
    justify-content: center;
  }

  /* Chapter popup: wider, scrollable */
  .chapter-popup {
    min-width: 220px;
    right: 0;
  }
  .chapter-list {
    max-height: 260px;
    overflow-y: auto;
  }
  .chapter-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 7px 12px;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
  }
  .chapter-item:hover {
    background: var(--bg-hover);
  }
  .chapter-item.is-active {
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .chapter-num {
    font-size: 0.7rem;
    color: var(--text-muted);
    width: 18px;
    flex-shrink: 0;
    text-align: right;
  }
  .chapter-title {
    font-size: 0.82rem;
    color: var(--text);
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .chapter-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
    flex-shrink: 0;
  }

  /* Bookmark popup */
  .bookmark-popup {
    min-width: 200px;
  }
  .add-bm {
    color: var(--accent);
    font-weight: 500;
    border-bottom: 1px solid var(--border);
  }
  .bm-item {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px 2px 0;
  }
  .bm-jump {
    flex: 1;
    display: flex;
    gap: 8px;
    padding: 6px 12px;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
  }
  .bm-jump:hover {
    background: var(--bg-hover);
  }
  .bm-time {
    font-size: 0.78rem;
    color: var(--accent);
    font-variant-numeric: tabular-nums;
  }
  .bm-note {
    font-size: 0.78rem;
    color: var(--text-muted);
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .bm-del {
    flex-shrink: 0;
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 4px 8px;
    opacity: 0;
    transition:
      opacity 0.15s,
      color 0.15s;
    display: inline-flex;
    align-items: center;
  }
  .bm-item:hover .bm-del {
    opacity: 1;
  }
  .bm-del:hover {
    color: var(--error, #e55);
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
