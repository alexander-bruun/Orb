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
    smartShuffleEnabled,
    userQueue,
    queueModalOpen,
    togglePlayPause,
    seek,
    setVolume,
    next,
    previous,
    toggleRepeat,
    toggleShuffle,
    autoplayEnabled,
    MUSIC_SLEEP_PRESETS,
    musicSleepPreset,
    musicSleepMsRemaining,
    musicSleepFading,
    setMusicSleepTimer,
    clearMusicSleepTimer,
  } from '$lib/stores/player';

  $: progress = $durationMs > 0 ? ($positionMs / $durationMs) * 100 : 0;
  function onSeek(e: Event) {
    const pct = parseFloat((e.target as HTMLInputElement).value);
    seek(($durationMs / 1000) * (pct / 100));
  }
  import { library } from '$lib/api/library';
  import { writable, get } from 'svelte/store';
  import { expanded } from './coverExpandStore';
  import {
    lpRole,
    lpPanelOpen,
    lpParticipants,
    createAndConnect,
  } from '$lib/stores/social/listenParty';
  import { lyricsOpen, lyricsLines, lyricsLoading } from '$lib/stores/player/lyrics';
  import { visualizerStore } from '$lib/stores/player/visualizer';
  import Visualizer from '$lib/components/ui/Visualizer.svelte';
  import TrackWaveform from '$lib/components/ui/TrackWaveform.svelte';
  import { seekBarMode, visualizerButtonEnabled, bottomBarSecondary, listenAlongEnabled } from '$lib/stores/settings/theme';
  import { onMount } from 'svelte';
  import { waveformFailed } from '$lib/stores/player/waveformPeaks';

  let waveformWidth = 0;
  let seekBarWidth = 0;

  // Sine-wave seek bar constants
  const SEEK_WAVE_LEN = 12; // px per full cycle
  const SEEK_WAVE_AMP = 2;  // amplitude in px
  const THUMB_R = 6;        // half of 12 px thumb

  // Match the browser's thumb-centering formula: the thumb doesn't overflow the
  // track ends, so center = thumbR + progress/100 × (trackWidth − 2×thumbR).
  // This keeps our SVG playhead line exactly under the thumb's center.
  $: progressX = seekBarWidth > 2 * THUMB_R
    ? THUMB_R + (progress / 100) * (seekBarWidth - 2 * THUMB_R)
    : seekBarWidth * progress / 100;

  // Phase offset driven by rAF so the wave travels without shifting the
  // smoothstep envelope — the wave is always flat at x=0 and x=progressX.
  // waveAmp (0–1) fades the whole squiggle in/out when pausing/resuming.
  let wavePhase = 0;
  let waveAmp = 1;
  onMount(() => {
    let raf: number;
    let last = performance.now();
    function tick(now: number) {
      const dt = now - last;
      last = now;
      const playing = get(playbackState) === 'playing';
      // Exponential ease toward target amplitude (~200 ms time constant)
      waveAmp += ((playing ? 1 : 0) - waveAmp) * (1 - Math.exp(-dt / 200));
      if (playing) {
        wavePhase += (dt / 1500) * Math.PI * 2;
      }
      raf = requestAnimationFrame(tick);
    }
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  });

  // Sine-wave path from x=0 to x=progressX.  A smoothstep envelope drives
  // amplitude to zero over one wave period at each end (C¹-continuous with the
  // straight tails), matching the same technique used on the album-card ring.
  $: seekWavePath = (() => {
    if (progressX <= 1) return '';
    const y = 2;
    const W = progressX;
    const fade = SEEK_WAVE_LEN; // fade zone = one wave period at each end
    const steps = Math.ceil(W / 1.5);
    let d = `M 0 ${y}`;
    for (let i = 1; i <= steps; i++) {
      const x = (i / steps) * W;
      const t = Math.min(x / fade, (W - x) / fade, 1);
      const env = t * t * (3 - 2 * t); // smoothstep
      const wy = y + SEEK_WAVE_AMP * waveAmp * env * Math.sin((x / SEEK_WAVE_LEN) * Math.PI * 2 + wavePhase);
      d += ` L ${x.toFixed(1)} ${wy.toFixed(2)}`;
    }
    d += ` L ${W.toFixed(1)} ${y}`; // guarantee exact flat endpoint
    return d;
  })();
  import DesktopDevicePicker from './DesktopDevicePicker.svelte';

  // ── Sleep timer ───────────────────────────────────────────────────────────────
  let sleepMenuOpen = false;

  function formatSleepRemaining(ms: number): string {
    if (ms < 0) return 'EOT';
    const totalSecs = Math.ceil(ms / 1000);
    const m = Math.floor(totalSecs / 60);
    const s = totalSecs % 60;
    if (m > 0) return `${m}:${s.toString().padStart(2, '0')}`;
    return `0:${s.toString().padStart(2, '0')}`;
  }

  const currentAlbum = writable<{ id: string; title: string } | null>(null);
  import { getApiBase } from '$lib/api/base';

  $: {
    if ($currentTrack?.album_id) {
      library.album($currentTrack.album_id)
        .then(res => currentAlbum.set({ id: res.album.id, title: res.album.title }))
        .catch(() => currentAlbum.set(null));
    } else {
      currentAlbum.set(null);
    }
  }


  function onVolume(e: Event) {
    const input = e.target as HTMLInputElement;
    setVolume(parseFloat(input.value));
  }

  function toggleExpand() {
    expanded.update(v => !v);
  }

</script>

<svelte:window on:click={(e) => {
  if (sleepMenuOpen) {
    const wrap = (e.target as HTMLElement).closest?.('.sleep-timer-wrap');
    if (!wrap) sleepMenuOpen = false;
  }
}} />

<footer class="bottom-bar">
  <!-- Left section: fixed width matching sidebar — content truncates at edge -->
  <div class="info-section">
    {#if $currentTrack}
      {#if !$expanded}
        <!-- Small mode: cover with hover-reveal expand button -->
        <div class="cover-hover-wrap">
          {#if $currentTrack.album_id}
            <img src="{getApiBase()}/covers/{$currentTrack.album_id}"
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
        {#if $bottomBarSecondary === 'album'}
          {#if $currentAlbum}
            <div class="album-title">
              <a href="/library/albums/{$currentAlbum.id}" class="album-link">{$currentAlbum.title}</a>
            </div>
          {:else if $currentTrack.album_name}
            <div class="album-title">
              {#if $currentTrack.album_id}
                <a href="/library/albums/{$currentTrack.album_id}" class="album-link">{$currentTrack.album_name}</a>
              {:else}
                <span class="album-link">{$currentTrack.album_name}</span>
              {/if}
            </div>
          {/if}
        {:else if $currentTrack.artist_id}
          <div class="album-title">
            <a href="/artists/{$currentTrack.artist_id}" class="album-link">{$currentTrack.artist_name ?? $currentTrack.artist_id}</a>
          </div>
        {:else if $currentTrack.artist_name}
          <div class="album-title">
            <span class="album-link">{$currentTrack.artist_name}</span>
          </div>
        {/if}
        <div class="song-title-row">
          <span class="song-title">{$currentTrack.title}</span>
        </div>
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
        title={$shuffle && $smartShuffleEnabled ? 'Smart Shuffle on' : 'Shuffle'}
        style="position:relative"
      >
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/>
          <polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/>
          <line x1="4" y1="4" x2="9" y2="9"/>
        </svg>
        {#if $shuffle && $smartShuffleEnabled}
          <span class="smart-dot" aria-hidden="true"></span>
        {/if}
      </button>
      <button class="ctrl-btn" on:click={previous} aria-label="Previous">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><polygon points="19,4 9,12 19,20"/><rect x="5" y="4" width="2.5" height="16" rx="1"/></svg>
      </button>
      <button
        class="ctrl-btn play-btn"
        on:click={togglePlayPause}
        aria-label={$playbackState === 'playing' ? 'Pause' : 'Play'}
      >
        {#if $playbackState === 'playing'}
          <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="4" width="4" height="16" rx="1"/><rect x="14" y="4" width="4" height="16" rx="1"/></svg>
        {:else}
          <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor"><polygon points="5,3 19,12 5,21"/></svg>
        {/if}
      </button>
      <button class="ctrl-btn" on:click={next} aria-label="Next">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><polygon points="5,4 15,12 5,20"/><rect x="16" y="4" width="2.5" height="16" rx="1"/></svg>
      </button>
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
      <button
        class="ctrl-btn icon-btn"
        class:active={$autoplayEnabled}
        on:click={() => autoplayEnabled.update(v => !v)}
        aria-label="Autoplay"
        title={$autoplayEnabled ? 'Autoplay on' : 'Autoplay off'}
      >
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M2 12a10 10 0 1 0 10-10"/>
          <polyline points="12 8 12 12 14 14"/>
          <polyline points="2 8 2 2 8 2"/>
        </svg>
      </button>
    </div>

    <div class="seek-area">
      <span class="time">{$formattedPosition}</span>
      {#if $seekBarMode === 'waveform' && !$waveformFailed}
        <div class="waveform-wrap" bind:clientWidth={waveformWidth}>
          {#if waveformWidth > 0}
            <TrackWaveform width={waveformWidth} height={36} />
          {/if}
        </div>
      {:else}
        <div class="seek-bar-wrap" bind:clientWidth={seekBarWidth}>
          {#if seekBarWidth > 0}
            <svg class="seek-svg" width={seekBarWidth} height="4" overflow="visible">
              <!-- Unplayed: grey line only from the playhead onward -->
              <line x1={progressX} y1="2" x2={seekBarWidth} y2="2"
                    stroke="var(--bg-hover)" stroke-width="2" stroke-linecap="round" />
              <!-- Buffered region overlay (only in unplayed zone) -->
              {#if seekBarWidth * $bufferedPct / 100 > progressX}
                <line x1={progressX} y1="2" x2={seekBarWidth * $bufferedPct / 100} y2="2"
                      stroke="rgba(160,160,160,0.35)" stroke-width="2" stroke-linecap="round" />
              {/if}
              {#if $seekBarMode !== 'line' && progressX > 1}
                <!-- Squiggle: phase-animated sine-wave with smoothstep fade at both ends -->
                <path d={seekWavePath}
                      stroke="var(--accent)" stroke-width="2" fill="none" stroke-linecap="round" />
              {:else if progressX > 0}
                <!-- Line: plain flat accent bar for the played region -->
                <line x1="0" y1="2" x2={progressX} y2="2"
                      stroke="var(--accent)" stroke-width="2" stroke-linecap="round" />
              {/if}
              <!-- Playhead: vertical tick — hidden on hover (thumb takes over) -->
              {#if progressX > 0 && progressX < seekBarWidth}
                <line class="seek-playhead" x1={progressX} y1="-3" x2={progressX} y2="7"
                      stroke="var(--accent)" stroke-width="1.5" stroke-linecap="round" />
              {/if}
            </svg>
          {/if}
          <input type="range" min="0" max="100" step="0.1" value={progress}
            on:input={onSeek} class="seek-input" aria-label="Seek" />
        </div>
      {/if}
      <span class="time">{$formattedDuration}</span>
    </div>

    <div class="right-controls">
      <!-- Listen Along button -->
      {#if $listenAlongEnabled}
        {#if $lpRole === 'host'}
          <button
            class="ctrl-btn icon-btn party-btn"
            class:active={$lpPanelOpen}
            on:click={() => lpPanelOpen.update(v => !v)}
            title="Listen Along"
            aria-label="Listen Along panel"
          >
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <circle cx="9" cy="7" r="3"/><path d="M3 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2"/>
              <circle cx="18" cy="7" r="2.5"/><path d="M22 21v-1.5a3.5 3.5 0 0 0-3.5-3.5H17"/>
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
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <circle cx="9" cy="7" r="3"/><path d="M3 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2"/>
              <circle cx="18" cy="7" r="2.5"/><path d="M22 21v-1.5a3.5 3.5 0 0 0-3.5-3.5H17"/>
            </svg>
          </button>
        {/if}
      {/if}
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
      {#if $currentTrack}
        <button
          class="ctrl-btn icon-btn lyrics-btn"
          class:active={$lyricsOpen}
          class:has-lyrics={$lyricsLines.length > 0}
          on:click={() => lyricsOpen.update(v => !v)}
          aria-label="Toggle lyrics"
          title="Lyrics"
          disabled={$lyricsLoading}
        >
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M9 18V5l12-2v13"/>
            <circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>
          </svg>
        </button>
      {/if}

      <!-- Sound visualizer toggle -->
      {#if $visualizerButtonEnabled}
        <button
          class="ctrl-btn icon-btn viz-toggle-btn"
          class:active={$visualizerStore.visible}
          on:click={() => visualizerStore.toggle()}
          aria-label="{$visualizerStore.visible ? 'Hide' : 'Show'} sound visualizer"
          title="Sound visualizer"
          aria-pressed={$visualizerStore.visible}
        >
          <!-- Simple spectrum-bar icon -->
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <rect x="2"  y="14" width="4" height="8"  rx="1" fill="currentColor"/>
            <rect x="8"  y="8"  width="4" height="14" rx="1" fill="currentColor"/>
            <rect x="14" y="4"  width="4" height="18" rx="1" fill="currentColor"/>
            <rect x="20" y="10" width="2" height="12" rx="1" fill="currentColor"/>
          </svg>
        </button>
      {/if}

      <!-- Sleep timer -->
      <div class="sleep-timer-wrap">
        <button
          class="ctrl-btn icon-btn sleep-btn"
          class:active={$musicSleepPreset !== null}
          class:fading={$musicSleepFading}
          on:click={() => sleepMenuOpen = !sleepMenuOpen}
          aria-label="Sleep timer"
          title={$musicSleepPreset !== null ? `Sleep timer: ${formatSleepRemaining($musicSleepMsRemaining)}` : 'Sleep timer'}
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
          </svg>
          {#if $musicSleepPreset !== null && $musicSleepMsRemaining !== 0}
            <span class="sleep-badge">{formatSleepRemaining($musicSleepMsRemaining)}</span>
          {/if}
        </button>

        {#if sleepMenuOpen}
          <div class="sleep-menu" role="menu">
            {#each MUSIC_SLEEP_PRESETS as preset}
              <button
                class="sleep-item"
                class:selected={$musicSleepPreset === preset}
                role="menuitem"
                on:click={() => {
                  if ($musicSleepPreset === preset) {
                    clearMusicSleepTimer();
                  } else {
                    setMusicSleepTimer(preset);
                  }
                  sleepMenuOpen = false;
                }}
              >
                {preset === 'end_of_track' ? 'End of track' : `${preset} min`}
              </button>
            {/each}
            {#if $musicSleepPreset !== null}
              <button class="sleep-item sleep-cancel" role="menuitem" on:click={() => { clearMusicSleepTimer(); sleepMenuOpen = false; }}>
                Cancel timer
              </button>
            {/if}
          </div>
        {/if}
      </div>

      <DesktopDevicePicker />
    </div>
  </div>
</footer>

<!-- Visualizer widget lives outside the footer so it can float freely -->
<Visualizer />

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
    overflow: visible;
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
    overflow: visible;
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
  .song-title-row {
    display: flex;
    align-items: center;
    gap: 2px;
    min-width: 0;
  }
  .song-title {
    font-size: 0.9rem;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
    min-width: 0;
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
  /* "S" dot on shuffle button when smart shuffle is active */
  .smart-dot {
    position: absolute;
    top: 2px;
    right: 2px;
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: var(--accent);
    opacity: 0.85;
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

  .waveform-wrap {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
  }

  /* Plain seek bar (fallback when waveform is disabled) */
  .seek-bar-wrap {
    flex: 1;
    position: relative;
    height: 8px; /* extra height so wave bumps above have room */
    display: flex;
    align-items: center;
    min-width: 0;
    overflow: visible;
  }
  .seek-svg {
    position: absolute;
    left: 0;
    display: block;
    overflow: visible;
  }
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
  .seek-input::-webkit-slider-runnable-track { background: transparent; height: 4px; }
  .seek-input::-moz-range-track { background: transparent; height: 4px; border: none; }
  .seek-input::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 12px; height: 12px; border-radius: 50%;
    background: var(--accent); margin-top: -4px; cursor: pointer;
    opacity: 0; transition: opacity 0.15s;
  }
  .seek-input::-moz-range-thumb {
    width: 12px; height: 12px; border-radius: 50%;
    background: var(--accent); border: none; cursor: pointer;
    opacity: 0; transition: opacity 0.15s;
  }
  .seek-bar-wrap:hover .seek-input::-webkit-slider-thumb { opacity: 1; }
  .seek-bar-wrap:hover .seek-input::-moz-range-thumb { opacity: 1; }
  .seek-playhead { transition: opacity 0.15s; }
  .seek-bar-wrap:hover .seek-playhead { opacity: 0; }

  .right-controls { flex-shrink: 0; display: flex; align-items: center; gap: 12px; }

  /* Listen Along button */
  .party-btn { position: relative; padding: 6px; }
  .party-btn svg { overflow: hidden; display: block; }
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

  /* ── Sleep timer ─────────────────────────────────────────── */
  .sleep-timer-wrap {
    position: relative;
    display: inline-flex;
    align-items: center;
  }
  .sleep-btn {
    position: relative;
    padding: 6px;
  }
  .sleep-btn.fading {
    animation: sleep-pulse 1s ease-in-out infinite alternate;
  }
  @keyframes sleep-pulse {
    from { opacity: 0.5; }
    to   { opacity: 1; }
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
    min-width: 130px;
    box-shadow: 0 4px 16px rgba(0,0,0,0.25);
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
  .sleep-item:hover { background: var(--bg-hover); color: var(--text); }
  .sleep-item.selected { color: var(--accent); }
  .sleep-cancel {
    margin-top: 2px;
    border-top: 1px solid var(--border);
    border-radius: 0 0 5px 5px;
    padding-top: 8px;
  }

  /* ── Mobile: replaced entirely by MobilePlayer component ── */
  @media (max-width: 640px) {
    .bottom-bar {
      display: none;
    }
  }
</style>
