<script lang="ts">
  import {
    currentTrack,
    playbackState,
    positionMs,
    durationMs,
    formattedPosition,
    formattedDuration,
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
    toggleShuffle,
  } from '$lib/stores/player';
  import { library } from '$lib/api/library';
  import { favorites } from '$lib/stores/favorites';
  import { writable } from 'svelte/store';
  import { getApiBase } from '$lib/api/base';
  import { lyricsOpen, lyricsLines, lyricsLoading, activeLyricIndex } from '$lib/stores/lyrics';
  import { goto } from '$app/navigation';

  const currentAlbum = writable<{ id: string; title: string; artist?: string } | null>(null);

  $: isFavorite = $currentTrack ? $favorites.has($currentTrack.id) : false;

  async function toggleFavorite() {
    if (!$currentTrack) return;
    await favorites.toggle($currentTrack.id);
  }

  let playerOpen = false;

  // Lyrics view
  let showLyrics = false;
  let lyricsContainer: HTMLElement | undefined;
  let lastScrolledIdx = -2;

  // Reset lyrics view when player closes
  $: if (!playerOpen) showLyrics = false;

  // Auto-scroll to the active lyric line when it changes
  $: if (showLyrics && lyricsContainer && $activeLyricIndex >= 0 && $activeLyricIndex !== lastScrolledIdx) {
    lastScrolledIdx = $activeLyricIndex;
    const el = lyricsContainer.querySelector<HTMLElement>(`[data-lyric-idx="${$activeLyricIndex}"]`);
    if (el) el.scrollIntoView({ block: 'center', behavior: 'smooth' });
  }

  // Touch-swipe to dismiss
  let touchStartY = 0;
  let touchCurrentY = 0;
  let swiping = false;
  let swipeDelta = 0;

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

  // Seek: track visual drag position separately; only actually seek on release
  let seekDragValue: number | null = null;

  function onSeekInput(e: Event) {
    seekDragValue = parseFloat((e.target as HTMLInputElement).value);
  }

  function onSeek(e: Event) {
    const pct = parseFloat((e.target as HTMLInputElement).value);
    seekDragValue = null;
    seek(($durationMs / 1000) * (pct / 100));
  }

  function onVolumeChange(e: Event) {
    setVolume(parseFloat((e.target as HTMLInputElement).value));
  }

  function openPlayer() {
    playerOpen = true;
  }

  function closePlayer() {
    playerOpen = false;
    swipeDelta = 0;
    swiping = false;
  }

  // Swipe down gesture on full screen player
  function onTouchStart(e: TouchEvent) {
    touchStartY = e.touches[0].clientY;
    touchCurrentY = touchStartY;
    swiping = true;
    swipeDelta = 0;
  }

  function onTouchMove(e: TouchEvent) {
    if (!swiping) return;
    touchCurrentY = e.touches[0].clientY;
    swipeDelta = Math.max(0, touchCurrentY - touchStartY);
  }

  function onTouchEnd() {
    if (!swiping) return;
    swiping = false;
    if (swipeDelta > 80) {
      closePlayer();
    } else {
      swipeDelta = 0;
    }
  }

  function goToAlbum(e: MouseEvent) {
    e.stopPropagation();
    if ($currentAlbum) {
      closePlayer();
      goto(`/library/albums/${$currentAlbum.id}`);
    }
  }

  function goToArtist(e: MouseEvent) {
    e.stopPropagation();
    if ($currentTrack?.artist_id) {
      closePlayer();
      goto(`/artists/${$currentTrack.artist_id}`);
    }
  }
</script>

<!-- ── Mini player (shown above bottom nav) ──────────────────────────────── -->
{#if $currentTrack}
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
  <div
    class="mini-player"
    role="complementary"
    aria-label="Now playing"
    on:click={openPlayer}
  >
    <!-- Thin progress line at top -->
    <div class="mini-progress-track">
      <div class="mini-progress-fill" style="width: {progress}%"></div>
    </div>

    <!-- Content row -->
    <div class="mini-content">
      <!-- Cover art -->
      {#if $currentTrack.album_id}
        <img
          src="{getApiBase()}/covers/{$currentTrack.album_id}"
          alt="album art"
          class="mini-cover"
        />
      {:else}
        <div class="mini-cover mini-cover--placeholder"></div>
      {/if}

      <!-- Track info -->
      <div class="mini-info">
        <span class="mini-title">{$currentTrack.title}</span>
        {#if $currentTrack.artist_name}
          <span class="mini-artist">{$currentTrack.artist_name}</span>
        {/if}
      </div>

      <!-- Controls -->
      <div class="mini-controls">
        <button
          class="mini-btn"
          on:click|stopPropagation={togglePlayPause}
          aria-label={$playbackState === 'playing' ? 'Pause' : 'Play'}
        >
          {#if $playbackState === 'playing'}
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
          on:click|stopPropagation={next}
          aria-label="Next"
        >
          <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <polygon points="5,4 15,12 5,20"/>
            <rect x="16" y="4" width="2.5" height="16" rx="1"/>
          </svg>
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- ── Full-screen player ─────────────────────────────────────────────────── -->
{#if playerOpen && $currentTrack}
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div
    class="fullscreen-player"
    style="transform: translateY({swipeDelta}px); transition: {swiping ? 'none' : 'transform 0.3s cubic-bezier(0.4,0,0.2,1)'};"
    on:touchstart={onTouchStart}
    on:touchmove={onTouchMove}
    on:touchend={onTouchEnd}
  >
    <!-- Blurred album art background -->
    {#if $currentTrack.album_id}
      <div
        class="fs-bg"
        style="background-image: url('{getApiBase()}/covers/{$currentTrack.album_id}')"
      ></div>
    {/if}
    <div class="fs-overlay"></div>

    <!-- Content -->
    <div class="fs-content">
      <!-- Top bar: swipe handle + close button -->
      <div class="fs-topbar">
        {#if $lyricsLines.length > 0}
          <!-- svelte-ignore a11y-click-events-have-key-events -->
          <button
            class="fs-lyrics-toggle"
            class:active={showLyrics}
            on:click|stopPropagation={() => { showLyrics = !showLyrics; }}
            aria-label={showLyrics ? 'Show player' : 'Show lyrics'}
            aria-pressed={showLyrics}
          >Lyrics</button>
        {:else}
          <div class="fs-topbar-spacer"></div>
        {/if}
        <div class="swipe-handle"></div>
        <button class="fs-close-btn" on:click={closePlayer} aria-label="Close player">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true">
            <polyline points="6 9 12 15 18 9"/>
          </svg>
        </button>
      </div>

      {#if showLyrics}
        <!-- ── Lyrics panel ───────────────────────────────────────── -->
        <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
        <div
          class="fs-lyrics"
          bind:this={lyricsContainer}
          role="region"
          aria-label="Lyrics"
          on:touchstart|stopPropagation={() => {}}
          on:touchmove|stopPropagation={() => {}}
        >
          {#if $lyricsLoading}
            <p class="fs-lyrics-status">Loading lyrics…</p>
          {:else if $lyricsLines.length === 0}
            <p class="fs-lyrics-status">No lyrics available</p>
          {:else}
            <div class="lyric-spacer-top"></div>
            {#each $lyricsLines as line, i}
              <p
                class="lyric-line"
                class:lyric-active={i === $activeLyricIndex}
                class:lyric-past={i < $activeLyricIndex}
                data-lyric-idx={i}
              >{line.text || '♩'}</p>
            {/each}
            <div class="lyric-spacer-bottom"></div>
          {/if}
        </div>

        <!-- Compact track name above seek bar (lyrics mode) -->
        <div class="fs-info fs-info--compact">
          <div class="fs-title">{$currentTrack.title}</div>
        </div>
      {:else}
        <!-- Album art -->
        <div class="fs-cover-wrap">
          {#if $currentTrack.album_id}
            <img
              src="{getApiBase()}/covers/{$currentTrack.album_id}"
              alt="album art"
              class="fs-cover"
            />
          {:else}
            <div class="fs-cover fs-cover--placeholder"></div>
          {/if}
        </div>

        <!-- Track info -->
        <div class="fs-info">
          <div class="fs-info-text">
            <div class="fs-title">{$currentTrack.title}</div>
            <div class="fs-sub">
              {#if $currentTrack.artist_name}
                <!-- svelte-ignore a11y-click-events-have-key-events -->
                <span class="fs-artist" on:click={goToArtist} role="link" tabindex="0">{$currentTrack.artist_name}</span>
              {/if}
              {#if $currentAlbum}
                <span class="fs-sep">·</span>
                <!-- svelte-ignore a11y-missing-attribute -->
                <!-- svelte-ignore a11y-click-events-have-key-events -->
                <span class="fs-album" on:click={goToAlbum} role="link" tabindex="0">
                  {$currentAlbum.title}
                </span>
              {/if}
            </div>
          </div>
          <button
            class="fs-fav-btn"
            class:active={isFavorite}
            on:click={toggleFavorite}
            aria-label={isFavorite ? 'Remove from favorites' : 'Add to favorites'}
            aria-pressed={isFavorite}
          >
            <svg width="24" height="24" viewBox="0 0 24 24" fill={isFavorite ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/>
            </svg>
          </button>
        </div>
      {/if}

      <!-- Seek bar -->
      <div class="fs-seek">
        <div class="seek-bar-wrap">
          <div class="seek-track">
            <div class="seek-buffered" style="width: {$bufferedPct}%"></div>
            <div class="seek-fill" style="width: {seekDragValue !== null ? seekDragValue : progress}%"></div>
          </div>
          <input
            type="range"
            min="0"
            max="100"
            step="0.1"
            value={seekDragValue !== null ? seekDragValue : progress}
            on:input={onSeekInput}
            on:change={onSeek}
            on:touchstart|stopPropagation={() => {}}
            on:touchmove|stopPropagation={() => {}}
            class="seek-input"
            aria-label="Seek"
          />
        </div>
        <div class="seek-times">
          <span>{$formattedPosition}</span>
          <span>{$formattedDuration}</span>
        </div>
      </div>

      <!-- Main controls -->
      <div class="fs-controls">
        <button
          class="fs-btn fs-btn--icon"
          class:active={$shuffle}
          on:click={toggleShuffle}
          aria-label="Shuffle"
          aria-pressed={$shuffle}
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/>
            <polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/>
            <line x1="4" y1="4" x2="9" y2="9"/>
          </svg>
        </button>

        <button class="fs-btn fs-btn--prev" on:click={previous} aria-label="Previous">
          <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <polygon points="19,4 9,12 19,20"/>
            <rect x="5" y="4" width="2.5" height="16" rx="1"/>
          </svg>
        </button>

        <button
          class="fs-btn fs-btn--play"
          on:click={togglePlayPause}
          aria-label={$playbackState === 'playing' ? 'Pause' : 'Play'}
        >
          {#if $playbackState === 'playing'}
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

        <button class="fs-btn fs-btn--next" on:click={next} aria-label="Next">
          <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <polygon points="5,4 15,12 5,20"/>
            <rect x="16" y="4" width="2.5" height="16" rx="1"/>
          </svg>
        </button>

        <button
          class="fs-btn fs-btn--icon"
          class:active={$repeatMode !== 'off'}
          on:click={toggleRepeat}
          aria-label="Repeat"
          aria-pressed={$repeatMode !== 'off'}
          style="position: relative;"
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/>
            <polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/>
          </svg>
          {#if $repeatMode === 'one'}
            <span class="one-badge">1</span>
          {/if}
        </button>
      </div>

      <!-- Volume row -->
      <div class="fs-volume">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
          <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>
        </svg>
        <input
          type="range"
          min="0"
          max="1"
          step="0.01"
          value={$volume}
          on:input={onVolumeChange}
          on:touchstart|stopPropagation={() => {}}
          on:touchmove|stopPropagation={() => {}}
          class="volume-slider"
          aria-label="Volume"
        />
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
          <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>
          <path d="M15.54 8.46a5 5 0 0 1 0 7.07"/>
          <path d="M19.07 4.93a10 10 0 0 1 0 14.14"/>
        </svg>
      </div>

      <!-- Extras row: queue -->
      <div class="fs-extras">
        {#if $userQueue.length > 1}
          <button
            class="fs-extra-btn"
            class:active={$queueModalOpen}
            on:click={() => queueModalOpen.update(v => !v)}
            aria-label="Queue"
            title="Queue"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
              <line x1="8" y1="6" x2="21" y2="6"/>
              <line x1="8" y1="12" x2="21" y2="12"/>
              <line x1="8" y1="18" x2="21" y2="18"/>
              <polyline points="3,6 4,7 6,5"/>
              <polyline points="3,12 4,13 6,11"/>
              <polyline points="3,18 4,19 6,17"/>
            </svg>
            <span>Queue <span class="queue-count">{$userQueue.length}</span></span>
          </button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  /* ── Mini player ─────────────────────────────────────────────────────────── */
  .mini-player {
    display: none; /* desktop: hidden */
  }

  @media (max-width: 640px) {
    .mini-player {
      display: block;
      position: fixed;
      left: 0;
      right: 0;
      bottom: calc(60px + env(safe-area-inset-bottom));
      background: var(--bg-elevated);
      border-top: 1px solid var(--border);
      z-index: 39;
      cursor: pointer;
      -webkit-tap-highlight-color: transparent;
      user-select: none;
    }

    .mini-progress-track {
      height: 2px;
      background: var(--bg-hover);
      position: relative;
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
      width: 44px;
      height: 44px;
      border-radius: 6px;
      object-fit: contain;
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

    /* ── Full screen player ──────────────────────────────────────────────── */
    .fullscreen-player {
      position: fixed;
      inset: 0;
      z-index: 100;
      display: flex;
      flex-direction: column;
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
      padding: 0 28px;
      padding-top: env(safe-area-inset-top, 16px);
      padding-bottom: calc(env(safe-area-inset-bottom, 16px) + 16px);
      box-sizing: border-box;
    }

    /* Top bar */
    .fs-topbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 8px 0 16px;
      position: relative;
    }

    .fs-topbar-spacer {
      width: 64px;
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

    .fs-lyrics-toggle {
      flex-shrink: 0;
      background: rgba(255, 255, 255, 0.12);
      border: 1px solid rgba(255, 255, 255, 0.2);
      color: rgba(255, 255, 255, 0.7);
      font-size: 0.75rem;
      font-weight: 600;
      letter-spacing: 0.04em;
      padding: 5px 14px;
      border-radius: 20px;
      cursor: pointer;
      -webkit-tap-highlight-color: transparent;
      transition: background 0.15s, color 0.15s, border-color 0.15s;
    }

    .fs-lyrics-toggle.active {
      background: rgba(255, 255, 255, 0.22);
      border-color: rgba(255, 255, 255, 0.5);
      color: #fff;
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

    /* Album art */
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
      object-fit: contain;
      background: rgba(255, 255, 255, 0.05);
      box-shadow: 0 24px 60px rgba(0, 0, 0, 0.6);
    }

    .fs-cover--placeholder {
      background: rgba(255, 255, 255, 0.08);
    }

    /* ── Lyrics panel ────────────────────────────────────────────── */
    .fs-lyrics {
      flex: 1;
      min-height: 0;
      overflow-y: auto;
      overflow-x: hidden;
      -webkit-overflow-scrolling: touch;
      padding: 0 4px;
      /* Hide scrollbar */
      scrollbar-width: none;
    }

    .fs-lyrics::-webkit-scrollbar {
      display: none;
    }

    .lyric-spacer-top,
    .lyric-spacer-bottom {
      height: 40%;
    }

    .lyric-line {
      font-size: 1.55rem;
      font-weight: 700;
      line-height: 1.3;
      color: rgba(255, 255, 255, 0.22);
      margin: 0 0 22px;
      padding: 0;
      transition: color 0.35s ease, font-size 0.2s ease;
      cursor: default;
      word-break: break-word;
    }

    .lyric-line.lyric-active {
      color: #fff;
      font-size: 1.75rem;
    }

    .lyric-line.lyric-past {
      color: rgba(255, 255, 255, 0.38);
    }

    .fs-lyrics-status {
      color: rgba(255, 255, 255, 0.45);
      font-size: 0.9rem;
      text-align: center;
      padding: 48px 16px;
    }

    /* Compact track info (used in lyrics view above seek bar) */
    .fs-info--compact {
      padding: 8px 0 10px;
      flex-shrink: 0;
    }

    .fs-info--compact .fs-title {
      font-size: 1rem;
      font-weight: 600;
      color: rgba(255, 255, 255, 0.85);
      margin-bottom: 0;
    }

    /* Track info */
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

    .fs-fav-btn {
      flex-shrink: 0;
      background: none;
      border: none;
      padding: 8px;
      cursor: pointer;
      color: rgba(255, 255, 255, 0.5);
      transition: color 0.15s, transform 0.1s;
      -webkit-tap-highlight-color: transparent;
    }

    .fs-fav-btn.active {
      color: #e85050;
    }

    .fs-fav-btn:active {
      transform: scale(0.88);
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

    .fs-sep {
      flex-shrink: 0;
      color: rgba(255, 255, 255, 0.4);
    }

    .fs-album {
      overflow: hidden;
      text-overflow: ellipsis;
      cursor: pointer;
    }

    /* Seek bar */
    .fs-seek {
      flex-shrink: 0;
      padding: 4px 0 12px;
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
      overflow: hidden;
    }

    .seek-buffered {
      position: absolute;
      height: 100%;
      background: rgba(255, 255, 255, 0.3);
      pointer-events: none;
    }

    .seek-fill {
      position: absolute;
      height: 100%;
      background: #fff;
      pointer-events: none;
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
      touch-action: none;
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
      width: 16px;
      height: 16px;
      border-radius: 50%;
      background: #fff;
      margin-top: -6px;
      cursor: pointer;
    }

    .seek-input::-moz-range-thumb {
      width: 16px;
      height: 16px;
      border-radius: 50%;
      background: #fff;
      border: none;
      cursor: pointer;
    }

    .seek-times {
      display: flex;
      justify-content: space-between;
      font-size: 0.72rem;
      color: rgba(255, 255, 255, 0.55);
      font-variant-numeric: tabular-nums;
    }

    /* Main controls */
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
      transition: color 0.15s, background 0.1s;
      -webkit-tap-highlight-color: transparent;
      position: relative;
    }

    .fs-btn:active {
      background: rgba(255, 255, 255, 0.1);
    }

    .fs-btn--icon {
      color: rgba(255, 255, 255, 0.5);
    }

    .fs-btn--icon.active {
      color: var(--accent);
    }

    .fs-btn--icon.active::after {
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

    .fs-btn--prev,
    .fs-btn--next {
      color: #fff;
    }

    .fs-btn--play {
      width: 68px;
      height: 68px;
      background: #fff;
      color: #000;
      border-radius: 50%;
      box-shadow: 0 4px 20px rgba(0, 0, 0, 0.4);
    }

    .fs-btn--play:active {
      background: rgba(255, 255, 255, 0.85);
    }

    .one-badge {
      position: absolute;
      bottom: 3px;
      right: 2px;
      font-size: 9px;
      font-weight: 700;
      line-height: 1;
      color: var(--accent);
      pointer-events: none;
    }

    /* Volume */
    .fs-volume {
      flex-shrink: 0;
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 0 0 16px;
      color: rgba(255, 255, 255, 0.45);
    }

    .volume-slider {
      flex: 1;
      height: 4px;
      accent-color: #fff;
      cursor: pointer;
      -webkit-appearance: none;
      appearance: none;
      background: rgba(255, 255, 255, 0.25);
      border-radius: 2px;
      touch-action: none;
    }

    .volume-slider::-webkit-slider-thumb {
      -webkit-appearance: none;
      width: 14px;
      height: 14px;
      border-radius: 50%;
      background: #fff;
      cursor: pointer;
    }

    .volume-slider::-moz-range-thumb {
      width: 14px;
      height: 14px;
      border-radius: 50%;
      background: #fff;
      border: none;
      cursor: pointer;
    }

    /* Extras */
    .fs-extras {
      flex-shrink: 0;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 32px;
      padding-bottom: 4px;
    }

    .fs-extra-btn {
      background: none;
      border: none;
      color: rgba(255, 255, 255, 0.45);
      cursor: pointer;
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 4px;
      font-size: 11px;
      padding: 8px;
      border-radius: 8px;
      -webkit-tap-highlight-color: transparent;
      transition: color 0.15s;
    }

    .fs-extra-btn.active {
      color: var(--accent);
    }

    .queue-count {
      font-weight: 700;
    }
  }
</style>
