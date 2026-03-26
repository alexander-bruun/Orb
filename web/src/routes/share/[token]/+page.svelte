<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { share as shareApi, type RedeemShareResp } from '$lib/api/share';
  import { getApiBase } from '$lib/api/base';
  import type { Track } from '$lib/types';

  let state: 'loading' | 'ready' | 'used' | 'error' = 'loading';
  let data: RedeemShareResp | null = null;
  let errMsg = '';

  // Playback state
  let audio: HTMLAudioElement | null = null;
  let currentTrack: Track | null = null;
  let playing = false;
  let posMs = 0;
  let durMs = 0;
  let volume = 1;
  let streamSession = '';

  // Lyrics
  interface LyricLine { time_ms: number; text: string; }
  let lyricLines: LyricLine[] = [];
  let lyricTrackId = '';

  async function fetchLyrics(trackId: string) {
    if (!streamSession || lyricTrackId === trackId) return;
    lyricTrackId = trackId;
    try {
      const res = await fetch(`${getApiBase()}/share/lyrics/${streamSession}/${trackId}`);
      if (res.ok) lyricLines = await res.json();
      else lyricLines = [];
    } catch { lyricLines = []; }
  }

  onMount(async () => {
    const token = $page.params.token ?? '';
    try {
      data = await shareApi.redeem(token);
      streamSession = data.stream_session;

      if (data.type === 'track' && data.track) {
        currentTrack = data.track;
      } else if (data.type === 'album' && data.tracks?.length) {
        currentTrack = data.tracks[0];
      }
      state = 'ready';
    if (currentTrack) fetchLyrics(currentTrack.id);
    } catch (e: any) {
      if (e?.status === 410) {
        state = 'used';
      } else {
        state = 'error';
        errMsg = e?.message ?? 'Unknown error';
      }
    }
  });

  function coverUrl(albumId: string | null | undefined): string | null {
    if (!albumId) return null;
    return `${getApiBase()}/covers/${albumId}`;
  }

  function streamUrl(trackId: string): string {
    return shareApi.streamUrl(streamSession, trackId);
  }

  function playTrack(track: Track) {
    lyricLines = [];
    lyricTrackId = '';
    currentTrack = track;
    fetchLyrics(track.id);
    if (audio) {
      audio.src = streamUrl(track.id);
      audio.load();
      audio.play().catch(() => {});
    }
  }

  function togglePlay() {
    if (!audio) return;
    if (playing) {
      audio.pause();
    } else {
      if (!audio.src || audio.src === window.location.href) {
        audio.src = streamUrl(currentTrack!.id);
        audio.load();
      }
      audio.play().catch(() => {});
    }
  }

  function seek(e: MouseEvent) {
    if (!audio || durMs === 0) return;
    const bar = e.currentTarget as HTMLElement;
    const rect = bar.getBoundingClientRect();
    const pct = (e.clientX - rect.left) / rect.width;
    audio.currentTime = pct * (durMs / 1000);
  }

  function handleScrubberKeydown(e: KeyboardEvent) {
    if (!audio || durMs === 0) return;
    const durationSecs = durMs / 1000;
    const clamp = (value: number) => Math.max(0, Math.min(durationSecs, value));
    const step = 5;
    if (e.key === 'ArrowRight') {
      e.preventDefault();
      audio.currentTime = clamp(audio.currentTime + step);
    } else if (e.key === 'ArrowLeft') {
      e.preventDefault();
      audio.currentTime = clamp(audio.currentTime - step);
    } else if (e.key === 'Home') {
      e.preventDefault();
      audio.currentTime = 0;
    } else if (e.key === 'End') {
      e.preventDefault();
      audio.currentTime = durationSecs;
    }
  }

  function onTimeUpdate() {
    if (!audio) return;
    posMs = audio.currentTime * 1000;
    durMs = (audio.duration || 0) * 1000;
  }

  function onEnded() {
    playing = false;
    if (data?.type === 'album' && data.tracks && currentTrack) {
      const idx = data.tracks.findIndex((t) => t.id === currentTrack!.id);
      if (idx >= 0 && idx < data.tracks.length - 1) {
        playTrack(data.tracks[idx + 1]);
      }
    }
  }

  function onVolumeInput(e: Event) {
    volume = parseFloat((e.target as HTMLInputElement).value);
    if (audio) audio.volume = volume;
  }

  function fmt(ms: number): string {
    if (!ms || isNaN(ms)) return '0:00';
    const s = Math.floor(ms / 1000);
    const m = Math.floor(s / 60);
    return `${m}:${String(s % 60).padStart(2, '0')}`;
  }

  $: progressPct = durMs > 0 ? (posMs / durMs) * 100 : 0;
  $: tracks = data?.tracks ?? (data?.track ? [data.track] : []);

  $: activeLyric = (() => {
    if (!lyricLines.length) return '';
    let idx = -1;
    for (let i = 0; i < lyricLines.length; i++) {
      if (lyricLines[i].time_ms <= posMs) idx = i;
      else break;
    }
    return idx >= 0 ? lyricLines[idx].text : '';
  })();

  // Cover art: prefer album cover, fall back to track's album_id
  $: artUrl = data?.type === 'album' && data.album
    ? coverUrl(data.album.id)
    : data?.type === 'track' && data.track
      ? coverUrl(data.track.album_id)
      : null;

  $: pageTitle = data?.type === 'album' && data.album
    ? `${data.album.title} — Shared Album · Orb`
    : data?.type === 'track' && data.track
      ? `${data.track.title} — Shared Track · Orb`
      : 'Shared · Orb';
</script>

<svelte:head>
  <title>{pageTitle}</title>
</svelte:head>

<audio
  bind:this={audio}
  on:play={() => (playing = true)}
  on:pause={() => (playing = false)}
  on:ended={onEnded}
  on:timeupdate={onTimeUpdate}
  preload="none"
></audio>

<div class="page">
  {#if state === 'loading'}
    <div class="splash">
      <div class="spinner"></div>
      <p class="muted">Loading…</p>
    </div>

  {:else if state === 'used'}
    <div class="center card">
      <div class="icon-wrap warn">
        <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
          <circle cx="12" cy="12" r="10"/>
          <line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/>
        </svg>
      </div>
      <h2>Link Already Used</h2>
      <p>This share link is one-time use and has already been redeemed.</p>
    </div>

  {:else if state === 'error'}
    <div class="center card">
      <div class="icon-wrap warn">
        <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
          <circle cx="12" cy="12" r="10"/>
          <line x1="12" y1="8" x2="12" y2="12"/>
          <circle cx="12" cy="16" r="0.5" fill="currentColor"/>
        </svg>
      </div>
      <h2>Something went wrong</h2>
      <p class="muted">{errMsg}</p>
    </div>

  {:else if state === 'ready' && data}
    <div class="share-card">
      <!-- Cover art -->
      <div class="cover-wrap">
        {#if artUrl}
          <img class="cover" src={artUrl} alt="Cover art" />
        {:else}
          <div class="cover cover-placeholder">
            <svg width="52" height="52" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1" aria-hidden="true">
              <circle cx="12" cy="12" r="10"/>
              <circle cx="12" cy="12" r="3"/>
              <line x1="12" y1="2" x2="12" y2="9"/>
            </svg>
          </div>
        {/if}
      </div>

      <!-- Player panel -->
      <div class="player-panel">
        <!-- Meta -->
        <div class="meta">
          <span class="badge">{data.type === 'album' ? 'Album' : 'Track'}</span>
          {#if data.type === 'album' && data.album}
            <h1 class="title">{data.album.title}</h1>
            {#if data.album.artist_name}<p class="sub">{data.album.artist_name}{data.album.release_year ? ` · ${data.album.release_year}` : ''}</p>{/if}
          {:else if data.type === 'track' && data.track}
            <h1 class="title">{data.track.title}</h1>
            {#if data.track.artist_name}<p class="sub">{data.track.artist_name}</p>{/if}
          {/if}
        </div>

        <!-- Scrubber -->
        <div class="scrubber-row">
          <span class="time-label">{fmt(posMs)}</span>
          
          
          <div
            class="scrubber"
            on:click={seek}
            role="slider"
            tabindex="0"
            aria-label="Seek"
            aria-valuemin="0"
            aria-valuemax="100"
            aria-valuenow={progressPct}
            aria-valuetext={fmt(posMs)}
            on:keydown={handleScrubberKeydown}
          >
            <div class="scrubber-bg"></div>
            <div class="scrubber-fill" style="width:{progressPct}%"></div>
            <div class="scrubber-thumb" style="left:{progressPct}%"></div>
          </div>
          <span class="time-label">{fmt(durMs)}</span>
        </div>

        <!-- Controls row: play + volume -->
        <div class="controls-row">
          <button class="play-btn" on:click={togglePlay} aria-label={playing ? 'Pause' : 'Play'}>
            {#if playing}
              <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <rect x="6" y="4" width="4" height="16" rx="1"/>
                <rect x="14" y="4" width="4" height="16" rx="1"/>
              </svg>
            {:else}
              <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <polygon points="5,3 19,12 5,21"/>
              </svg>
            {/if}
          </button>

          <div class="vol-row">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
              {#if volume === 0}
                <polygon points="11,5 6,9 2,9 2,15 6,15 11,19" fill="currentColor" stroke="none"/>
                <line x1="23" y1="9" x2="17" y2="15"/>
                <line x1="17" y1="9" x2="23" y2="15"/>
              {:else if volume < 0.5}
                <polygon points="11,5 6,9 2,9 2,15 6,15 11,19" fill="currentColor" stroke="none"/>
                <path d="M15.54 8.46a5 5 0 0 1 0 7.07"/>
              {:else}
                <polygon points="11,5 6,9 2,9 2,15 6,15 11,19" fill="currentColor" stroke="none"/>
                <path d="M19.07 4.93a10 10 0 0 1 0 14.14"/>
                <path d="M15.54 8.46a5 5 0 0 1 0 7.07"/>
              {/if}
            </svg>
            <input
              class="vol-slider"
              type="range"
              min="0" max="1" step="0.02"
              value={volume}
              on:input={onVolumeInput}
              aria-label="Volume"
            />
          </div>
        </div>

        <!-- Active lyric line -->
        {#if activeLyric}
          <p class="lyric-line">{activeLyric}</p>
        {/if}

        <!-- Track list (album mode) -->
        {#if data.type === 'album' && tracks.length > 0}
          <div class="track-list">
            {#each tracks as track (track.id)}
              <button
                class="track-row"
                class:active={currentTrack?.id === track.id}
                on:click={() => playTrack(track)}
              >
                <span class="track-num">
                  {#if currentTrack?.id === track.id && playing}
                    <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                      <rect x="4" y="4" width="4" height="16" rx="1"/>
                      <rect x="16" y="4" width="4" height="16" rx="1"/>
                    </svg>
                  {:else}
                    {track.track_number ?? ''}
                  {/if}
                </span>
                <span class="track-title">{track.title}</span>
                <span class="track-dur">{fmt(track.duration_ms)}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    </div>

    <p class="branding">Shared via <strong>Orb</strong></p>
  {/if}
</div>

<style>
  :global(*, *::before, *::after) { box-sizing: border-box; }
  :global(body) {
    margin: 0;
    background: #0f0f13;
    color: #e2e2e8;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    min-height: 100dvh;
  }

  .page {
    min-height: 100dvh;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 32px 16px;
    gap: 20px;
  }

  /* Loading */
  .splash { display: flex; flex-direction: column; align-items: center; gap: 16px; }
  .spinner {
    width: 32px; height: 32px;
    border: 3px solid rgba(255,255,255,0.1);
    border-top-color: #7c6af7;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }

  /* Status cards */
  .center { text-align: center; max-width: 380px; width: 100%; }
  .muted { color: #888; }
  .card {
    background: #1a1a22;
    border: 1px solid #2a2a38;
    border-radius: 20px;
    padding: 40px 32px;
  }
  .card h2 { margin: 14px 0 8px; font-size: 1.15rem; }
  .card p  { margin: 0; font-size: 0.88rem; color: #888; }
  .icon-wrap { display: flex; justify-content: center; }
  .icon-wrap.warn { color: #888; }

  /* Main card */
  .share-card {
    display: flex;
    gap: 36px;
    align-items: flex-start;
    background: #1a1a22;
    border: 1px solid #2a2a38;
    border-radius: 20px;
    padding: 32px;
    max-width: 780px;
    width: 100%;
    box-shadow: 0 24px 60px rgba(0,0,0,0.5);
  }

  @media (max-width: 620px) {
    .share-card { flex-direction: column; align-items: center; padding: 24px 18px; gap: 24px; }
  }

  /* Cover */
  .cover-wrap { flex-shrink: 0; }
  .cover {
    width: 200px;
    height: 200px;
    border-radius: 12px;
    object-fit: cover;
    display: block;
    background: #252530;
    box-shadow: 0 8px 24px rgba(0,0,0,0.4);
  }
  .cover-placeholder {
    display: flex;
    align-items: center;
    justify-content: center;
    background: #252530;
    color: #555;
  }
  @media (max-width: 620px) {
    .cover { width: 160px; height: 160px; }
  }

  /* Player panel */
  .player-panel {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 18px;
  }

  .meta { display: flex; flex-direction: column; gap: 5px; }

  .badge {
    display: inline-block;
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: #7c6af7;
  }

  .title {
    margin: 0;
    font-size: 1.45rem;
    font-weight: 700;
    line-height: 1.2;
    color: #f0f0f5;
  }

  .sub {
    margin: 0;
    font-size: 0.9rem;
    color: #888;
  }

  /* Scrubber */
  .scrubber-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .time-label {
    font-size: 0.72rem;
    color: #666;
    min-width: 34px;
    font-variant-numeric: tabular-nums;
  }
  .time-label:last-child { text-align: right; }

  .scrubber {
    flex: 1;
    height: 20px;
    display: flex;
    align-items: center;
    position: relative;
    cursor: pointer;
  }
  .scrubber-bg {
    position: absolute;
    inset: 0;
    top: 50%;
    height: 3px;
    transform: translateY(-50%);
    background: #2e2e3e;
    border-radius: 2px;
  }
  .scrubber-fill {
    position: absolute;
    top: 50%;
    left: 0;
    height: 3px;
    transform: translateY(-50%);
    background: #7c6af7;
    border-radius: 2px;
    transition: width 0.15s linear;
  }
  .scrubber-thumb {
    position: absolute;
    top: 50%;
    width: 12px;
    height: 12px;
    background: #fff;
    border-radius: 50%;
    transform: translate(-50%, -50%);
    box-shadow: 0 1px 4px rgba(0,0,0,0.4);
    transition: left 0.15s linear;
  }
  .scrubber:hover .scrubber-fill { background: #9585ff; }
  .scrubber:hover .scrubber-thumb { transform: translate(-50%, -50%) scale(1.2); }

  /* Controls */
  .controls-row {
    display: flex;
    align-items: center;
    gap: 16px;
  }

  .play-btn {
    width: 46px;
    height: 46px;
    flex-shrink: 0;
    border-radius: 50%;
    background: #7c6af7;
    border: none;
    color: #fff;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: background 0.15s, transform 0.1s;
  }
  .play-btn:hover { background: #9585ff; }
  .play-btn:active { transform: scale(0.94); }

  /* Volume */
  .vol-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1;
    color: #666;
  }

  .vol-slider {
    flex: 1;
    -webkit-appearance: none;
    appearance: none;
    height: 3px;
    background: #2e2e3e;
    border-radius: 2px;
    outline: none;
    cursor: pointer;
  }
  .vol-slider::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: #fff;
    box-shadow: 0 1px 4px rgba(0,0,0,0.4);
    cursor: pointer;
  }
  .vol-slider::-moz-range-thumb {
    width: 12px;
    height: 12px;
    border: none;
    border-radius: 50%;
    background: #fff;
    box-shadow: 0 1px 4px rgba(0,0,0,0.4);
    cursor: pointer;
  }
  .vol-slider:hover { background: #3e3e52; }

  /* Track list */
  .track-list {
    display: flex;
    flex-direction: column;
    gap: 1px;
    max-height: 280px;
    overflow-y: auto;
    scrollbar-width: thin;
    scrollbar-color: #333 transparent;
    margin: 0 -6px;
  }

  .track-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 7px 10px;
    border-radius: 7px;
    background: none;
    border: none;
    color: #ccc;
    font-size: 0.84rem;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
    width: 100%;
  }
  .track-row:hover { background: #222230; color: #f0f0f5; }
  .track-row.active { color: #7c6af7; }

  .track-num {
    width: 22px;
    text-align: right;
    color: #555;
    font-size: 0.78rem;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: flex-end;
  }
  .track-row.active .track-num { color: #7c6af7; }

  .track-title {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .track-dur {
    color: #555;
    font-size: 0.76rem;
    flex-shrink: 0;
    font-variant-numeric: tabular-nums;
  }

  /* Lyrics */
  .lyric-line {
    margin: 0;
    font-size: 0.88rem;
    color: #9585ff;
    font-style: italic;
    text-align: center;
    min-height: 1.3em;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    animation: lyric-in 0.3s ease;
  }
  @keyframes lyric-in {
    from { opacity: 0; transform: translateY(4px); }
    to   { opacity: 1; transform: translateY(0); }
  }

  /* Branding */
  .branding {
    font-size: 0.78rem;
    color: #444;
    margin: 0;
  }
  .branding strong { color: #666; }
</style>
