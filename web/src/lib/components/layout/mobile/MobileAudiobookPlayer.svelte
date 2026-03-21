<script lang="ts">
  import { onMount } from 'svelte';
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
    seekAudiobookMs,
    skipForward,
    skipBackward,
    setABSpeed,
    setABVolume,
    setSleepTimer,
    jumpToChapter,
    createBookmark,
    deleteBookmark,
    closeAudiobook,
    AB_SPEEDS,
    SLEEP_PRESETS,
    abFormattedFormat,
    abChapterProgress,
    abPreviousChapter,
    abNextChapter,
  } from '$lib/stores/player/audiobookPlayer';
  import { getApiBase } from '$lib/api/base';
  import type { AudiobookChapter } from '$lib/types';
  import { goto } from '$app/navigation';
  import { activeDevices, deviceId, exclusiveMode } from '$lib/stores/player/deviceSession';
  import { devices as devicesApi } from '$lib/api/devices';

  let playerOpen = false;
  let playerHistoryPushed = false;

  onMount(() => {
    function handlePopState() {
      if (playerOpen) {
        playerHistoryPushed = false;
        playerOpen = false;
        rawDelta = 0;
        swipeDelta = 0;
        swiping = false;
        dismissing = false;
      }
    }
    window.addEventListener('popstate', handlePopState);
    return () => window.removeEventListener('popstate', handlePopState);
  });

  // Touch-swipe to dismiss
  let touchStartY = 0;
  let touchCurrentY = 0;
  let swiping = false;
  let rawDelta = 0;
  let swipeDelta = 0;
  let dismissing = false;

  // Mini-player horizontal swipe → skip backward / forward
  let miniStartX = 0;
  let miniStartY = 0;
  let miniDeltaX = 0;
  let miniSwipeAxis: 'h' | 'v' | null = null;
  let miniIsSwiping = false;
  let miniDidSwipe = false;

  function onMiniTouchStart(e: TouchEvent) {
    miniStartX = e.touches[0].clientX;
    miniStartY = e.touches[0].clientY;
    miniDeltaX = 0;
    miniSwipeAxis = null;
    miniIsSwiping = true;
    miniDidSwipe = false;
  }

  function onMiniTouchMove(e: TouchEvent) {
    if (!miniIsSwiping) return;
    const dx = e.touches[0].clientX - miniStartX;
    const dy = e.touches[0].clientY - miniStartY;

    if (!miniSwipeAxis && (Math.abs(dx) > 8 || Math.abs(dy) > 8)) {
      miniSwipeAxis = Math.abs(dx) >= Math.abs(dy) ? 'h' : 'v';
    }

    if (miniSwipeAxis === 'h') {
      e.preventDefault();
      miniDeltaX = dx;
    }
  }

  function onMiniTouchEnd() {
    if (!miniIsSwiping) return;
    miniIsSwiping = false;

    if (miniSwipeAxis === 'h' && Math.abs(miniDeltaX) > 55) {
      miniDidSwipe = true;
      const goForward = miniDeltaX > 0;
      miniDeltaX = 0;
      miniSwipeAxis = null;
      if (goForward) skipForward(30); else skipBackward(10);
    } else {
      miniDeltaX = 0;
      miniSwipeAxis = null;
    }
  }

  function onMiniClick() {
    if (miniDidSwipe) {
      miniDidSwipe = false;
      return;
    }
    openPlayer();
  }

  function onSeek(e: Event) {
    const pct = parseFloat((e.target as HTMLInputElement).value);
    if ($abCurrentChapter) {
      // Seek bar shows chapter-relative progress — map pct back to absolute ms
      const nextChapter = $currentAudiobook?.chapters?.find(ch => ch.start_ms > $abCurrentChapter!.start_ms);
      const chDurationMs = nextChapter
        ? nextChapter.start_ms - $abCurrentChapter.start_ms
        : ($currentAudiobook?.duration_ms ?? 0) - $abCurrentChapter.start_ms;
      seekAudiobookMs($abCurrentChapter.start_ms + (pct / 100) * chDurationMs);
    } else {
      seekAudiobook(($abDurationMs / 1000) * (pct / 100));
    }
  }

  function openPlayer() {
    playerOpen = true;
    history.pushState({ orbPlayer: true }, '');
    playerHistoryPushed = true;
  }

  function closePlayer(skipHistory = false) {
    playerOpen = false;
    rawDelta = 0;
    swipeDelta = 0;
    swiping = false;
    dismissing = false;
    if (playerHistoryPushed) {
      playerHistoryPushed = false;
      if (!skipHistory) {
        history.back();
      }
    }
  }

  function onTouchStart(e: TouchEvent) {
    if (dismissing) return;
    touchStartY = e.touches[0].clientY;
    touchCurrentY = touchStartY;
    swiping = true;
    rawDelta = 0;
    swipeDelta = 0;
  }

  function onTouchMove(e: TouchEvent) {
    if (!swiping) return;
    touchCurrentY = e.touches[0].clientY;
    rawDelta = Math.max(0, touchCurrentY - touchStartY);
    swipeDelta = rawDelta / (1 + rawDelta / 220);
  }

  async function onTouchEnd() {
    if (!swiping) return;
    swiping = false;
    if (rawDelta > 100) {
      dismissing = true;
      swipeDelta = window.innerHeight * 1.1;
      await new Promise<void>(r => setTimeout(r, 400));
      dismissing = false;
      playerOpen = false;
      rawDelta = 0;
      swipeDelta = 0;
      if (playerHistoryPushed) {
        playerHistoryPushed = false;
        history.back();
      }
    } else {
      rawDelta = 0;
      swipeDelta = 0;
    }
  }

  let showChapters = false;
  let showSpeed    = false;
  let showSleep    = false;
  let showBookmarks = false;
  let devicePickerOpen = false;

  function closeSheets() {
    showChapters = false;
    showSpeed    = false;
    showSleep    = false;
    showBookmarks = false;
    devicePickerOpen = false;
  }

  async function transferToDevice(targetId: string) {
    devicePickerOpen = false;
    const { transferAudiobookPlayback } = await import('$lib/stores/player/audiobookPlayer');
    await transferAudiobookPlayback(targetId);
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

  function goToAudiobook() {
    if ($currentAudiobook) {
      closePlayer(true);
      goto(`/audiobooks/${$currentAudiobook.id}`);
    }
  }
</script>

{#if $currentAudiobook}
  <!-- ── Mini player (shown above bottom nav) ──────────────────────────────── -->
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
  <div
    class="mini-player"
    role="complementary"
    aria-label="Now playing"
    on:click={onMiniClick}
    on:touchstart={onMiniTouchStart}
    on:touchmove|nonpassive={onMiniTouchMove}
    on:touchend={onMiniTouchEnd}
    style="transform: translateX({miniDeltaX * 0.42}px) rotate({miniDeltaX * 0.015}deg);
           transition: {miniIsSwiping ? 'none' : 'transform 0.4s cubic-bezier(0.22, 1, 0.36, 1)'};"
  >
    <!-- Thin progress line at top -->
    <div class="mini-progress-track">
      <div class="mini-progress-fill" style="width: {$abProgress}%"></div>
    </div>

    <!-- Content row -->
    <div class="mini-content">
      <!-- Cover art -->
      {#if $currentAudiobook.id}
        <img
          src="{getApiBase()}/covers/audiobook/{$currentAudiobook.id}"
          alt="cover"
          class="mini-cover"
        />
      {:else}
        <div class="mini-cover mini-cover--placeholder"></div>
      {/if}

      <!-- Track info -->
      <div class="mini-info">
        <span class="mini-title">{$currentAudiobook.title}</span>
        {#if $abCurrentChapter}
          <span class="mini-artist">{$abCurrentChapter.title}</span>
        {:else if $currentAudiobook.author_name}
          <span class="mini-artist">{$currentAudiobook.author_name}</span>
        {/if}
      </div>

      <!-- Controls -->
      <div class="mini-controls">
        <button
          class="mini-btn"
          on:click|stopPropagation={() => skipBackward(10)}
          aria-label="Back 10 seconds"
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 5V1L7 6l5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6H4c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z"/>
          </svg>
        </button>
        <button
          class="mini-btn"
          on:click|stopPropagation={toggleABPlayPause}
          aria-label={$abPlaybackState === 'playing' ? 'Pause' : 'Play'}
        >
          {#if $abPlaybackState === 'playing'}
            <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <rect x="6" y="4" width="4" height="16" rx="1"/>
              <rect x="14" y="4" width="4" height="16" rx="1"/>
            </svg>
          {:else}
            <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <polygon points="5,3 19,12 5,21"/>
            </svg>
          {/if}
        </button>
        <button
          class="mini-btn"
          on:click|stopPropagation={() => skipForward(30)}
          aria-label="Forward 30 seconds"
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
            <path d="M18 13c0 3.31-2.69 6-6 6s-6-2.69-6-6 2.69-6 6-6v4l5-5-5-5v4c-4.42 0-8 3.58-8 8s3.58 8 8 8 8-3.58 8-8h-2z"/>
          </svg>
        </button>
      </div>
    </div>
  </div>

  <!-- ── Full-screen player ─────────────────────────────────────────────────── -->
  {#if playerOpen}
    <!-- svelte-ignore a11y-click-events-have-key-events -->
    <!-- svelte-ignore a11y-no-static-element-interactions -->
    <div
      class="fullscreen-player"
      style="
        transform: translateY({swipeDelta}px) scale({1 - swipeDelta * 0.00032});
        opacity: {Math.max(0.12, 1 - swipeDelta / 310)};
        border-radius: {Math.min(swipeDelta * 0.22, 20)}px;
        transition: {swiping ? 'none' : dismissing
          ? 'transform 0.4s cubic-bezier(0.4, 0, 1, 1), opacity 0.4s cubic-bezier(0.4, 0, 1, 1), border-radius 0.4s cubic-bezier(0.4, 0, 1, 1)'
          : 'transform 0.55s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.55s cubic-bezier(0.22, 1, 0.36, 1), border-radius 0.55s cubic-bezier(0.22, 1, 0.36, 1)'};
      "
      on:touchstart={onTouchStart}
      on:touchmove={onTouchMove}
      on:touchend={onTouchEnd}
      on:click={closeSheets}
    >
      <!-- Blurred background -->
      {#if $currentAudiobook.id}
        <div
          class="fs-bg"
          style="
            background-image: url('{getApiBase()}/covers/audiobook/{$currentAudiobook.id}');
            transform: translateY({-swipeDelta * 0.38}px) scale({1 + swipeDelta * 0.00045});
            transition: {swiping ? 'none' : 'transform 0.55s cubic-bezier(0.22, 1, 0.36, 1)'};
          "
        ></div>
      {/if}
      <div class="fs-overlay"></div>

      <!-- Content -->
      <div class="fs-content">
        <!-- Top bar: swipe handle + close button -->
        <div class="fs-topbar">
          <div class="fs-topbar-spacer"></div>
          <div class="swipe-handle"></div>
          <button class="fs-close-btn" on:click={closePlayer} aria-label="Close player">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true">
              <polyline points="6 9 12 15 18 9"/>
            </svg>
          </button>
        </div>

        <!-- Format badge -->
        {#if $abFormattedFormat}
          <div class="fs-format-badge">{$abFormattedFormat}</div>
        {/if}

        <!-- Cover art -->
        <div class="fs-cover-wrap">
          {#if $currentAudiobook.id}
            <img
              src="{getApiBase()}/covers/audiobook/{$currentAudiobook.id}"
              alt="cover"
              class="fs-cover"
            />
          {:else}
            <div class="fs-cover fs-cover--placeholder"></div>
          {/if}
        </div>

        <!-- Active chapter info -->
        <div class="fs-lyric-slot">
          {#if $abCurrentChapter}
            <span class="fs-lyric-preview">
              {$abCurrentChapter.title}
            </span>
          {/if}
        </div>

        <!-- Info -->
        <div class="fs-info">
          <div class="fs-info-text">
            <div class="fs-title">{$currentAudiobook.title}</div>
            <div class="fs-sub">
              {#if $currentAudiobook.author_name}
                <!-- svelte-ignore a11y-click-events-have-key-events -->
                <span class="fs-artist" on:click={goToAudiobook} role="link" tabindex="0">{$currentAudiobook.author_name}</span>
              {/if}
            </div>
          </div>
          <div class="fs-actions">
            <button class="fs-close-btn" on:click|stopPropagation={closeAudiobook} aria-label="Stop playback" title="Stop playback" style="background:none">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
                <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
              </svg>
            </button>
          </div>
        </div>

        <!-- Seek bar (chapter-aware) -->
        <div class="fs-seek">
          {#if $abCurrentChapter}
            <!-- Chapter info header -->
            <div class="chapter-nav-info">
              {#if $abPreviousChapter}
                <!-- svelte-ignore a11y-click-events-have-key-events -->
                <button class="chapter-nav prev-nav" on:click|stopPropagation={() => jumpToChapter($abPreviousChapter!)} title={$abPreviousChapter.title}>
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
                <!-- svelte-ignore a11y-click-events-have-key-events -->
                <button class="chapter-nav next-nav" on:click|stopPropagation={() => jumpToChapter($abNextChapter!)} title={$abNextChapter.title}>
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

        <!-- Main controls -->
        <div class="fs-controls">
          <button
            class="fs-btn fs-btn--icon"
            class:active={$abSpeed !== 1.0}
            on:click|stopPropagation={() => { showSpeed = !showSpeed; showSleep = false; showChapters = false; showBookmarks = false; }}
            aria-label="Playback speed"
          >
            <span class="util-speed-label">{$abSpeed}×</span>
          </button>

          <button class="fs-btn fs-btn--prev" on:click|stopPropagation={() => skipBackward(10)} aria-label="Back 10 seconds">
            <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 5V1L7 6l5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6H4c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z"/>
            </svg>
            <span class="skip-label-fs">10</span>
          </button>

          <button
            class="fs-btn fs-btn--play"
            on:click={toggleABPlayPause}
            aria-label={$abPlaybackState === 'playing' ? 'Pause' : 'Play'}
          >
            {#if $abPlaybackState === 'loading'}
              <div class="spin-ring-fs"></div>
            {:else if $abPlaybackState === 'playing'}
              <svg width="32" height="32" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <rect x="6" y="4" width="4" height="16" rx="1.5"/>
                <rect x="14" y="4" width="4" height="16" rx="1.5"/>
              </svg>
            {:else}
              <svg width="32" height="32" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <polygon points="5,3 19,12 5,21"/>
              </svg>
            {/if}
          </button>

          <button class="fs-btn fs-btn--next" on:click|stopPropagation={() => skipForward(30)} aria-label="Forward 30 seconds">
            <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor">
              <path d="M18 13c0 3.31-2.69 6-6 6s-6-2.69-6-6 2.69-6 6-6v4l5-5-5-5v4c-4.42 0-8 3.58-8 8s3.58 8 8 8 8-3.58 8-8h-2z"/>
            </svg>
            <span class="skip-label-fs">30</span>
          </button>

          <button
            class="fs-btn fs-btn--icon"
            class:active={$sleepTimerMins > 0}
            on:click|stopPropagation={() => { showSleep = !showSleep; showSpeed = false; showChapters = false; showBookmarks = false; }}
            aria-label="Sleep timer"
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
            </svg>
            {#if $sleepTimerMins > 0}
              <span class="sleep-badge-fs">{$sleepTimerMins}</span>
            {/if}
          </button>
        </div>

        <!-- Volume row -->
        <div class="fs-volume">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
            <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>
          </svg>
          <div class="volume-slider-wrap">
            <div class="volume-slider-fill" style="width: {$abVolume * 100}%"></div>
            <input
              type="range"
              min="0"
              max="1"
              step="0.01"
              value={$abVolume}
              on:input={(e) => setABVolume(parseFloat((e.target as HTMLInputElement).value))}
              on:touchstart|stopPropagation
              on:touchmove|stopPropagation
              on:touchend|stopPropagation
              class="volume-input"
              aria-label="Volume"
            />
          </div>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
            <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>
            <path d="M15.54 8.46a5 5 0 0 1 0 7.07"/>
            <path d="M19.07 4.93a10 10 0 0 1 0 14.14"/>
          </svg>
        </div>

        <!-- Extras row -->
        <div class="fs-extras">
          <button
            class="fs-extra-btn"
            class:active={showChapters}
            on:click|stopPropagation={() => { showChapters = !showChapters; showSpeed = false; showSleep = false; showBookmarks = false; }}
            aria-label="Chapters"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/>
              <line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/>
              <line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/>
            </svg>
            <span>Chapters</span>
          </button>

          <button
            class="fs-extra-btn"
            class:active={showBookmarks}
            on:click|stopPropagation={() => { showBookmarks = !showBookmarks; showSpeed = false; showSleep = false; showChapters = false; devicePickerOpen = false; }}
            aria-label="Bookmarks"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill={$abBookmarks.length > 0 ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z"/>
            </svg>
            <span>Bookmarks</span>
          </button>

          {#if $exclusiveMode && $activeDevices.length > 0}
            <div class="fs-device-wrap">
              <button
                class="fs-extra-btn"
                class:active={devicePickerOpen}
                on:click|stopPropagation={() => { devicePickerOpen = !devicePickerOpen; showSpeed = false; showSleep = false; showChapters = false; showBookmarks = false; }}
                aria-label="Switch playback device"
                title="Switch device"
              >
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <rect x="2" y="3" width="20" height="14" rx="2"/>
                  <path d="M8 21h8"/>
                  <path d="M12 17v4"/>
                </svg>
                <span>Devices{#if $activeDevices.length > 1}&nbsp;<span class="queue-count">{$activeDevices.length}</span>{/if}</span>
              </button>

              {#if devicePickerOpen}
                <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
                <div
                  class="sheet-overlay"
                  on:click|stopPropagation={() => (devicePickerOpen = false)}
                  on:touchstart|stopPropagation={() => {}}
                  on:touchmove|stopPropagation={() => {}}
                ></div>
                <!-- svelte-ignore a11y-no-static-element-interactions -->
                <div
                  class="bottom-sheet"
                  on:click|stopPropagation
                  on:touchstart|stopPropagation={() => {}}
                  on:touchmove|stopPropagation={() => {}}
                >
                  <div class="sheet-handle"></div>
                  <p class="sheet-title">Sessions</p>
                  {#each $activeDevices as device (device.id)}
                    <button
                      class="fs-device-item"
                      class:is-active={device.is_active}
                      class:is-this={device.id === deviceId}
                      on:click={() => transferToDevice(device.id)}
                    >
                      <div class="fs-device-left">
                        <span class="fs-device-dot" class:fs-device-dot--active={device.is_active}></span>
                        <div class="fs-device-info">
                          <span class="fs-device-name">
                            {device.name}
                            {#if device.id === deviceId}<span class="fs-this-badge">this device</span>{/if}
                          </span>
                          <span class="fs-device-track">{device.state.audiobook_title || device.state.track_title || 'Idle'}</span>
                        </div>
                      </div>
                      {#if device.id !== deviceId}
                        <span class="fs-transfer-hint">Transfer</span>
                      {:else if !device.is_active}
                        <span class="fs-transfer-hint">Play here</span>
                      {/if}
                    </button>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}
        </div>
      </div>

      <!-- Bottom sheets -->
      {#if showSpeed}
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div class="sheet-overlay" on:click={closeSheets} on:touchstart|stopPropagation on:touchmove|stopPropagation on:touchend|stopPropagation></div>
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div class="bottom-sheet" on:click|stopPropagation on:touchstart|stopPropagation on:touchmove|stopPropagation on:touchend|stopPropagation>
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
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div class="sheet-overlay" on:click={closeSheets} on:touchstart|stopPropagation on:touchmove|stopPropagation on:touchend|stopPropagation></div>
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div class="bottom-sheet" on:click|stopPropagation on:touchstart|stopPropagation on:touchmove|stopPropagation on:touchend|stopPropagation>
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
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div class="sheet-overlay" on:click={closeSheets} on:touchstart|stopPropagation on:touchmove|stopPropagation on:touchend|stopPropagation></div>
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div class="bottom-sheet chapter-sheet" on:click|stopPropagation on:touchstart|stopPropagation on:touchmove|stopPropagation on:touchend|stopPropagation>
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
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div class="sheet-overlay" on:click={closeSheets} on:touchstart|stopPropagation on:touchmove|stopPropagation on:touchend|stopPropagation></div>
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div class="bottom-sheet" on:click|stopPropagation on:touchstart|stopPropagation on:touchmove|stopPropagation on:touchend|stopPropagation>
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
    </div>
  {/if}
{/if}

<style>
  /* ── Mini player (shared with MobilePlayer) ─────────────────────────── */
  .mini-player {
    display: none;
  }

  @media (max-width: 640px) {
    .mini-player {
      display: block;
      position: fixed;
      left: 12px;
      right: 12px;
      bottom: calc(68px + env(safe-area-inset-bottom));
      background: var(--bg-elevated);
      border: 1px solid var(--border);
      border-radius: 16px;
      box-shadow: 0 8px 32px rgba(0, 0, 0, 0.28), 0 2px 8px rgba(0, 0, 0, 0.18);
      overflow: hidden;
      z-index: 39;
      cursor: pointer;
      -webkit-tap-highlight-color: transparent;
      user-select: none;
    }

    .mini-progress-track {
      height: 2px;
      background: var(--bg-hover);
      position: relative;
      margin: 0 12px;
      border-radius: 1px;
      overflow: hidden;
    }

    .mini-progress-fill {
      position: absolute;
      left: 0;
      top: 0;
      height: 100%;
      background: var(--accent);
      pointer-events: none;
    }

    .mini-content {
      display: flex;
      align-items: center;
      gap: 12px;
      padding: 10px 12px;
    }

    .mini-cover {
      width: 42px;
      height: 42px;
      border-radius: 8px;
      object-fit: cover;
      flex-shrink: 0;
      background: var(--bg-hover);
    }

    .mini-cover--placeholder {
      background: var(--bg-hover);
    }

    .mini-info {
      flex: 1;
      min-width: 0;
      display: flex;
      flex-direction: column;
      gap: 2px;
    }

    .mini-title {
      font-size: 0.875rem;
      font-weight: 500;
      color: var(--text);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .mini-artist {
      font-size: 0.75rem;
      color: var(--text-muted);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .mini-controls {
      display: flex;
      align-items: center;
      gap: 4px;
      flex-shrink: 0;
    }

    .mini-btn {
      background: none;
      border: none;
      color: var(--text);
      cursor: pointer;
      padding: 8px;
      display: flex;
      align-items: center;
      justify-content: center;
      border-radius: 50%;
      -webkit-tap-highlight-color: transparent;
      transition: background 0.1s;
    }

    .mini-btn:active {
      background: var(--bg-hover);
    }

    /* ── Full screen player (shared with MobilePlayer) ───────────────────── */
    .fullscreen-player {
      position: fixed;
      inset: 0;
      z-index: 100;
      display: flex;
      flex-direction: column;
      overflow: hidden;
      background: var(--bg);
    }

    .fs-bg {
      position: absolute;
      inset: -20px;
      background-size: cover;
      background-position: center;
      filter: blur(50px) brightness(0.35) saturate(1.8);
      z-index: 0;
    }

    .fs-overlay {
      position: absolute;
      inset: 0;
      background: rgba(0, 0, 0, 0.45);
      z-index: 1;
    }

    .fs-content {
      position: relative;
      z-index: 2;
      display: flex;
      flex-direction: column;
      height: 100%;
      padding: 0 28px;
      padding-top: env(safe-area-inset-top, 16px);
      padding-bottom: calc(env(safe-area-inset-bottom, 16px) + 16px);
      box-sizing: border-box;
    }

    .fs-topbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 8px 0 16px;
      position: relative;
    }

    .fs-topbar-spacer {
      width: 40px;
      flex-shrink: 0;
    }

    .swipe-handle {
      width: 36px;
      height: 4px;
      background: rgba(255, 255, 255, 0.35);
      border-radius: 2px;
      position: absolute;
      left: 50%;
      transform: translateX(-50%);
    }

    .fs-close-btn {
      flex-shrink: 0;
      background: rgba(255, 255, 255, 0.1);
      border: none;
      color: rgba(255, 255, 255, 0.8);
      cursor: pointer;
      padding: 8px;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      -webkit-tap-highlight-color: transparent;
    }

    .fs-format-badge {
      align-self: center;
      font-size: 0.65rem;
      font-weight: 600;
      letter-spacing: 0.06em;
      color: rgba(255, 255, 255, 0.6);
      background: rgba(255, 255, 255, 0.1);
      border: 1px solid rgba(255, 255, 255, 0.15);
      border-radius: 12px;
      padding: 3px 10px;
      flex-shrink: 0;
    }

    .fs-cover-wrap {
      flex: 1;
      display: flex;
      align-items: center;
      justify-content: center;
      min-height: 0;
      padding: 8px 0;
    }

    .fs-cover {
      width: 100%;
      max-width: min(320px, calc(100vw - 56px));
      aspect-ratio: 1;
      border-radius: 12px;
      object-fit: cover;
      background: rgba(255, 255, 255, 0.05);
      box-shadow: 0 24px 60px rgba(0, 0, 0, 0.6);
    }

    .fs-cover--placeholder {
      background: rgba(255, 255, 255, 0.08);
    }

    .fs-lyric-slot {
      flex-shrink: 0;
      height: 3.2em;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 0 20px;
    }

    .fs-lyric-preview {
      display: -webkit-box;
      -webkit-line-clamp: 2;
      -webkit-box-orient: vertical;
      overflow: hidden;
      text-align: center;
      font-size: 0.9rem;
      font-weight: 500;
      color: var(--accent);
      line-height: 1.4;
      font-style: italic;
    }

    .fs-info {
      padding: 20px 0 12px;
      flex-shrink: 0;
      display: flex;
      align-items: center;
      gap: 12px;
    }

    .fs-info-text {
      flex: 1;
      min-width: 0;
    }

    .fs-title {
      font-size: 1.35rem;
      font-weight: 700;
      color: #fff;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      margin-bottom: 4px;
    }

    .fs-sub {
      display: flex;
      align-items: center;
      gap: 6px;
      font-size: 0.875rem;
      color: rgba(255, 255, 255, 0.65);
      white-space: nowrap;
      overflow: hidden;
    }

    .fs-artist {
      overflow: hidden;
      text-overflow: ellipsis;
      cursor: pointer;
    }

    .fs-actions {
      flex-shrink: 0;
    }

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
      /* button reset */
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

    .volume-input {
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

    .volume-input::-webkit-slider-runnable-track {
      background: transparent;
      height: 4px;
    }
    .volume-input::-moz-range-track {
      background: transparent;
      height: 4px;
      border: none;
    }
    .volume-input::-webkit-slider-thumb {
      -webkit-appearance: none;
      width: 16px; height: 16px; border-radius: 50%; background: #fff; margin-top: -6px;
    }
    .volume-input::-moz-range-thumb {
      width: 16px; height: 16px; border-radius: 50%; background: #fff; border: none;
    }

    .seek-times {
      display: flex;
      justify-content: space-between;
      font-size: 0.72rem;
      color: rgba(255, 255, 255, 0.55);
      font-variant-numeric: tabular-nums;
    }

    .fs-controls {
      flex-shrink: 0;
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 8px 0 16px;
    }

    .fs-btn {
      background: none;
      border: none;
      color: rgba(255, 255, 255, 0.8);
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 8px;
      border-radius: 50%;
      position: relative;
      -webkit-tap-highlight-color: transparent;
    }

    .fs-btn:active { background: rgba(255, 255, 255, 0.1); }

    .fs-btn--icon { color: rgba(255, 255, 255, 0.5); }
    .fs-btn--icon.active { color: var(--accent); }

    .util-speed-label { font-size: 0.9rem; font-weight: 700; }

    .fs-btn--prev, .fs-btn--next { color: #fff; flex-direction: column; gap: 2px; }
    .skip-label-fs { font-size: 9px; font-weight: 800; }

    .fs-btn--play {
      width: 68px; height: 68px; background: #fff; color: #000; border-radius: 50%;
      box-shadow: 0 4px 20px rgba(0, 0, 0, 0.4);
    }
    .fs-btn--play:active { background: rgba(255, 255, 255, 0.85); }

    .sleep-badge-fs {
      position: absolute; bottom: 3px; right: 2px; font-size: 9px; font-weight: 800; color: var(--accent);
    }

    .spin-ring-fs {
      width: 28px; height: 28px; border: 3px solid rgba(0,0,0,0.1); border-top-color: #000; border-radius: 50%;
      animation: spin 0.8s linear infinite;
    }
    @keyframes spin { to { transform: rotate(360deg); } }

    .fs-volume {
      flex-shrink: 0;
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 0 0 16px;
      color: rgba(255, 255, 255, 0.45);
    }

    .volume-slider-wrap {
      flex: 1; height: 28px; display: flex; align-items: center; position: relative;
    }
    .volume-slider-fill {
      position: absolute; left: 0; top: 50%; transform: translateY(-50%); height: 4px; background: rgba(255, 255, 255, 0.85); border-radius: 2px; pointer-events: none; z-index: 1;
    }

    .volume-slider-wrap::before {
      content: '';
      position: absolute; left: 0; right: 0; top: 50%; transform: translateY(-50%);
      height: 4px; background: rgba(255, 255, 255, 0.25); border-radius: 2px;
      pointer-events: none;
    }

    .fs-extras {
      flex-shrink: 0;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 48px;
      padding-bottom: 4px;
    }

    .fs-extra-btn {
      background: none; border: none; color: rgba(255, 255, 255, 0.45); cursor: pointer;
      display: flex; flex-direction: column; align-items: center; gap: 4px; font-size: 11px;
      padding: 8px; border-radius: 8px; -webkit-tap-highlight-color: transparent;
    }
    .fs-extra-btn.active { color: var(--accent); }

    /* ── Device picker (uses shared .bottom-sheet styling) ──────────── */
    .fs-device-wrap { position: relative; }
    .fs-device-item {
      width: 100%; background: none; border: none; color: var(--text);
      display: flex; align-items: center; justify-content: space-between;
      padding: 10px 8px; border-radius: 8px; cursor: pointer;
      -webkit-tap-highlight-color: transparent;
    }
    .fs-device-item:active,
    .fs-device-item:hover {
      background: var(--bg-hover);
    }
    .fs-device-item.is-active {
      background: rgba(var(--accent-rgb, 100,100,255), 0.08);
    }
    .fs-device-left {
      display: flex; align-items: center; gap: 10px;
      overflow: hidden; min-width: 0;
    }
    .fs-device-dot {
      width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0;
      background: var(--text-muted); opacity: 0.3;
    }
    .fs-device-dot--active {
      background: var(--accent); opacity: 1;
    }
    .fs-device-info {
      display: flex; flex-direction: column; gap: 2px;
      overflow: hidden; min-width: 0;
    }
    .fs-device-name {
      font-size: 0.85rem; font-weight: 500;
      white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
    }
    .fs-device-track {
      font-size: 0.72rem; color: var(--text-muted);
      white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
    }
    .fs-this-badge {
      font-size: 0.65rem; color: var(--accent); font-weight: 600;
      margin-left: 6px; text-transform: uppercase; letter-spacing: 0.04em;
    }
    .fs-transfer-hint {
      font-size: 0.72rem; color: var(--accent); font-weight: 600;
      white-space: nowrap; flex-shrink: 0; padding-left: 8px;
    }
    .queue-count {
      font-size: 0.65rem; background: var(--accent); color: var(--bg);
      border-radius: 8px; padding: 0 5px; font-weight: 700; vertical-align: middle;
    }

    /* ── Sheets ────────────────────────────────────────────────────────── */
    .sheet-overlay { position: fixed; inset: 0; background: rgba(0,0,0,0.45); z-index: 700; }
    .bottom-sheet {
      position: fixed; bottom: 0; left: 0; right: 0; background: var(--bg-elevated);
      border-radius: 20px 20px 0 0; padding: 12px 24px calc(24px + env(safe-area-inset-bottom, 0px));
      z-index: 800; border-top: 1px solid var(--border);
      box-shadow: 0 -8px 32px rgba(0,0,0,0.4);
    }
    .sheet-handle { width: 36px; height: 4px; background: var(--border); border-radius: 2px; margin: 0 auto 16px; }
    .sheet-title { font-size: 0.75rem; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-muted); margin: 0 0 18px; text-align: center; }

    .speed-grid { display: flex; flex-wrap: wrap; gap: 10px; justify-content: center; }
    .speed-chip {
      padding: 12px 20px; background: var(--bg-hover); border: 1px solid var(--border); border-radius: 24px;
      font-size: 0.9rem; font-weight: 600; color: var(--text-muted); cursor: pointer;
    }
    .chip-active { background: var(--accent) !important; color: #fff !important; border-color: var(--accent) !important; font-weight: 700; }

    .chapter-sheet { max-height: 70vh; display: flex; flex-direction: column; }
    .chapter-scroll { overflow-y: auto; flex: 1; }
    .chapter-row {
      display: flex; align-items: center; gap: 12px; width: 100%; padding: 14px 8px;
      background: none; border: none; border-bottom: 1px solid var(--border); cursor: pointer; text-align: left;
    }
    .ch-active { background: rgba(var(--accent-rgb), 0.1) !important; }
    .ch-num { font-size: 0.75rem; color: var(--text-muted); width: 24px; text-align: right; }
    .ch-name { flex: 1; font-size: 0.9rem; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .ch-time { font-size: 0.75rem; color: var(--text-muted); font-variant-numeric: tabular-nums; }
    .ch-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--accent); }

    .bm-add-btn {
      display: flex; align-items: center; gap: 10px; width: 100%; padding: 14px 8px;
      background: none; border: none; border-bottom: 1px solid var(--border);
      cursor: pointer; font-size: 0.9rem; font-weight: 600; color: var(--accent);
    }
    .bm-empty { font-size: 0.85rem; color: var(--text-muted); text-align: center; padding: 24px 0; }
    .bm-list { max-height: 300px; overflow-y: auto; }
    .bm-row { display: flex; align-items: center; border-bottom: 1px solid var(--border); }
    .bm-jump { flex: 1; display: flex; align-items: center; gap: 12px; padding: 14px 8px; background: none; border: none; cursor: pointer; text-align: left; }
    .bm-t { font-size: 0.85rem; color: var(--accent); font-weight: 600; }
    .bm-n { font-size: 0.85rem; color: var(--text-muted); flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .bm-del { background: none; border: none; color: var(--text-muted); padding: 12px; }
  }
</style>