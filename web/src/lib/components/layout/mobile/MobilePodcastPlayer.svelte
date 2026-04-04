<script lang="ts">
  import { onMount } from "svelte";
  import {
    currentEpisode,
    currentPodcast,
    podcastPlaybackState,
    podcastPositionMs,
    podcastDurationMs,
    podcastBufferedPct,
    podcastSpeed,
    togglePodcastPlayPause,
    seekPodcastMs,
    skipForwardPodcast,
    skipBackwardPodcast,
    closePodcast,
    formatPodcastMs,
    podcastSleepTimerMins,
    setPodcastSleepTimer,
    PODCAST_SLEEP_PRESETS,
    setPodcastSpeed,
    PODCAST_SPEEDS,
  } from "$lib/stores/player/podcastPlayer";
  import { engineVolume, setVolume } from "$lib/stores/player/engine";
  import { getApiBase } from "$lib/api/base";
  import { goto } from "$app/navigation";

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
    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  });

  // Touch-swipe to dismiss
  let touchStartY = 0;
  let touchCurrentY = 0;
  let swiping = false;
  let rawDelta = 0;
  let swipeDelta = 0;
  let dismissing = false;

  // Mini-player horizontal swipe -> skip backward / forward
  let miniStartX = 0;
  let miniStartY = 0;
  let miniDeltaX = 0;
  let miniSwipeAxis: "h" | "v" | null = null;
  let miniIsSwiping = false;
  let miniDidSwipe = false;

  let showSleepSheet = false;
  let showSpeedSheet = false;

  $: progress =
    $podcastDurationMs > 0
      ? ($podcastPositionMs / $podcastDurationMs) * 100
      : 0;

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
      miniSwipeAxis = Math.abs(dx) >= Math.abs(dy) ? "h" : "v";
    }

    if (miniSwipeAxis === "h") {
      e.preventDefault();
      miniDeltaX = dx;
    }
  }

  function onMiniTouchEnd() {
    if (!miniIsSwiping) return;
    miniIsSwiping = false;

    if (miniSwipeAxis === "h" && Math.abs(miniDeltaX) > 55) {
      miniDidSwipe = true;
      const goForward = miniDeltaX > 0;
      miniDeltaX = 0;
      miniSwipeAxis = null;
      if (goForward) skipForwardPodcast(30);
      else skipBackwardPodcast(15);
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

  function onMiniKeyDown(e: KeyboardEvent) {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onMiniClick();
    }
  }

  function openPlayer() {
    playerOpen = true;
    history.pushState({ orbPlayer: true }, "");
    playerHistoryPushed = true;
  }

  function closePlayer(skipHistory = false) {
    playerOpen = false;
    showSleepSheet = false;
    showSpeedSheet = false;
    rawDelta = 0;
    swipeDelta = 0;
    swiping = false;
    dismissing = false;
    if (playerHistoryPushed) {
      playerHistoryPushed = false;
      if (!skipHistory) history.back();
    }
  }

  function closeSheets() {
    showSleepSheet = false;
    showSpeedSheet = false;
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
      await new Promise<void>((r) => setTimeout(r, 400));
      dismissing = false;
      closePlayer();
    } else {
      rawDelta = 0;
      swipeDelta = 0;
    }
  }

  function onSeekInput(e: Event) {
    const pct = parseFloat((e.target as HTMLInputElement).value);
    seekPodcastMs(($podcastDurationMs * pct) / 100);
  }

  function goToPodcast() {
    if ($currentPodcast) {
      closePlayer(true);
      goto(`/podcasts/${$currentPodcast.id}`);
    }
  }
</script>

{#if $currentEpisode}
  <section class="mini-player-wrap" role="complementary" aria-label="Now playing">
    <div
      class="mini-player"
      role="button"
      tabindex="0"
      aria-label="Open full player"
      on:click={onMiniClick}
      on:keydown={onMiniKeyDown}
      on:touchstart={onMiniTouchStart}
      on:touchmove|nonpassive={onMiniTouchMove}
      on:touchend={onMiniTouchEnd}
      style="transform: translateX({miniDeltaX * 0.42}px) rotate({miniDeltaX * 0.015}deg); transition: {miniIsSwiping ? 'none' : 'transform 0.4s cubic-bezier(0.22, 1, 0.36, 1)'};"
    >
      <div class="mini-progress-track">
        <div class="mini-progress-fill buffered" style="width: {$podcastBufferedPct}%"></div>
        <div class="mini-progress-fill" style="width: {progress}%"></div>
      </div>

      <div class="mini-content">
        {#if $currentPodcast?.cover_art_key}
          <img
            src="{getApiBase()}/covers/podcast/{$currentEpisode.podcast_id}"
            alt="cover"
            class="mini-cover"
          />
        {:else}
          <div class="mini-cover mini-cover--placeholder"></div>
        {/if}

        <div class="mini-info">
          <span class="mini-title">{$currentEpisode.title}</span>
          {#if $currentPodcast}
            <span class="mini-artist">{$currentPodcast.title}</span>
          {/if}
        </div>

        <div class="mini-controls">
          <button
            class="mini-btn"
            on:click|stopPropagation={() => skipBackwardPodcast(15)}
            aria-label="Back 15 seconds"
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M12 5V1L7 6l5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6H4c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z" />
            </svg>
          </button>
          <button
            class="mini-btn"
            on:click|stopPropagation={togglePodcastPlayPause}
            aria-label={$podcastPlaybackState === "playing" ? "Pause" : "Play"}
          >
            {#if $podcastPlaybackState === "loading"}
              <div class="spin-ring-mini"></div>
            {:else if $podcastPlaybackState === "playing"}
              <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <rect x="6" y="4" width="4" height="16" rx="1" />
                <rect x="14" y="4" width="4" height="16" rx="1" />
              </svg>
            {:else}
              <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <polygon points="5,3 19,12 5,21" />
              </svg>
            {/if}
          </button>
          <button
            class="mini-btn"
            on:click|stopPropagation={() => skipForwardPodcast(30)}
            aria-label="Forward 30 seconds"
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M18 13c0 3.31-2.69 6-6 6s-6-2.69-6-6 2.69-6 6-6v4l5-5-5-5v4c-4.42 0-8 3.58-8 8s3.58 8 8 8 8-3.58 8-8h-2z" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  </section>

  {#if playerOpen}
    <div
      class="fullscreen-player"
      role="dialog"
      aria-label="Podcast player"
      tabindex="-1"
      style="
        transform: translateY({swipeDelta}px) scale({1 - swipeDelta * 0.00032});
        opacity: {Math.max(0.12, 1 - swipeDelta / 310)};
        border-radius: {Math.min(swipeDelta * 0.22, 20)}px;
        transition: {swiping
        ? 'none'
        : dismissing
          ? 'transform 0.4s cubic-bezier(0.4, 0, 1, 1), opacity 0.4s cubic-bezier(0.4, 0, 1, 1), border-radius 0.4s cubic-bezier(0.4, 0, 1, 1)'
          : 'transform 0.55s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.55s cubic-bezier(0.22, 1, 0.36, 1), border-radius 0.55s cubic-bezier(0.22, 1, 0.36, 1)'};"
      on:touchstart={onTouchStart}
      on:touchmove={onTouchMove}
      on:touchend={onTouchEnd}
      on:click={closeSheets}
      on:keydown={(e) => { if (e.key === "Escape") closePlayer(); }}
    >
      {#if $currentPodcast?.cover_art_key}
        <div
          class="fs-bg"
          style="
            background-image: url('{getApiBase()}/covers/podcast/{$currentEpisode.podcast_id}');
            transform: translateY({-swipeDelta * 0.38}px) scale({1 + swipeDelta * 0.00045});
            transition: {swiping ? 'none' : 'transform 0.55s cubic-bezier(0.22, 1, 0.36, 1)'};"
        ></div>
      {/if}
      <div class="fs-overlay"></div>

      <div class="fs-content">
        <!-- Top bar -->
        <div class="fs-topbar">
          <div class="fs-topbar-spacer"></div>
          <div class="swipe-handle"></div>
          <button class="fs-icon-btn" on:click={() => closePlayer()} aria-label="Close player">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </button>
        </div>

        <div class="fs-format-badge">PODCAST</div>

        <!-- Cover -->
        <div class="fs-cover-wrap">
          {#if $currentPodcast?.cover_art_key}
            <img
              src="{getApiBase()}/covers/podcast/{$currentEpisode.podcast_id}"
              alt="cover"
              class="fs-cover"
            />
          {:else}
            <div class="fs-cover fs-cover--placeholder"></div>
          {/if}
        </div>

        <!-- Info row: title + close (stop) button -->
        <div class="fs-info">
          <div class="fs-info-text">
            <div class="fs-title">{$currentEpisode.title}</div>
            {#if $currentPodcast}
              <button class="fs-artist" on:click={goToPodcast}>
                {$currentPodcast.title}
              </button>
            {/if}
          </div>
          <div class="fs-info-actions">
            <button
              class="fs-icon-btn"
              on:click|stopPropagation={closePodcast}
              aria-label="Stop playback"
              title="Stop playback"
            >
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true">
                <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </div>
        </div>

        <!-- Seek bar with buffered layer -->
        <div class="seek-row">
          <span class="time">{formatPodcastMs($podcastPositionMs)}</span>
          <div class="seek-track-wrap">
            <div class="seek-buffered" style="width: {$podcastBufferedPct}%"></div>
            <div class="seek-played" style="width: {progress}%"></div>
            <input
              type="range"
              min="0"
              max="100"
              step="0.05"
              value={progress}
              on:input={onSeekInput}
              on:touchstart|stopPropagation
              on:touchmove|stopPropagation
              on:touchend|stopPropagation
              aria-label="Seek"
            />
          </div>
          <span class="time">{formatPodcastMs($podcastDurationMs)}</span>
        </div>

        <!-- Main controls -->
        <div class="fs-controls">
          <!-- Speed -->
          <button
            class="fs-btn fs-btn--icon"
            class:active={$podcastSpeed !== 1.0}
            on:click|stopPropagation={() => {
              showSpeedSheet = !showSpeedSheet;
              showSleepSheet = false;
            }}
            aria-label="Playback speed"
          >
            <span class="speed-label">{$podcastSpeed}×</span>
          </button>

          <!-- Back 15s -->
          <button
            class="fs-btn fs-btn--prev"
            on:click|stopPropagation={() => skipBackwardPodcast(15)}
            aria-label="Back 15 seconds"
          >
            <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M12 5V1L7 6l5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6H4c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z" />
            </svg>
            <span class="skip-label">15</span>
          </button>

          <!-- Play / Pause -->
          <button
            class="fs-btn fs-btn--play"
            on:click={togglePodcastPlayPause}
            aria-label={$podcastPlaybackState === "playing" ? "Pause" : "Play"}
          >
            {#if $podcastPlaybackState === "loading"}
              <div class="spin-ring-fs"></div>
            {:else if $podcastPlaybackState === "playing"}
              <svg width="32" height="32" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <rect x="6" y="4" width="4" height="16" rx="1.5" />
                <rect x="14" y="4" width="4" height="16" rx="1.5" />
              </svg>
            {:else}
              <svg width="32" height="32" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <polygon points="5,3 19,12 5,21" />
              </svg>
            {/if}
          </button>

          <!-- Forward 30s -->
          <button
            class="fs-btn fs-btn--next"
            on:click|stopPropagation={() => skipForwardPodcast(30)}
            aria-label="Forward 30 seconds"
          >
            <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M18 13c0 3.31-2.69 6-6 6s-6-2.69-6-6 2.69-6 6-6v4l5-5-5-5v4c-4.42 0-8 3.58-8 8s3.58 8 8 8 8-3.58 8-8h-2z" />
            </svg>
            <span class="skip-label">30</span>
          </button>

          <!-- Sleep timer -->
          <button
            class="fs-btn fs-btn--icon"
            class:active={$podcastSleepTimerMins > 0}
            on:click|stopPropagation={() => {
              showSleepSheet = !showSleepSheet;
              showSpeedSheet = false;
            }}
            aria-label="Sleep timer"
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
            </svg>
            {#if $podcastSleepTimerMins > 0}
              <span class="sleep-badge">{$podcastSleepTimerMins}</span>
            {/if}
          </button>
        </div>

        <!-- Volume -->
        <div class="fs-volume">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
            <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
          </svg>
          <div class="volume-slider-wrap">
            <div class="volume-slider-fill" style="width: {$engineVolume * 100}%"></div>
            <input
              type="range"
              min="0"
              max="1"
              step="0.01"
              value={$engineVolume}
              on:input={(e) => setVolume(parseFloat((e.target as HTMLInputElement).value))}
              on:touchstart|stopPropagation
              on:touchmove|stopPropagation
              on:touchend|stopPropagation
              aria-label="Volume"
            />
          </div>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
            <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
            <path d="M15.54 8.46a5 5 0 0 1 0 7.07" />
            <path d="M19.07 4.93a10 10 0 0 1 0 14.14" />
          </svg>
        </div>
      </div>

      <!-- Speed sheet -->
      {#if showSpeedSheet}
        <button
          class="sheet-overlay"
          on:click|stopPropagation={closeSheets}
          on:touchstart|stopPropagation
          on:touchmove|stopPropagation
          on:touchend|stopPropagation
          aria-label="Close speed menu"
        ></button>
        <div
          class="bottom-sheet"
          role="dialog"
          tabindex="-1"
          aria-label="Playback speed"
          on:click|stopPropagation
          on:keydown|stopPropagation
          on:touchstart|stopPropagation
          on:touchmove|stopPropagation
          on:touchend|stopPropagation
        >
          <div class="sheet-handle"></div>
          <p class="sheet-title">Playback Speed</p>
          <div class="sheet-grid">
            {#each PODCAST_SPEEDS as s}
              <button
                class="sheet-item"
                class:active={$podcastSpeed === s}
                on:click={() => {
                  setPodcastSpeed(s);
                  showSpeedSheet = false;
                }}
              >
                {s}×
              </button>
            {/each}
          </div>
        </div>
      {/if}

      <!-- Sleep sheet -->
      {#if showSleepSheet}
        <button
          class="sheet-overlay"
          on:click|stopPropagation={closeSheets}
          on:touchstart|stopPropagation
          on:touchmove|stopPropagation
          on:touchend|stopPropagation
          aria-label="Close sleep timer menu"
        ></button>
        <div
          class="bottom-sheet"
          role="dialog"
          tabindex="-1"
          aria-label="Sleep timer options"
          on:click|stopPropagation
          on:keydown|stopPropagation
          on:touchstart|stopPropagation
          on:touchmove|stopPropagation
          on:touchend|stopPropagation
        >
          <div class="sheet-handle"></div>
          <p class="sheet-title">Sleep timer</p>
          <div class="sheet-grid">
            <button
              class="sheet-item"
              class:active={$podcastSleepTimerMins === 0}
              on:click={() => {
                setPodcastSleepTimer(0);
                showSleepSheet = false;
              }}
            >
              Off
            </button>
            {#each PODCAST_SLEEP_PRESETS as mins}
              <button
                class="sheet-item"
                class:active={$podcastSleepTimerMins === mins}
                on:click={() => {
                  setPodcastSleepTimer(mins);
                  showSleepSheet = false;
                }}
              >
                {mins}m
              </button>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}
{/if}

<style>
  .mini-player-wrap {
    display: contents;
  }
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

    .mini-progress-fill.buffered {
      background: var(--bg-hover);
      filter: brightness(1.6);
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
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
    }

    .spin-ring-mini {
      width: 18px;
      height: 18px;
      border: 2px solid rgba(255, 255, 255, 0.25);
      border-top-color: currentColor;
      border-radius: 50%;
      animation: spin 0.8s linear infinite;
    }

    /* ── Fullscreen player ─────────────────────────────────────── */

    .fullscreen-player {
      position: fixed;
      inset: 0;
      z-index: 100;
      background: var(--bg);
      overflow: hidden;
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
      padding: env(safe-area-inset-top, 14px) 24px calc(env(safe-area-inset-bottom, 14px) + 20px);
      box-sizing: border-box;
      color: #fff;
    }

    .fs-topbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      position: relative;
      padding: 8px 0 16px;
    }

    .fs-topbar-spacer {
      width: 40px;
    }

    .swipe-handle {
      width: 36px;
      height: 4px;
      border-radius: 2px;
      background: rgba(255, 255, 255, 0.35);
      position: absolute;
      left: 50%;
      transform: translateX(-50%);
    }

    .fs-icon-btn {
      background: rgba(255, 255, 255, 0.1);
      border: none;
      color: rgba(255, 255, 255, 0.85);
      border-radius: 999px;
      width: 38px;
      height: 38px;
      display: grid;
      place-items: center;
      flex-shrink: 0;
    }

    .fs-format-badge {
      align-self: center;
      font-size: 0.65rem;
      font-weight: 700;
      letter-spacing: 0.08em;
      padding: 3px 10px;
      border-radius: 12px;
      border: 1px solid rgba(255, 255, 255, 0.2);
      background: rgba(255, 255, 255, 0.08);
      color: rgba(255, 255, 255, 0.8);
    }

    .fs-cover-wrap {
      flex: 1;
      min-height: 0;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 8px 0;
    }

    .fs-cover {
      width: 100%;
      max-width: min(320px, calc(100vw - 56px));
      aspect-ratio: 1;
      border-radius: 14px;
      object-fit: cover;
      background: rgba(255, 255, 255, 0.1);
      box-shadow: 0 24px 60px rgba(0, 0, 0, 0.6);
    }

    .fs-cover--placeholder {
      background: rgba(255, 255, 255, 0.08);
    }

    .fs-info {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 8px;
      padding: 8px 0 6px;
    }

    .fs-info-text {
      flex: 1;
      min-width: 0;
    }

    .fs-info-actions {
      flex-shrink: 0;
      padding-top: 2px;
    }

    .fs-title {
      font-size: 1.2rem;
      font-weight: 700;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .fs-artist {
      border: none;
      background: none;
      color: rgba(255, 255, 255, 0.75);
      font-size: 0.9rem;
      margin-top: 4px;
      padding: 0;
      display: block;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    /* ── Seek bar ─────────────────────────────────────────────── */

    .seek-row {
      display: grid;
      grid-template-columns: auto 1fr auto;
      gap: 10px;
      align-items: center;
      padding: 10px 0;
    }

    .time {
      font-size: 0.78rem;
      color: rgba(255, 255, 255, 0.75);
      font-variant-numeric: tabular-nums;
    }

    .seek-track-wrap {
      position: relative;
      height: 28px;
      display: flex;
      align-items: center;
    }

    .seek-buffered,
    .seek-played {
      position: absolute;
      left: 0;
      height: 3px;
      border-radius: 2px;
      pointer-events: none;
    }

    .seek-buffered {
      background: rgba(255, 255, 255, 0.18);
    }

    .seek-played {
      background: var(--accent);
    }

    .seek-track-wrap input[type="range"] {
      position: relative;
      width: 100%;
      z-index: 1;
      appearance: none;
      background: rgba(255, 255, 255, 0.18);
      height: 3px;
      border-radius: 2px;
    }

    .seek-track-wrap input[type="range"]::-webkit-slider-thumb {
      appearance: none;
      width: 14px;
      height: 14px;
      border-radius: 50%;
      background: #fff;
      box-shadow: 0 1px 4px rgba(0, 0, 0, 0.4);
    }

    /* ── Main controls ────────────────────────────────────────── */

    .fs-controls {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 6px 0 4px;
    }

    .fs-btn {
      border: none;
      border-radius: 999px;
      color: #fff;
      background: none;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: 3px;
      cursor: pointer;
    }

    .fs-btn--icon {
      width: 46px;
      height: 46px;
      background: rgba(255, 255, 255, 0.1);
      position: relative;
    }

    .fs-btn--icon.active {
      background: rgba(255, 255, 255, 0.22);
    }

    .fs-btn--prev,
    .fs-btn--next {
      width: 58px;
      height: 58px;
      background: rgba(255, 255, 255, 0.1);
    }

    .fs-btn--play {
      width: 72px;
      height: 72px;
      background: var(--accent);
      box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
    }

    .skip-label {
      font-size: 0.65rem;
      font-weight: 700;
      opacity: 0.85;
      line-height: 1;
    }

    .speed-label {
      font-size: 0.75rem;
      font-weight: 700;
    }

    .sleep-badge {
      position: absolute;
      top: 4px;
      right: 4px;
      font-size: 0.55rem;
      font-weight: 700;
      background: var(--accent);
      color: #fff;
      border-radius: 999px;
      padding: 1px 4px;
      line-height: 1.2;
    }

    .spin-ring-fs {
      width: 28px;
      height: 28px;
      border: 3px solid rgba(255, 255, 255, 0.3);
      border-top-color: #fff;
      border-radius: 50%;
      animation: spin 0.8s linear infinite;
    }

    /* ── Volume ───────────────────────────────────────────────── */

    .fs-volume {
      display: grid;
      grid-template-columns: 16px 1fr 16px;
      gap: 10px;
      align-items: center;
      padding-top: 14px;
      color: rgba(255, 255, 255, 0.6);
    }

    .volume-slider-wrap {
      position: relative;
      height: 28px;
      display: flex;
      align-items: center;
    }

    .volume-slider-fill {
      position: absolute;
      left: 0;
      height: 3px;
      background: var(--accent);
      border-radius: 2px;
      pointer-events: none;
    }

    .volume-slider-wrap input[type="range"] {
      position: relative;
      width: 100%;
      z-index: 1;
      appearance: none;
      background: rgba(255, 255, 255, 0.18);
      height: 3px;
      border-radius: 2px;
    }

    .volume-slider-wrap input[type="range"]::-webkit-slider-thumb {
      appearance: none;
      width: 14px;
      height: 14px;
      border-radius: 50%;
      background: #fff;
      box-shadow: 0 1px 4px rgba(0, 0, 0, 0.4);
    }

    /* ── Bottom sheets ────────────────────────────────────────── */

    .bottom-sheet {
      position: fixed;
      left: 10px;
      right: 10px;
      bottom: calc(env(safe-area-inset-bottom) + 8px);
      background: color-mix(in srgb, var(--bg-elevated) 85%, #000);
      border: 1px solid var(--border);
      border-radius: 16px;
      z-index: 110;
      padding: 8px 10px 12px;
    }

    .sheet-overlay {
      position: fixed;
      inset: 0;
      border: none;
      background: transparent;
      z-index: 105;
    }

    .sheet-handle {
      width: 32px;
      height: 4px;
      border-radius: 999px;
      background: var(--text-muted);
      margin: 2px auto 10px;
      opacity: 0.5;
    }

    .sheet-title {
      margin: 0 0 10px;
      text-align: center;
      font-size: 0.88rem;
      color: var(--text);
      font-weight: 600;
    }

    .sheet-grid {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 8px;
    }

    .sheet-item {
      border: 1px solid var(--border);
      background: var(--bg);
      color: var(--text);
      border-radius: 10px;
      height: 36px;
      font-size: 0.82rem;
      font-weight: 600;
    }

    .sheet-item.active {
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
    }
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
