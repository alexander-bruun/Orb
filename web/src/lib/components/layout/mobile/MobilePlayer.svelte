<script lang="ts">
  import { onMount } from 'svelte';
  import {
    currentTrack,
    playbackState,
    positionMs,
    durationMs,
    userQueue,
    queueModalOpen,
    togglePlayPause,
    seek,
    next,
    previous,
    transferPlayback,
    formattedFormat,
  } from '$lib/stores/player';
  import { abFormattedFormat } from '$lib/stores/player/audiobookPlayer';
  import { activePlayer } from '$lib/stores/player/engine';
  import { library } from '$lib/api/library';
  import { favorites } from '$lib/stores/library/favorites';
  import { get, writable } from 'svelte/store';
  import { getApiBase } from '$lib/api/base';
  import { lyricsLines, lyricsLoading, activeLyricIndex } from '$lib/stores/player/lyrics';
  import { goto } from '$app/navigation';
  import {
    castState,
    castDeviceName,
    initCastSdk,
    startCast,
    stopCast,
    remotePlaybackSupported,
    promptRemotePlayback,
  } from '$lib/stores/player/casting';
  import {
    lpRole,
    lpPanelOpen,
    lpParticipants,
    lpSessionId,
    createAndConnect,
  } from '$lib/stores/social/listenParty';
  import StarRating from '$lib/components/ui/StarRating.svelte';
  import MobilePlaybackControls from './MobilePlaybackControls.svelte';
  import MobileProgressBar from './MobileProgressBar.svelte';
  import MobileVolumeSlider from './MobileVolumeSlider.svelte';
  import MobileDevicePicker from './MobileDevicePicker.svelte';
  import { nativePlatform } from '$lib/utils/platform';
  import { invoke } from '@tauri-apps/api/core';

  const currentAlbum = writable<{ id: string; title: string; artist?: string } | null>(null);

  $: isFavorite = $currentTrack ? $favorites.has($currentTrack.id) : false;

  async function toggleFavorite() {
    if (!$currentTrack) return;
    await favorites.toggle($currentTrack.id, $currentTrack);
  }

  let playerOpen = false;
  let playerHistoryPushed = false;

  onMount(() => {
    function handlePopState() {
      if (playerOpen) {
        // Back gesture/button while player is open — close without re-popping
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
  let rawDelta = 0;      // raw touch offset
  let swipeDelta = 0;    // rubber-banded visual offset
  let dismissing = false;

  // Mini-player horizontal swipe → next / previous
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
      const goNext = miniDeltaX > 0;
      miniDeltaX = 0;
      miniSwipeAxis = null;
      if (goNext) next(); else previous();
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
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onMiniClick();
    }
  }

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
        history.back(); // pops the state we pushed; popstate handler is a no-op since playerOpen is already false
      }
    }
  }

  // Swipe down gesture on full screen player
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
    // Rubber-band resistance: starts ~1:1, progressively heavier
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

  function handleFullscreenKeyDown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      closePlayer();
    }
  }

  function seekToLyric(timeMs: number) {
    seek(timeMs / 1000);
  }

  function goToAlbum(e?: MouseEvent) {
    e?.stopPropagation();
    if ($currentAlbum) {
      closePlayer(true);
      goto(`/library/albums/${$currentAlbum.id}`);
    }
  }

  function goToArtist(e?: MouseEvent) {
    e?.stopPropagation();
    if ($currentTrack?.artist_id) {
      closePlayer(true);
      goto(`/artists/${$currentTrack.artist_id}`);
    }
  }

  // ── Device transfer ────────────────────────────────────────────────────────
  let devicePickerOpen = false;

  // Initialise Cast SDK so it's ready when the user opens the picker.
  initCastSdk();

  async function handleCastToggle() {
    // On Android native, open the system Bluetooth / connected devices settings.
    if (nativePlatform() === 'android') {
      try { await invoke('open_bluetooth_settings'); } catch { /* ignore */ }
      return;
    }
    if ($castState === 'connected') {
      stopCast();
    } else if ($castState === 'idle') {
      try { await startCast(); } catch { /* user cancelled */ }
    } else if ($castState === 'unavailable') {
      // Try the Remote Playback API first (mobile browsers).
      if (remotePlaybackSupported) {
        try {
          await promptRemotePlayback();
          return;
        } catch {
          // User cancelled or no devices found — fall through.
        }
        return;
      }
      // Retry Cast SDK init — may succeed if conditions changed.
      initCastSdk();
      // Give it a moment then check again.
      await new Promise(r => setTimeout(r, 1500));
      if (get(castState) === 'idle') {
        try { await startCast(); } catch { /* user cancelled */ }
      }
    }
  }

  async function transferToDevice(targetId: string) {
    devicePickerOpen = false;
    await transferPlayback(targetId);
  }
</script>

<!-- ── Mini player (shown above bottom nav) ──────────────────────────────── -->
{#if $currentTrack}
  
  
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
      style="transform: translateX({miniDeltaX * 0.42}px) rotate({miniDeltaX * 0.015}deg);
             transition: {miniIsSwiping ? 'none' : 'transform 0.4s cubic-bezier(0.22, 1, 0.36, 1)'};"
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
          class="mini-btn mini-btn--fav"
          class:active={isFavorite}
          on:click|stopPropagation={toggleFavorite}
          aria-label={isFavorite ? 'Remove from favorites' : 'Add to favorites'}
          aria-pressed={isFavorite}
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill={isFavorite ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/>
          </svg>
        </button>
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
  </section>
{/if}

<!-- ── Full-screen player ─────────────────────────────────────────────────── -->
{#if playerOpen && $currentTrack}
  
  
    <div
      class="fullscreen-player"
      role="dialog"
      aria-label="Full-screen player"
      tabindex="-1"
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
      on:keydown={handleFullscreenKeyDown}
    >
    <!-- Blurred album art background (parallax: moves slower than content) -->
    {#if $currentTrack.album_id}
      <div
        class="fs-bg"
        style="
          background-image: url('{getApiBase()}/covers/{$currentTrack.album_id}');
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
        {#if $lyricsLines.length > 0}
          
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
        <button class="fs-close-btn" on:click={() => closePlayer()} aria-label="Close player">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true">
            <polyline points="6 9 12 15 18 9"/>
          </svg>
        </button>
      </div>

      {#if showLyrics}
        <!-- ── Lyrics panel ───────────────────────────────────────── -->
        
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
              
              
              <button
                type="button"
                class="lyric-line"
                class:lyric-active={i === $activeLyricIndex}
                class:lyric-past={i < $activeLyricIndex}
                data-lyric-idx={i}
                on:click|stopPropagation={() => seekToLyric(line.time_ms)}
              >{line.text || '♩'}</button>
            {/each}
            <div class="lyric-spacer-bottom"></div>
          {/if}
        </div>

        <!-- Compact track name above seek bar (lyrics mode) -->
        <div class="fs-info fs-info--compact">
          <div class="fs-title">{$currentTrack.title}</div>
        </div>
      {:else}
        <!-- Bitrate / format badge -->
        {#if $activePlayer === 'audiobook' ? $abFormattedFormat : $formattedFormat}
          <div class="fs-format-badge">{$activePlayer === 'audiobook' ? $abFormattedFormat : $formattedFormat}</div>
        {/if}

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

        <!-- Active lyric preview (shown when lyrics panel is closed) -->
        
        
        <button
          type="button"
          class="fs-lyric-slot"
          class:fs-lyric-slot--active={$lyricsLines.length > 0 && $activeLyricIndex >= 0}
          aria-label="Show lyrics"
          on:click|stopPropagation={() => { if ($lyricsLines.length > 0 && $activeLyricIndex >= 0) showLyrics = true; }}
        >
          {#if $lyricsLines.length > 0 && $activeLyricIndex >= 0}
            <span class="fs-lyric-preview">
              {$lyricsLines[$activeLyricIndex]?.text ?? ''}
            </span>
          {/if}
        </button>

        <!-- Track info -->
        <div class="fs-info">
          <div class="fs-info-text">
            <div class="fs-title">{$currentTrack.title}</div>
            <div class="fs-sub">
              {#if $currentTrack.artist_name}
                
              <span
                class="fs-artist"
                on:click={goToArtist}
                on:keydown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    goToArtist();
                  }
                }}
                role="link"
                tabindex="0"
              >{$currentTrack.artist_name}</span>
              {/if}
              {#if $currentAlbum}
                <span class="fs-sep">·</span>
                
                
            <span
              class="fs-album"
              on:click={goToAlbum}
              on:keydown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  goToAlbum();
                }
              }}
              role="link"
              tabindex="0"
            >
                  {$currentAlbum.title}
                </span>
              {/if}
            </div>
          </div>
          <div class="fs-actions">
            <StarRating trackId={$currentTrack.id} size={22} />
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
        </div>
      {/if}

      <!-- Seek bar -->
      <MobileProgressBar />

      <!-- Main controls -->
      <MobilePlaybackControls />

      <!-- Volume row -->
      <MobileVolumeSlider />

      <!-- Extras row: queue + listen along + device transfer -->
      <div class="fs-extras">
        {#if $lpRole === 'host'}
          <button
            class="fs-extra-btn"
            class:active={$lpPanelOpen}
            on:click={() => { closePlayer(); lpPanelOpen.update(v => !v); }}
            aria-label="Listen Along"
            title="Listen Along"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <circle cx="9" cy="7" r="3"/><path d="M3 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2"/>
              <circle cx="18" cy="7" r="2.5"/><path d="M22 21v-1.5a3.5 3.5 0 0 0-3.5-3.5H17"/>
            </svg>
            <span>Party{#if $lpParticipants.length > 0}&nbsp;<span class="party-count">{$lpParticipants.length}</span>{/if}</span>
          </button>
        {:else if $lpRole === null}
          <button
            class="fs-extra-btn"
            on:click={createAndConnect}
            aria-label="Start Listen Along"
            title="Start Listen Along"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <circle cx="9" cy="7" r="3"/><path d="M3 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2"/>
              <circle cx="18" cy="7" r="2.5"/><path d="M22 21v-1.5a3.5 3.5 0 0 0-3.5-3.5H17"/>
            </svg>
            <span>Party</span>
          </button>
        {/if}

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

        <button
          class="fs-extra-btn"
          class:active={$castState === 'connected'}
          on:click={handleCastToggle}
          disabled={$castState === 'connecting'}
          aria-label={$castState === 'connected' ? 'Stop casting' : 'Cast to device'}
          title={$castState === 'connected' ? `Casting to ${$castDeviceName}` : 'Cast'}
        >
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M2 8.5V6a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-6"/>
            <path d="M2 15a7 7 0 0 1 7 7"/>
            <path d="M2 15a3 3 0 0 1 3 3"/>
            <line x1="2" y1="22" x2="2.01" y2="22"/>
          </svg>
          {#if $castState === 'connected'}
            <span class="fs-cast-dot"></span>
          {/if}
          <span>{$castState === 'connected' ? $castDeviceName : 'Cast'}</span>
        </button>

        <MobileDevicePicker
          bind:open={devicePickerOpen}
          onCastToggle={handleCastToggle}
          onTransfer={transferToDevice}
        />
      </div>
    </div>
  </div>
{/if}

<style>
  /* ── Mini player ─────────────────────────────────────────────────────────── */
  .mini-player-wrap {
    display: contents;
  }
  .mini-player {
    display: none; /* desktop: hidden */
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

    .mini-btn--fav {
      color: var(--text-muted);
      transition: color 0.15s, background 0.1s, transform 0.1s;
    }

    .mini-btn--fav.active {
      color: #e85050;
    }

    .mini-btn--fav:active {
      transform: scale(0.85);
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

    /* Format badge */
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

    /* Fixed-height slot between cover and track info — always reserves space */
    .fs-lyric-slot {
      flex-shrink: 0;
      height: 3.2em;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 0 20px;
      border: none;
      background: none;
    }

    .fs-lyric-slot--active {
      cursor: pointer;
    }

    .fs-lyric-slot:focus-visible {
      outline: 1px solid var(--accent);
      border-radius: 6px;
    }

    /* Active lyric preview — centered inside the slot */
    .fs-lyric-preview {
      display: -webkit-box;
      -webkit-line-clamp: 2;
      line-clamp: 2;
      -webkit-box-orient: vertical;
      overflow: hidden;
      text-align: center;
      font-size: 0.9rem;
      font-weight: 500;
      color: rgba(255, 255, 255, 0.7);
      line-height: 1.4;
      transition: color 0.25s ease;
    }

    .fs-lyric-slot--active:active .fs-lyric-preview {
      color: rgba(255, 255, 255, 0.95);
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
      transition: color 0.35s ease, font-size 0.2s ease, opacity 0.1s ease;
      cursor: pointer;
      word-break: break-word;
      -webkit-tap-highlight-color: transparent;
    }

    .lyric-line:active {
      opacity: 0.6;
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

    .fs-actions {
      flex-shrink: 0;
      display: flex;
      align-items: center;
      gap: 4px;
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
      position: relative;
      -webkit-tap-highlight-color: transparent;
      transition: color 0.15s;
    }

    .fs-extra-btn.active {
      color: var(--accent);
    }

    .party-count {
      font-weight: 700;
    }

    .queue-count {
      font-weight: 700;
    }

  }
</style>
