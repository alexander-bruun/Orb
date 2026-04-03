<script lang="ts">
  import {
    currentEpisode,
    currentPodcast,
    podcastPlaybackState,
    podcastPositionMs,
    podcastDurationMs,
    togglePodcastPlayPause,
    seekPodcastMs,
    skipForwardPodcast,
    skipBackwardPodcast,
    formatPodcastMs,
    podcastSleepTimerMins,
    setPodcastSleepTimer,
    PODCAST_SLEEP_PRESETS,
  } from "$lib/stores/player/podcastPlayer";
  import {
    podcastSleepTimerEnabled,
    listenAlongEnabled,
  } from "$lib/stores/settings/theme";
  import { engineVolume, setVolume } from "$lib/stores/player/engine";
  import { expanded } from "$lib/components/layout/desktop/coverExpandStore";
  import {
    lpRole,
    lpPanelOpen,
    lpParticipants,
    createAndConnect,
  } from "$lib/stores/social/listenParty";
  import { getApiBase } from "$lib/api/base";

  let volumePopupOpen = false;

  function onVolume(e: Event) {
    setVolume(parseFloat((e.target as HTMLInputElement).value));
  }

  let sleepMenuOpen = false;

  $: progress =
    $podcastDurationMs > 0
      ? ($podcastPositionMs / $podcastDurationMs) * 100
      : 0;

  const THUMB_R = 6;
  let seekWrapWidth = 0;
  $: fillPx =
    seekWrapWidth > 2 * THUMB_R
      ? THUMB_R + (progress / 100) * (seekWrapWidth - 2 * THUMB_R)
      : (seekWrapWidth * progress) / 100;

  function onSeek(e: Event) {
    const pct = parseFloat((e.target as HTMLInputElement).value);
    seekPodcastMs(($podcastDurationMs * pct) / 100);
  }
</script>

<svelte:window
  on:click={(e) => {
    if (sleepMenuOpen && !(e.target as HTMLElement).closest?.(".sleep-wrap"))
      sleepMenuOpen = false;
    if (
      volumePopupOpen &&
      !(e.target as HTMLElement).closest?.(".volume-popup-wrap")
    )
      volumePopupOpen = false;
  }}
/>

<footer class="pod-bar bottom-bar">
  <!-- Left: cover + metadata -->
  <div class="pod-info">
    {#if $currentEpisode}
      {#if !$expanded}
        <div class="cover-hover-wrap">
          {#if $currentPodcast?.cover_art_key}
            <img
              src="{getApiBase()}/covers/podcast/{$currentEpisode.podcast_id}"
              alt="cover"
              class="pod-cover"
            />
          {:else}
            <div class="pod-cover pod-cover-placeholder">
              <svg
                width="18"
                height="18"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path
                  d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"
                />
                <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
                <line x1="12" y1="19" x2="12" y2="23" />
                <line x1="8" y1="23" x2="16" y2="23" />
              </svg>
            </div>
          {/if}
          <button
            class="cover-expand-btn"
            on:click={() => expanded.update((v) => !v)}
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
      <div class="pod-meta">
        <div class="pod-title">{$currentEpisode.title}</div>
        {#if $currentPodcast}
          <a class="pod-sub pod-sub-link" href="/podcasts/{$currentPodcast.id}"
            >{$currentPodcast.title}</a
          >
        {/if}
      </div>
    {/if}
  </div>

  <!-- Controls + seek -->
  <div class="pod-playback">
    <div class="pod-controls">
      <button
        class="ctrl-btn"
        on:click={() => skipBackwardPodcast(15)}
        title="Back 15 s"
        aria-label="Skip back 15 seconds"
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
            fill="currentColor">15</text
          >
        </svg>
      </button>

      <button
        class="ctrl-btn play-btn"
        on:click={togglePodcastPlayPause}
        aria-label={$podcastPlaybackState === "playing" ? "Pause" : "Play"}
      >
        {#if $podcastPlaybackState === "playing"}
          <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor">
            <rect x="6" y="4" width="4" height="16" rx="1" />
            <rect x="14" y="4" width="4" height="16" rx="1" />
          </svg>
        {:else if $podcastPlaybackState === "loading"}
          <div class="spin-ring"></div>
        {:else}
          <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor">
            <polygon points="5,3 19,12 5,21" />
          </svg>
        {/if}
      </button>

      <button
        class="ctrl-btn"
        on:click={() => skipForwardPodcast(30)}
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

    <div class="pod-center">
      <div class="pod-seek-row">
        <span class="time">{formatPodcastMs($podcastPositionMs)}</span>
        <div class="pod-seek-wrap" bind:clientWidth={seekWrapWidth}>
          <div class="seek-track">
            <div class="seek-fill" style="width: {fillPx}px"></div>
          </div>
          <input
            type="range"
            min="0"
            max="100"
            step="0.05"
            value={progress}
            on:input={onSeek}
            class="seek-input"
            aria-label="Seek"
          />
        </div>
        <span class="time">{formatPodcastMs($podcastDurationMs)}</span>
      </div>
    </div>

    <!-- Wide: inline volume slider -->
    <input
      type="range"
      min="0"
      max="1"
      step="0.01"
      value={$engineVolume}
      on:input={onVolume}
      class="volume-bar wide-only"
      aria-label="Volume"
    />
    <!-- Narrow (≤800 px): vertical volume popup -->
    <div class="volume-popup-wrap">
      <button
        class="ctrl-btn icon-btn narrow-only"
        on:click|stopPropagation={() => {
          volumePopupOpen = !volumePopupOpen;
        }}
        aria-label="Volume"
        title="Volume"
      >
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
          <path d="M19.07 4.93a10 10 0 0 1 0 14.14" />
        </svg>
      </button>
      {#if volumePopupOpen}
        <div class="volume-popup" role="dialog" aria-label="Volume slider">
          <input
            type="range"
            min="0"
            max="1"
            step="0.01"
            value={$engineVolume}
            on:input={onVolume}
            class="volume-vertical"
            aria-label="Volume"
          />
        </div>
      {/if}
    </div>

    <!-- Right: sleep timer -->
    <div class="pod-right">
      {#if $listenAlongEnabled}
        {#if $lpRole === "host"}
          <button
            class="ctrl-btn icon-btn party-btn"
            class:active={$lpPanelOpen}
            on:click={() => lpPanelOpen.update((v) => !v)}
            title="Listen Along"
            aria-label="Listen Along panel"
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
              aria-hidden="true"
            >
              <circle cx="9" cy="7" r="3" /><path
                d="M3 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2"
              />
              <circle cx="18" cy="7" r="2.5" /><path
                d="M22 21v-1.5a3.5 3.5 0 0 0-3.5-3.5H17"
              />
            </svg>
            {#if $lpParticipants.length > 0}
              <span class="party-count">{$lpParticipants.length}</span>
            {/if}
          </button>
        {:else if $lpRole === null}
          <button
            class="ctrl-btn icon-btn party-btn"
            on:click={createAndConnect}
            title="Start Listen Along"
            aria-label="Start Listen Along"
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
              aria-hidden="true"
            >
              <circle cx="9" cy="7" r="3" /><path
                d="M3 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2"
              />
              <circle cx="18" cy="7" r="2.5" /><path
                d="M22 21v-1.5a3.5 3.5 0 0 0-3.5-3.5H17"
              />
            </svg>
          </button>
        {/if}
      {/if}
      {#if $podcastSleepTimerEnabled}
        <div class="sleep-wrap">
          <button
            class="ctrl-btn icon-btn"
            class:active={$podcastSleepTimerMins > 0}
            on:click|stopPropagation={() => {
              sleepMenuOpen = !sleepMenuOpen;
            }}
            title={$podcastSleepTimerMins > 0
              ? `Sleep in ${$podcastSleepTimerMins} min`
              : "Sleep timer"}
            aria-label="Sleep timer"
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
            </svg>
            {#if $podcastSleepTimerMins > 0}
              <span class="sleep-badge">{$podcastSleepTimerMins}m</span>
            {/if}
          </button>
          {#if sleepMenuOpen}
            <div class="sleep-menu" role="menu">
              {#each PODCAST_SLEEP_PRESETS as preset}
                <button
                  class="sleep-item"
                  class:selected={$podcastSleepTimerMins === preset}
                  role="menuitem"
                  on:click={() => {
                    setPodcastSleepTimer(
                      $podcastSleepTimerMins === preset ? 0 : preset,
                    );
                    sleepMenuOpen = false;
                  }}>{preset} min</button
                >
              {/each}
              {#if $podcastSleepTimerMins > 0}
                <button
                  class="sleep-item sleep-cancel"
                  role="menuitem"
                  on:click={() => {
                    setPodcastSleepTimer(0);
                    sleepMenuOpen = false;
                  }}
                >
                  Cancel timer
                </button>
              {/if}
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</footer>

<style>
  .pod-bar {
    display: flex;
    align-items: center;
    height: var(--bottom-h);
    background: var(--bg-elevated);
    border-top: 1px solid var(--border);
    flex-shrink: 0;
    gap: 0;
  }

  .pod-info {
    width: var(--sidebar-w);
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 8px 0 16px;
    min-width: 0;
  }

  .cover-hover-wrap {
    position: relative;
    flex-shrink: 0;
    width: 44px;
    height: 44px;
  }
  .cover-hover-wrap .cover-expand-btn {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    background: rgba(0, 0, 0, 0.45);
    border: none;
    border-radius: 4px;
    color: #fff;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    opacity: 0;
    transition: opacity 0.15s;
  }
  .cover-hover-wrap:hover .cover-expand-btn {
    opacity: 1;
  }

  .pod-cover {
    width: 44px;
    height: 44px;
    border-radius: 4px;
    object-fit: cover;
    display: block;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
    background: var(--bg-hover);
  }
  .pod-cover-placeholder {
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
  }

  .pod-meta {
    min-width: 0;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .pod-title {
    font-size: 0.88rem;
    font-weight: 500;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .pod-sub {
    font-size: 0.75rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .pod-sub-link {
    text-decoration: none;
    transition: color 0.15s;
  }
  .pod-sub-link:hover {
    color: var(--text);
  }

  .pod-playback {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 16px 0 0;
    min-width: 0;
  }

  .pod-controls {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 2px;
  }

  .pod-center {
    flex: 1;
    display: flex;
    align-items: center;
    min-width: 0;
  }

  .pod-right {
    flex-shrink: 0;
    display: flex;
    align-items: center;
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

  .pod-seek-row {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
  }
  .time {
    font-size: 0.72rem;
    color: var(--text-muted);
    width: 38px;
    text-align: center;
    flex-shrink: 0;
    font-variant-numeric: tabular-nums;
  }

  .pod-seek-wrap {
    flex: 1;
    position: relative;
    height: 18px;
    display: flex;
    align-items: center;
  }
  .seek-track {
    position: absolute;
    left: 0;
    right: 0;
    height: 3px;
    border-radius: 2px;
    background: var(--border);
    overflow: hidden;
    pointer-events: none;
  }
  .seek-fill {
    height: 100%;
    background: var(--accent);
    border-radius: 2px;
  }
  .seek-input {
    position: absolute;
    left: 0;
    right: 0;
    width: 100%;
    margin: 0;
    opacity: 0;
    cursor: pointer;
    height: 18px;
  }

  /* ── Volume ── */
  .volume-bar {
    width: 80px;
    height: 4px;
    accent-color: var(--accent);
    cursor: pointer;
  }
  .volume-popup-wrap {
    position: relative;
    display: none;
    align-items: center;
  }
  .volume-popup {
    position: absolute;
    bottom: calc(100% + 10px);
    left: 50%;
    transform: translateX(-50%);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px 8px;
    height: 100px;
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.25);
  }
  .volume-vertical {
    writing-mode: vertical-lr;
    direction: rtl;
    width: 4px;
    height: 80px;
    accent-color: var(--accent);
    cursor: pointer;
  }
  @media (max-width: 800px) {
    .wide-only {
      display: none !important;
    }
    .volume-popup-wrap {
      display: inline-flex;
    }
    .narrow-only {
      display: inline-flex !important;
    }
  }
  .narrow-only {
    display: none;
  }
  .icon-btn {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 0;
    padding: 6px;
  }

  /* ── Sleep timer ── */
  .sleep-wrap {
    position: relative;
    display: inline-flex;
    align-items: center;
  }
  .icon-btn {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 0;
    padding: 6px;
  }
  .icon-btn.active {
    color: var(--accent);
  }
  .party-btn {
    position: relative;
  }
  .party-count {
    position: absolute;
    top: 1px;
    right: 0;
    font-size: 9px;
    font-weight: 700;
    line-height: 1;
    color: var(--accent);
    pointer-events: none;
  }
  .sleep-badge {
    position: absolute;
    top: 0;
    right: -2px;
    font-size: 8px;
    font-weight: 700;
    line-height: 1;
    color: var(--accent);
    pointer-events: none;
    white-space: nowrap;
  }
  .sleep-menu {
    position: absolute;
    bottom: calc(100% + 8px);
    right: 0;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 4px;
    min-width: 120px;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.25);
    z-index: 200;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .sleep-item {
    background: none;
    border: none;
    border-radius: 5px;
    padding: 7px 12px;
    text-align: left;
    font-size: 0.8rem;
    color: var(--text-muted);
    cursor: pointer;
    white-space: nowrap;
  }
  .sleep-item:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .sleep-item.selected {
    color: var(--accent);
  }
  .sleep-cancel {
    margin-top: 2px;
    border-top: 1px solid var(--border);
    padding-top: 8px;
  }

  .spin-ring {
    width: 16px;
    height: 16px;
    border: 2px solid var(--border);
    border-top-color: var(--text);
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
