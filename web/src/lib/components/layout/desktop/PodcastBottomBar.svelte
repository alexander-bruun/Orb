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
    closePodcast,
    formatPodcastMs,
  } from "$lib/stores/player/podcastPlayer";
  import { getApiBase } from "$lib/api/base";
  import { goto } from "$app/navigation";

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

  function goToPodcast() {
    if ($currentEpisode) goto(`/podcasts/${$currentEpisode.podcast_id}`);
  }
</script>

<footer class="pod-bar bottom-bar">
  <!-- Left: cover + metadata -->
  <div class="pod-info">
    {#if $currentEpisode}
      <button class="pod-cover-btn" on:click={goToPodcast} aria-label="Go to podcast">
        {#if $currentPodcast?.cover_art_key}
          <img
            src="{getApiBase()}/covers/podcast/{$currentEpisode.podcast_id}"
            alt="cover"
            class="pod-cover"
          />
        {:else}
          <div class="pod-cover pod-cover-placeholder">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
              <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
              <line x1="12" y1="19" x2="12" y2="23"/>
              <line x1="8" y1="23" x2="16" y2="23"/>
            </svg>
          </div>
        {/if}
      </button>
      <div class="pod-meta">
        <div class="pod-title">{$currentEpisode.title}</div>
        {#if $currentPodcast}
          <div class="pod-sub">{$currentPodcast.title}</div>
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
          <path d="M12 5V1L7 6l5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6H4c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z"/>
          <text x="12" y="15.5" text-anchor="middle" font-size="5.5" font-family="sans-serif" font-weight="bold" fill="currentColor">15</text>
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
          <path d="M18 13c0 3.31-2.69 6-6 6s-6-2.69-6-6 2.69-6 6-6v4l5-5-5-5v4c-4.42 0-8 3.58-8 8s3.58 8 8 8 8-3.58 8-8h-2z"/>
          <text x="12" y="15.5" text-anchor="middle" font-size="5.5" font-family="sans-serif" font-weight="bold" fill="currentColor">30</text>
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

    <!-- Right: close -->
    <div class="pod-right">
      <button class="ctrl-btn" on:click={closePodcast} title="Close" aria-label="Close podcast player">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="18" y1="6" x2="6" y2="18"/>
          <line x1="6" y1="6" x2="18" y2="18"/>
        </svg>
      </button>
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

  .pod-cover-btn {
    background: none;
    border: none;
    padding: 0;
    cursor: pointer;
    flex-shrink: 0;
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
    transition: background 0.15s, color 0.15s;
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

  .spin-ring {
    width: 16px;
    height: 16px;
    border: 2px solid var(--border);
    border-top-color: var(--text);
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
