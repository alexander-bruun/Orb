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
    setSleepTimer,
    jumpToChapter,
    createBookmark,
    deleteBookmark,
    closeAudiobook,
    AB_SPEEDS,
    SLEEP_PRESETS,
  } from '$lib/stores/audiobookPlayer';
  import { getApiBase } from '$lib/api/base';
  import type { AudiobookChapter } from '$lib/types';

  // Expanded full-screen view vs. mini player
  let expanded = false;

  function onSeek(e: Event) {
    const pct = parseFloat((e.target as HTMLInputElement).value);
    seekAudiobook(($abDurationMs / 1000) * (pct / 100));
  }

  let showChapters = false;
  let showSpeed    = false;
  let showSleep    = false;
  let showBookmarks = false;

  function closeSheets() {
    showChapters = false;
    showSpeed    = false;
    showSleep    = false;
    showBookmarks = false;
  }

  function chapterPct(ch: AudiobookChapter): number {
    return $abDurationMs > 0 ? (ch.start_ms / $abDurationMs) * 100 : 0;
  }

  function fmtMs(ms: number): string {
    const s = Math.floor(ms / 1000);
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = s % 60;
    if (h > 0) return `${h}:${String(m).padStart(2,'0')}:${String(sec).padStart(2,'0')}`;
    return `${m}:${String(sec).padStart(2,'0')}`;
  }
</script>

{#if $currentAudiobook}
  {#if expanded}
    <!-- ── Full-screen player ───────────────────────────────────────────── -->
    <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
    <div class="ab-full" on:click={closeSheets}>

      <!-- Header -->
      <div class="full-header">
        <button class="pill-btn" on:click|stopPropagation={() => expanded = false} aria-label="Collapse">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
            <polyline points="18 15 12 9 6 15"/>
          </svg>
        </button>
        <span class="full-type-label">Audiobook</span>
        <button class="pill-btn" on:click|stopPropagation={closeAudiobook} aria-label="Close">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
            <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
          </svg>
        </button>
      </div>

      <!-- Cover art -->
      <div class="full-cover-wrap">
        {#if $currentAudiobook.cover_art_key}
          <img
            src="{getApiBase()}/covers/audiobook/{$currentAudiobook.id}"
            alt="cover"
            class="full-cover"
          />
        {:else}
          <div class="full-cover full-cover-ph">
            <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
              <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/>
              <path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/>
            </svg>
          </div>
        {/if}
      </div>

      <!-- Title + author -->
      <div class="full-meta">
        <h2 class="full-title">{$currentAudiobook.title}</h2>
        {#if $currentAudiobook.author_name}
          <p class="full-author">{$currentAudiobook.author_name}</p>
        {/if}
        {#if $abCurrentChapter}
          <p class="full-chapter">{$abCurrentChapter.title}</p>
        {/if}
      </div>

      <!-- Seek bar + chapter marks -->
      <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
      <div class="full-seek-area" on:click|stopPropagation>
        <div class="full-seek-wrap">
          {#if $currentAudiobook.chapters?.length}
            {#each $currentAudiobook.chapters.slice(1) as ch (ch.id)}
              <div class="ch-tick" style="left: {chapterPct(ch)}%" title={ch.title}></div>
            {/each}
          {/if}
          <div class="seek-track">
            <div class="seek-fill" style="width: {$abProgress}%"></div>
          </div>
          <input
            type="range" min="0" max="100" step="0.05"
            value={$abProgress}
            on:input={onSeek}
            class="seek-input"
            aria-label="Seek"
          />
        </div>
        <div class="seek-times">
          <span>{$abFormattedPosition}</span>
          <span>{$abFormattedDuration}</span>
        </div>
      </div>

      <!-- Transport -->
      <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
      <div class="full-transport" on:click|stopPropagation>
        <button class="skip-btn" on:click={() => skipBackward(10)} aria-label="Back 10 s">
          <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="1 4 1 10 7 10"/>
            <path d="M3.51 15a9 9 0 1 0 .49-3.5"/>
          </svg>
          <span class="skip-label">10</span>
        </button>

        <button class="play-btn" on:click={toggleABPlayPause} aria-label={$abPlaybackState === 'playing' ? 'Pause' : 'Play'}>
          {#if $abPlaybackState === 'loading'}
            <div class="spin-ring"></div>
          {:else if $abPlaybackState === 'playing'}
            <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor">
              <rect x="6" y="4" width="4" height="16" rx="1.5"/>
              <rect x="14" y="4" width="4" height="16" rx="1.5"/>
            </svg>
          {:else}
            <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor">
              <polygon points="5,3 19,12 5,21"/>
            </svg>
          {/if}
        </button>

        <button class="skip-btn" on:click={() => skipForward(30)} aria-label="Forward 30 s">
          <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="23 4 23 10 17 10"/>
            <path d="M20.49 15a9 9 0 1 1-.49-3.5"/>
          </svg>
          <span class="skip-label">30</span>
        </button>
      </div>

      <!-- Utility row: speed, sleep, chapters, bookmarks -->
      <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
      <div class="util-row" on:click|stopPropagation>
        <!-- Speed -->
        <button class="util-btn" class:util-active={$abSpeed !== 1.0}
          on:click={() => { showSpeed = !showSpeed; showSleep = false; showChapters = false; showBookmarks = false; }}
          aria-label="Playback speed">
          <span class="util-speed-label">{$abSpeed}×</span>
        </button>

        <!-- Sleep -->
        <button class="util-btn" class:util-active={$sleepTimerMins > 0}
          on:click={() => { showSleep = !showSleep; showSpeed = false; showChapters = false; showBookmarks = false; }}
          aria-label="Sleep timer">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
          </svg>
          {#if $sleepTimerMins > 0}
            <span class="util-badge">{$sleepTimerMins}m</span>
          {/if}
        </button>

        <!-- Chapters -->
        {#if $currentAudiobook.chapters?.length}
          <button class="util-btn" class:util-active={showChapters}
            on:click={() => { showChapters = !showChapters; showSpeed = false; showSleep = false; showBookmarks = false; }}
            aria-label="Chapters">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/>
              <line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/>
              <line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/>
            </svg>
          </button>
        {/if}

        <!-- Bookmark -->
        <button class="util-btn" class:util-active={$abBookmarks.length > 0}
          on:click={() => { showBookmarks = !showBookmarks; showSpeed = false; showSleep = false; showChapters = false; }}
          aria-label="Bookmarks">
          <svg width="18" height="18" viewBox="0 0 24 24" fill={$abBookmarks.length > 0 ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z"/>
          </svg>
        </button>
      </div>

      <!-- Bottom sheets (slide up) -->
      {#if showSpeed}
        <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
        <div class="sheet-overlay" on:click={closeSheets}></div>
        <div class="bottom-sheet" on:click|stopPropagation>
          <div class="sheet-handle"></div>
          <p class="sheet-title">Playback Speed</p>
          <div class="speed-grid">
            {#each AB_SPEEDS as s}
              <button class="speed-chip" class:chip-active={$abSpeed === s}
                on:click={() => { setABSpeed(s); showSpeed = false; }}>{s}×</button>
            {/each}
          </div>
        </div>
      {/if}

      {#if showSleep}
        <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
        <div class="sheet-overlay" on:click={closeSheets}></div>
        <div class="bottom-sheet" on:click|stopPropagation>
          <div class="sheet-handle"></div>
          <p class="sheet-title">Sleep Timer</p>
          <div class="speed-grid">
            <button class="speed-chip" class:chip-active={$sleepTimerMins === 0}
              on:click={() => { setSleepTimer(0); showSleep = false; }}>Off</button>
            {#each SLEEP_PRESETS as mins}
              <button class="speed-chip" class:chip-active={$sleepTimerMins === mins}
                on:click={() => { setSleepTimer(mins); showSleep = false; }}>{mins}m</button>
            {/each}
          </div>
        </div>
      {/if}

      {#if showChapters && $currentAudiobook.chapters?.length}
        <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
        <div class="sheet-overlay" on:click={closeSheets}></div>
        <div class="bottom-sheet chapter-sheet" on:click|stopPropagation>
          <div class="sheet-handle"></div>
          <p class="sheet-title">Chapters</p>
          <div class="chapter-scroll">
            {#each $currentAudiobook.chapters as ch (ch.id)}
              {@const active = $abCurrentChapter?.id === ch.id}
              <button class="chapter-row" class:ch-active={active}
                on:click={() => { jumpToChapter(ch); showChapters = false; }}>
                <span class="ch-num">{ch.chapter_num + 1}</span>
                <span class="ch-name">{ch.title}</span>
                <span class="ch-time">{fmtMs(ch.start_ms)}</span>
                {#if active}<span class="ch-dot"></span>{/if}
              </button>
            {/each}
          </div>
        </div>
      {/if}

      {#if showBookmarks}
        <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
        <div class="sheet-overlay" on:click={closeSheets}></div>
        <div class="bottom-sheet" on:click|stopPropagation>
          <div class="sheet-handle"></div>
          <p class="sheet-title">Bookmarks</p>
          <button class="bm-add-btn" on:click={() => { createBookmark(); showBookmarks = false; }}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
              <line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>
            </svg>
            Bookmark current position
          </button>
          {#if $abBookmarks.length === 0}
            <p class="bm-empty">No bookmarks yet</p>
          {:else}
            <div class="bm-list">
              {#each $abBookmarks as bm (bm.id)}
                <div class="bm-row">
                  <button class="bm-jump" on:click={() => { seekAudiobook(bm.position_ms / 1000); showBookmarks = false; }}>
                    <span class="bm-t">{fmtMs(bm.position_ms)}</span>
                    {#if bm.note}<span class="bm-n">{bm.note}</span>{/if}
                  </button>
                  <button class="bm-del" on:click={() => deleteBookmark(bm.id)} aria-label="Delete">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
                      <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
                    </svg>
                  </button>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/if}

    </div><!-- end ab-full -->

  {:else}
    <!-- ── Mini bar ────────────────────────────────────────────────────── -->
    <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
    <div class="ab-mini" on:click={() => expanded = true}>
      <div class="mini-progress" style="width: {$abProgress}%"></div>

      {#if $currentAudiobook.cover_art_key}
        <img src="{getApiBase()}/covers/audiobook/{$currentAudiobook.id}" alt="" class="mini-cover" />
      {:else}
        <div class="mini-cover mini-cover-ph">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/>
            <path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/>
          </svg>
        </div>
      {/if}

      <div class="mini-info">
        <span class="mini-title">{$currentAudiobook.title}</span>
        {#if $abCurrentChapter}
          <span class="mini-chapter">{$abCurrentChapter.title}</span>
        {:else if $currentAudiobook.author_name}
          <span class="mini-chapter">{$currentAudiobook.author_name}</span>
        {/if}
      </div>

      <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
      <div class="mini-controls" on:click|stopPropagation>
        <button class="mini-btn" on:click={() => skipBackward(10)} aria-label="Back 10 s">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-3.5"/>
          </svg>
        </button>
        <button class="mini-btn mini-play" on:click={toggleABPlayPause} aria-label={$abPlaybackState === 'playing' ? 'Pause' : 'Play'}>
          {#if $abPlaybackState === 'playing'}
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
              <rect x="6" y="4" width="4" height="16" rx="1"/><rect x="14" y="4" width="4" height="16" rx="1"/>
            </svg>
          {:else}
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor"><polygon points="5,3 19,12 5,21"/></svg>
          {/if}
        </button>
        <button class="mini-btn" on:click={() => skipForward(30)} aria-label="Forward 30 s">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-.49-3.5"/>
          </svg>
        </button>
      </div>
    </div>
  {/if}
{/if}

<style>
  /* ── Mini bar ─────────────────────────────────────────── */
  .ab-mini {
    position: fixed;
    bottom: calc(64px + env(safe-area-inset-bottom, 0px)); /* above mobile nav */
    left: 0;
    right: 0;
    height: 66px;
    background: var(--bg-elevated);
    border-top: 1px solid var(--border);
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 12px;
    cursor: pointer;
    z-index: 500;
    overflow: hidden;
  }

  /* Thin progress strip at the very top of the mini bar */
  .mini-progress {
    position: absolute;
    top: 0;
    left: 0;
    height: 2px;
    background: var(--accent);
    transition: width 0.4s linear;
    pointer-events: none;
  }

  .mini-cover {
    width: 44px;
    height: 44px;
    border-radius: 6px;
    object-fit: cover;
    flex-shrink: 0;
    box-shadow: 0 2px 8px rgba(0,0,0,0.2);
  }
  .mini-cover-ph {
    background: var(--bg-hover);
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
  }

  .mini-info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .mini-title {
    font-size: 0.88rem;
    font-weight: 500;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .mini-chapter {
    font-size: 0.73rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .mini-controls {
    display: flex;
    align-items: center;
    gap: 2px;
    flex-shrink: 0;
  }
  .mini-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    transition: color 0.15s, background 0.15s;
  }
  .mini-btn:hover { color: var(--text); background: var(--bg-hover); }
  .mini-play { color: var(--text); }

  /* ── Full-screen player ───────────────────────────────── */
  .ab-full {
    position: fixed;
    inset: 0;
    background: var(--bg);
    z-index: 600;
    display: flex;
    flex-direction: column;
    align-items: center;
    overflow: hidden;
    padding-bottom: env(safe-area-inset-bottom, 0px);
  }

  .full-header {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 16px 4px;
    flex-shrink: 0;
  }
  .pill-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 6px;
    border-radius: 50%;
    display: inline-flex;
    align-items: center;
    transition: color 0.15s, background 0.15s;
  }
  .pill-btn:hover { color: var(--text); background: var(--bg-hover); }
  .full-type-label {
    font-size: 0.72rem;
    font-weight: 600;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  .full-cover-wrap {
    flex-shrink: 0;
    padding: 20px 32px 12px;
    width: 100%;
    display: flex;
    justify-content: center;
  }
  .full-cover {
    width: min(280px, 72vw);
    height: min(280px, 72vw);
    border-radius: 12px;
    object-fit: cover;
    box-shadow: 0 16px 48px rgba(0,0,0,0.35);
  }
  .full-cover-ph {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
  }

  .full-meta {
    width: 100%;
    padding: 0 28px 6px;
    text-align: center;
    flex-shrink: 0;
  }
  .full-title {
    font-size: 1.15rem;
    font-weight: 700;
    color: var(--text);
    margin: 0 0 4px;
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .full-author {
    font-size: 0.85rem;
    color: var(--text-muted);
    margin: 0 0 2px;
  }
  .full-chapter {
    font-size: 0.8rem;
    color: var(--accent);
    margin: 0;
    font-style: italic;
  }

  /* Seek area */
  .full-seek-area {
    width: 100%;
    padding: 8px 28px 4px;
    flex-shrink: 0;
  }
  .full-seek-wrap {
    position: relative;
    height: 24px;
    display: flex;
    align-items: center;
    margin-bottom: 4px;
  }
  .seek-track {
    position: absolute;
    left: 0; right: 0;
    height: 4px;
    background: var(--bg-elevated);
    border-radius: 2px;
    overflow: hidden;
  }
  .seek-fill {
    height: 100%;
    background: var(--accent);
  }
  .seek-input {
    position: absolute;
    left: 0; right: 0;
    width: 100%;
    margin: 0;
    height: 24px;
    cursor: pointer;
    -webkit-appearance: none;
    appearance: none;
    background: transparent;
    z-index: 2;
  }
  .seek-input::-webkit-slider-runnable-track { background: transparent; height: 4px; }
  .seek-input::-moz-range-track { background: transparent; height: 4px; border: none; }
  .seek-input::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 18px; height: 18px; border-radius: 50%;
    background: var(--accent); margin-top: -7px; cursor: pointer;
  }
  .seek-input::-moz-range-thumb {
    width: 18px; height: 18px; border-radius: 50%;
    background: var(--accent); border: none; cursor: pointer;
  }
  .seek-times {
    display: flex;
    justify-content: space-between;
    font-size: 0.72rem;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }
  .ch-tick {
    position: absolute;
    top: 50%;
    transform: translateX(-50%) translateY(-50%);
    width: 2px;
    height: 12px;
    background: var(--text-muted);
    opacity: 0.35;
    border-radius: 1px;
    pointer-events: none;
    z-index: 1;
  }

  /* Transport */
  .full-transport {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 28px;
    padding: 8px 28px 10px;
    width: 100%;
    flex-shrink: 0;
  }
  .skip-btn {
    position: relative;
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 8px;
    border-radius: 50%;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: color 0.15s, background 0.15s;
  }
  .skip-btn:hover { color: var(--text); background: var(--bg-elevated); }
  .skip-label {
    position: absolute;
    font-size: 8px;
    font-weight: 700;
    color: currentColor;
    line-height: 1;
    bottom: 6px;
  }
  .play-btn {
    width: 68px;
    height: 68px;
    border-radius: 50%;
    background: var(--accent);
    border: none;
    color: #fff;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    box-shadow: 0 4px 20px color-mix(in srgb, var(--accent) 40%, transparent);
    transition: transform 0.1s, box-shadow 0.15s;
  }
  .play-btn:active { transform: scale(0.94); }

  /* Utility row */
  .util-row {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 16px;
    padding: 4px 24px 12px;
    flex-shrink: 0;
  }
  .util-btn {
    position: relative;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    color: var(--text-muted);
    cursor: pointer;
    padding: 10px 14px;
    border-radius: 10px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
    min-width: 52px;
  }
  .util-btn:hover { background: var(--bg-hover); color: var(--text); }
  .util-active { color: var(--accent) !important; border-color: var(--accent) !important; }
  .util-speed-label { font-size: 0.88rem; font-weight: 600; }
  .util-badge {
    position: absolute;
    top: 3px;
    right: 5px;
    font-size: 9px;
    font-weight: 700;
    color: var(--accent);
    line-height: 1;
  }

  /* ── Bottom sheets ──────────────────────────────────────── */
  .sheet-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.45);
    z-index: 700;
  }
  .bottom-sheet {
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
    background: var(--bg-elevated);
    border-radius: 16px 16px 0 0;
    padding: 8px 20px calc(20px + env(safe-area-inset-bottom, 0px));
    z-index: 800;
    border-top: 1px solid var(--border);
  }
  .sheet-handle {
    width: 36px;
    height: 4px;
    background: var(--border);
    border-radius: 2px;
    margin: 0 auto 12px;
  }
  .sheet-title {
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
    margin: 0 0 14px;
    text-align: center;
  }

  /* Speed / sleep chips */
  .speed-grid {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    justify-content: center;
    padding-bottom: 4px;
  }
  .speed-chip {
    padding: 10px 20px;
    background: var(--bg-hover);
    border: 1px solid var(--border);
    border-radius: 24px;
    font-size: 0.88rem;
    font-weight: 500;
    color: var(--text-muted);
    cursor: pointer;
    transition: background 0.12s, color 0.12s, border-color 0.12s;
  }
  .speed-chip:hover { background: var(--bg); color: var(--text); }
  .chip-active {
    background: var(--accent) !important;
    color: #fff !important;
    border-color: var(--accent) !important;
    font-weight: 700;
  }

  /* Chapter sheet */
  .chapter-sheet { max-height: 60vh; display: flex; flex-direction: column; }
  .chapter-scroll { overflow-y: auto; flex: 1; }
  .chapter-row {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 12px 4px;
    background: none;
    border: none;
    border-bottom: 1px solid var(--border);
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
  }
  .chapter-row:last-child { border-bottom: none; }
  .chapter-row:hover { background: var(--bg-hover); }
  .ch-active { background: color-mix(in srgb, var(--accent) 8%, transparent) !important; }
  .ch-num {
    font-size: 0.72rem;
    color: var(--text-muted);
    width: 22px;
    text-align: right;
    flex-shrink: 0;
  }
  .ch-name {
    flex: 1;
    font-size: 0.88rem;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .ch-time {
    font-size: 0.72rem;
    color: var(--text-muted);
    flex-shrink: 0;
    font-variant-numeric: tabular-nums;
  }
  .ch-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
    flex-shrink: 0;
  }

  /* Bookmark sheet */
  .bm-add-btn {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 12px 4px;
    background: none;
    border: none;
    border-bottom: 1px solid var(--border);
    cursor: pointer;
    font-size: 0.88rem;
    font-weight: 500;
    color: var(--accent);
    margin-bottom: 4px;
  }
  .bm-empty { font-size: 0.82rem; color: var(--text-muted); text-align: center; padding: 16px 0; margin: 0; }
  .bm-list { max-height: 240px; overflow-y: auto; }
  .bm-row { display: flex; align-items: center; border-bottom: 1px solid var(--border); }
  .bm-row:last-child { border-bottom: none; }
  .bm-jump {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 4px;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
  }
  .bm-t { font-size: 0.85rem; color: var(--accent); font-variant-numeric: tabular-nums; flex-shrink: 0; }
  .bm-n { font-size: 0.82rem; color: var(--text-muted); flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .bm-del {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 10px 12px;
    display: inline-flex;
    align-items: center;
    transition: color 0.15s;
  }
  .bm-del:hover { color: var(--error, #e55); }

  /* Loading spinner */
  .spin-ring {
    width: 26px;
    height: 26px;
    border: 2.5px solid rgba(255,255,255,0.3);
    border-top-color: #fff;
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }

  /* Only show on mobile */
  @media (min-width: 641px) {
    .ab-mini, .ab-full { display: none; }
  }
</style>
