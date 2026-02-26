<script lang="ts">
  import {
    currentTrack,
    playbackState,
    positionMs,
    durationMs,
    formattedPosition,
    formattedDuration,
    formattedFormat,
    volume,
    bufferedPct,
    repeatMode,
    shuffle,
    userQueue,
    queueModalOpen,
    togglePlayPause,
    seek,
    setVolume,
    next,
    previous,
    toggleRepeat,
    toggleShuffle
  } from '$lib/stores/player';
  import { library } from '$lib/api/library';
  import { writable } from 'svelte/store';
  import { expanded } from './coverExpandStore';

  const currentAlbum = writable<{ id: string; title: string } | null>(null);
  const BASE = import.meta.env.VITE_API_BASE ?? '/api';

  $: {
    if ($currentTrack?.album_id) {
      library.album($currentTrack.album_id)
        .then(res => currentAlbum.set({ id: res.album.id, title: res.album.title }))
        .catch(() => currentAlbum.set(null));
    } else {
      currentAlbum.set(null);
    }
  }

  $: progress = $durationMs > 0 ? ($positionMs / $durationMs) * 100 : 0;

  function onSeek(e: Event) {
    const input = e.target as HTMLInputElement;
    const pct = parseFloat(input.value);
    const seconds = ($durationMs / 1000) * (pct / 100);
    seek(seconds);
  }

  function onVolume(e: Event) {
    const input = e.target as HTMLInputElement;
    setVolume(parseFloat(input.value));
  }

  function toggleExpand() {
    expanded.update(v => !v);
  }
</script>

<footer class="bottom-bar">
  <!-- Left section: fixed width matching sidebar — content truncates at edge -->
  <div class="info-section">
    {#if $currentTrack}
      {#if !$expanded}
        <!-- Small mode: cover with hover-reveal expand button -->
        <div class="cover-hover-wrap">
          {#if $currentTrack.album_id}
            <img src="{BASE}/covers/{$currentTrack.album_id}"
                 alt="album art"
                 class="bottom-cover animate-in" />
          {:else}
            <div class="bottom-cover placeholder animate-in"></div>
          {/if}
          <button class="cover-expand-btn" on:click={toggleExpand} aria-label="Expand cover">
            <svg width="16" height="16" viewBox="0 0 20 20"><path d="M4 4h12v12H4V4zm2 2v8h8V6H6z" fill="currentColor"/></svg>
          </button>
        </div>
      {/if}
      <!-- Track metadata: always visible when a track is loaded -->
      <div class="track-meta" class:full-width={$expanded}>
        {#if $currentAlbum}
          <div class="album-title">
            <a href="/library/albums/{$currentAlbum.id}" class="album-link">{$currentAlbum.title}</a>
          </div>
        {/if}
        <div class="song-title">{$currentTrack.title}</div>
      </div>
    {:else}
      <!-- Skeleton placeholder when no track is loaded -->
      <div class="skeleton-cover"></div>
      <div class="track-meta">
        <div class="skeleton-line skeleton-album"></div>
        <div class="skeleton-line skeleton-title"></div>
      </div>
    {/if}
  </div>

  <!-- Playback section: always starts at the sidebar edge, controls never shift -->
  <div class="playback-section">
    <div class="controls">
      <button
        class="ctrl-btn icon-btn"
        class:active={$shuffle}
        on:click={toggleShuffle}
        aria-label="Shuffle"
        title="Shuffle"
      >
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/>
          <polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/>
          <line x1="4" y1="4" x2="9" y2="9"/>
        </svg>
      </button>
      <button class="ctrl-btn" on:click={previous} aria-label="Previous">⏮</button>
      <button
        class="ctrl-btn play-btn"
        on:click={togglePlayPause}
        aria-label={$playbackState === 'playing' ? 'Pause' : 'Play'}
      >
        {$playbackState === 'playing' ? '⏸' : '▶'}
      </button>
      <button class="ctrl-btn" on:click={next} aria-label="Next">⏭</button>
      <button
        class="ctrl-btn icon-btn"
        class:active={$repeatMode !== 'off'}
        on:click={toggleRepeat}
        aria-label="Repeat"
        title={$repeatMode === 'off' ? 'Repeat off' : $repeatMode === 'one' ? 'Repeat one' : 'Repeat all'}
      >
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/>
          <polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/>
        </svg>
        {#if $repeatMode === 'one'}
          <span class="one-badge">1</span>
        {/if}
      </button>
    </div>

    <div class="seek-area">
      <span class="time">{$formattedPosition}</span>
      <div class="seek-bar-wrap">
        <div class="seek-track">
          <div class="seek-buffered" style="width: {$bufferedPct}%"></div>
          <div class="seek-progress" style="width: {progress}%"></div>
        </div>
        <input
          type="range"
          min="0"
          max="100"
          step="0.1"
          value={progress}
          on:input={onSeek}
          class="seek-input"
          aria-label="Seek"
        />
      </div>
      <span class="time">{$formattedDuration}</span>
    </div>

    <div class="right-controls">
      <input
        type="range"
        min="0"
        max="1"
        step="0.01"
        value={$volume}
        on:input={onVolume}
        class="volume-bar"
        aria-label="Volume"
      />
      {#if $userQueue.length > 1}
        <button
          class="ctrl-btn icon-btn queue-btn"
          class:active={$queueModalOpen}
          on:click={() => queueModalOpen.update(v => !v)}
          aria-label="Up Next queue"
          title="Up Next"
        >
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <line x1="8" y1="6" x2="21" y2="6"/>
            <line x1="8" y1="12" x2="21" y2="12"/>
            <line x1="8" y1="18" x2="21" y2="18"/>
            <polyline points="3,6 4,7 6,5"/>
            <polyline points="3,12 4,13 6,11"/>
            <polyline points="3,18 4,19 6,17"/>
          </svg>
          <span class="queue-count">{$userQueue.length}</span>
        </button>
      {/if}
      {#if $formattedFormat}
        <span class="format-badge">{$formattedFormat}</span>
      {/if}
    </div>
  </div>
</footer>

<style>
  .bottom-bar {
    display: flex;
    align-items: center;
    height: var(--bottom-h);
    background: var(--bg-elevated);
    border-top: 1px solid var(--border);
    flex-shrink: 0;
  }

  /* Fixed-width left section: aligned to sidebar, text truncates at the edge */
  .info-section {
    width: var(--sidebar-w);
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 0 8px 0 20px;
    overflow: hidden;
  }

  /* Cover with hover-reveal expand button */
  .cover-hover-wrap {
    position: relative;
    flex-shrink: 0;
    width: 40px;
    height: 40px;
  }
  .cover-hover-wrap .cover-expand-btn {
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
  .cover-hover-wrap:hover .cover-expand-btn {
    opacity: 1;
  }

  .bottom-cover {
    width: 40px;
    height: 40px;
    border-radius: 4px;
    object-fit: contain;
    flex-shrink: 0;
    background: var(--bg-hover);
    display: block;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }
  .placeholder { background: var(--bg-hover); }

  /* Skeleton placeholders */
  @keyframes skeleton-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
  .skeleton-cover {
    width: 40px;
    height: 40px;
    border-radius: 4px;
    background: var(--bg-hover);
    flex-shrink: 0;
    animation: skeleton-pulse 1.6s ease-in-out infinite;
  }
  .skeleton-line {
    border-radius: 3px;
    background: var(--bg-hover);
    animation: skeleton-pulse 1.6s ease-in-out infinite;
  }
  .skeleton-album {
    width: 60%;
    height: 10px;
    animation-delay: 0.1s;
  }
  .skeleton-title {
    width: 85%;
    height: 13px;
  }

  .animate-in {
    animation: fadeInSlide 0.25s cubic-bezier(0.4,0,0.2,1);
  }
  @keyframes fadeInSlide {
    from { opacity: 0; transform: translateX(-12px) scale(0.95); }
    to   { opacity: 1; transform: translateX(0) scale(1); }
  }

  .track-meta {
    min-width: 0;
    flex: 1;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .album-title {
    font-size: 0.8rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .album-link {
    color: var(--text-muted);
    text-decoration: none;
    transition: color 0.15s;
  }
  .album-link:hover { color: var(--accent); text-decoration: underline; }
  .song-title {
    font-size: 0.9rem;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* Playback section: fills the rest, controls are the first item so they never move */
  .playback-section {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 0 20px 0 8px;
    min-width: 0;
  }

  .controls {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .ctrl-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 1.2rem;
    line-height: 1;
    padding: 0 6px;
    height: 28px;
    transition: color 0.15s;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .ctrl-btn:hover { color: var(--text); }
  .play-btn { font-size: 1.6rem; color: var(--text); width: 36px; text-align: center; }

  /* Shuffle / repeat icon buttons */
  .icon-btn {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 0; /* suppress any stray text sizing */
    padding: 6px;
  }
  .icon-btn.active { color: var(--accent); }
  .icon-btn.active::after {
    content: '';
    position: absolute;
    bottom: 3px;
    left: 50%;
    transform: translateX(-50%);
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: var(--accent);
  }
  /* "1" badge overlaid on the repeat icon */
  .one-badge {
    position: absolute;
    bottom: 2px;
    right: 1px;
    font-size: 9px;
    font-weight: 700;
    line-height: 1;
    color: var(--accent);
    pointer-events: none;
  }

  .seek-area {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .time { font-size: 0.75rem; color: var(--text-muted); width: 36px; flex-shrink: 0; }

  /* Custom seek bar: layered track divs + transparent native input on top */
  .seek-bar-wrap {
    flex: 1;
    position: relative;
    height: 4px;
    display: flex;
    align-items: center;
    min-width: 0;
  }
  .seek-track {
    position: absolute;
    left: 0; right: 0;
    height: 4px;
    background: var(--bg-hover);
    border-radius: 2px;
    overflow: hidden;
  }
  .seek-buffered {
    position: absolute;
    height: 100%;
    background: rgba(160, 160, 160, 0.45);
    transition: width 0.4s ease;
    pointer-events: none;
  }
  .seek-progress {
    position: absolute;
    height: 100%;
    background: var(--accent);
    pointer-events: none;
  }
  /* Range input sits above the track divs; its track is transparent so only the thumb shows */
  .seek-input {
    position: absolute;
    left: 0; right: 0;
    width: 100%;
    margin: 0;
    height: 20px;
    cursor: pointer;
    -webkit-appearance: none;
    appearance: none;
    background: transparent;
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
  /* Thumb hidden at rest, shown on hover */
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
  .seek-bar-wrap:hover .seek-track { height: 6px; }
  .seek-bar-wrap:hover .seek-input::-webkit-slider-thumb { opacity: 1; }
  .seek-bar-wrap:hover .seek-input::-moz-range-thumb { opacity: 1; }

  .right-controls { flex-shrink: 0; display: flex; align-items: center; gap: 12px; }
  .volume-bar { width: 80px; height: 4px; accent-color: var(--accent); cursor: pointer; }

  /* Queue button with item count badge */
  .queue-btn {
    position: relative;
    padding: 8px 6px 10px;
  }
  .queue-count {
    position: absolute;
    top: 1px;
    right: 0px;
    font-size: 9px;
    font-weight: 700;
    line-height: 1;
    color: var(--accent);
    pointer-events: none;
  }
  .format-badge {
    font-family: 'DM Mono', monospace;
    font-size: 10px;
    letter-spacing: 0.08em;
    color: var(--accent);
    background: var(--accent-dim);
    border: 1px solid var(--accent-glow);
    border-radius: 4px;
    padding: 3px 8px;
    white-space: nowrap;
  }
</style>
