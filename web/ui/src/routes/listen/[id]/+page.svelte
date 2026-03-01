<script lang="ts">
  import { page } from '$app/stores';
  import { onMount, onDestroy } from 'svelte';
  import { listenPartyApi } from '$lib/api/listenParty';
  import type { SessionInfo } from '$lib/api/listenParty';
  import {
    connectAsGuest,
    leaveSession,
    setGuestVolume,
    lpParticipants,
    lpGuestTrack,
    lpGuestPositionMs,
    lpGuestDurationMs,
    lpGuestPlaying,
    lpKicked,
    lpSessionEnded,
    lpGuestParticipantId,
    lpGuestToken,
  } from '$lib/stores/listenParty';

  import { getApiBase } from '$lib/api/base';
  const sessionId: string = $page.params.id ?? '';

  // Page phases: 'loading' | 'join' | 'playing' | 'kicked' | 'ended' | 'error'
  let phase = $state<'loading' | 'join' | 'playing' | 'kicked' | 'ended' | 'error'>('loading');
  let session = $state<SessionInfo | null>(null);
  let errorMsg = $state('');
  let nickname = $state('');
  let nicknameError = $state('');
  let joining = $state(false);
  let volume = $state(1);

  // Lyrics state
  type LyricLine = { time_ms: number; text: string };
  let lyricsLines = $state<LyricLine[]>([]);
  let lyricsOpen = $state(true);
  let lyricsTrackId = $state('');

  // Active lyric index: last line where time_ms <= current position
  let activeLyricIndex = $derived.by(() => {
    const pos = $lpGuestPositionMs;
    let idx = -1;
    for (let i = 0; i < lyricsLines.length; i++) {
      if (lyricsLines[i].time_ms <= pos) idx = i;
      else break;
    }
    return idx;
  });

  // Derived progress percentage.
  let progress = $derived(
    $lpGuestDurationMs > 0 ? ($lpGuestPositionMs / $lpGuestDurationMs) * 100 : 0
  );

  function formatTime(ms: number): string {
    const s = Math.floor(ms / 1000);
    const m = Math.floor(s / 60);
    return `${m}:${(s % 60).toString().padStart(2, '0')}`;
  }

  onMount(async () => {
    try {
      session = await listenPartyApi.getSession(sessionId);
      phase = 'join';
    } catch {
      phase = 'error';
      errorMsg = 'This listen-along session does not exist or has ended.';
    }
  });

  // React to kicked / ended states from the store.
  $effect(() => { if ($lpKicked) phase = 'kicked'; });
  $effect(() => { if ($lpSessionEnded) phase = 'ended'; });

  // Fetch lyrics when track changes
  $effect(() => {
    const track = $lpGuestTrack;
    const token = $lpGuestToken;
    if (!track?.id || !token || track.id === lyricsTrackId) return;
    lyricsTrackId = track.id;
    lyricsLines = [];
    fetch(`${getApiBase()}/listen/${sessionId}/lyrics/${track.id}?guest_token=${encodeURIComponent(token)}`)
      .then(r => r.ok ? r.json() : [])
      .then((lines: LyricLine[]) => { lyricsLines = lines ?? []; })
      .catch(() => { lyricsLines = []; });
  });

  // ── Smooth scroll (identical to LyricsModal approach) ──
  let lyricsListEl: HTMLDivElement | undefined = $state();
  let targetScrollTop = 0;
  let currentScrollTop = 0;
  let rafId: number | null = null;
  let userScrolling = false;
  let userScrollTimer: ReturnType<typeof setTimeout> | null = null;
  let prevLyricIdx = -1;

  const LERP_SPEED = 0.08;

  function scrollTick() {
    if (!lyricsListEl || !lyricsOpen) { rafId = null; return; }
    if (!userScrolling) {
      const diff = targetScrollTop - currentScrollTop;
      if (Math.abs(diff) > 0.5) {
        currentScrollTop += diff * LERP_SPEED;
        lyricsListEl.scrollTop = Math.round(currentScrollTop);
      } else {
        currentScrollTop = targetScrollTop;
        lyricsListEl.scrollTop = targetScrollTop;
      }
    } else {
      currentScrollTop = lyricsListEl.scrollTop;
    }
    rafId = requestAnimationFrame(scrollTick);
  }

  function startScrollLoop() {
    if (rafId === null) {
      currentScrollTop = lyricsListEl?.scrollTop ?? 0;
      rafId = requestAnimationFrame(scrollTick);
    }
  }

  function stopScrollLoop() {
    if (rafId !== null) { cancelAnimationFrame(rafId); rafId = null; }
  }

  function onLyricsWheel() {
    userScrolling = true;
    if (userScrollTimer) clearTimeout(userScrollTimer);
    userScrollTimer = setTimeout(() => {
      userScrolling = false;
      currentScrollTop = lyricsListEl?.scrollTop ?? 0;
    }, 3000);
  }

  function updateTargetScroll(idx: number) {
    if (!lyricsListEl || idx < 0) return;
    const el = lyricsListEl.querySelector<HTMLElement>(`[data-idx="${idx}"]`);
    if (!el) return;
    const containerH = lyricsListEl.clientHeight;
    // Use getBoundingClientRect for accurate position relative to scroll container
    const containerRect = lyricsListEl.getBoundingClientRect();
    const elRect = el.getBoundingClientRect();
    const elTopInContainer = elRect.top - containerRect.top + lyricsListEl.scrollTop;
    targetScrollTop = Math.max(0, elTopInContainer - containerH / 2 + el.offsetHeight / 2);
  }

  // Re-target when active line changes
  $effect(() => {
    const idx = activeLyricIndex;
    if (idx !== prevLyricIdx && lyricsOpen && lyricsListEl) {
      prevLyricIdx = idx;
      userScrolling = false;
      if (userScrollTimer) clearTimeout(userScrollTimer);
      updateTargetScroll(idx);
    }
  });

  // Start/stop loop when lyrics section opens/closes or mounts
  $effect(() => {
    if (lyricsOpen && lyricsListEl) {
      currentScrollTop = lyricsListEl.scrollTop;
      updateTargetScroll(activeLyricIndex);
      startScrollLoop();
    } else {
      stopScrollLoop();
    }
  });

  // Reset on track change
  $effect(() => {
    // eslint-disable-next-line @typescript-eslint/no-unused-expressions
    lyricsTrackId; // track dependency
    prevLyricIdx = -1;
    targetScrollTop = 0;
    currentScrollTop = 0;
    if (lyricsListEl) lyricsListEl.scrollTop = 0;
  });

  // Next-line fade interpolation (foo_openlyrics approach)
  let nextLineFade = $derived.by(() => {
    const lines = lyricsLines;
    const pos = $lpGuestPositionMs;
    const idx = activeLyricIndex;
    if (idx >= 0 && idx < lines.length - 1) {
      const nextTime = lines[idx + 1].time_ms;
      const fadeStart = Math.max(lines[idx].time_ms, nextTime - 600);
      if (pos >= fadeStart) return Math.min(1, (pos - fadeStart) / (nextTime - fadeStart));
    }
    return 0;
  });

  async function join() {
    const name = nickname.trim();
    if (!name) { nicknameError = 'Please enter a nickname.'; return; }
    if (name.length > 32) { nicknameError = 'Nickname must be 32 characters or fewer.'; return; }
    nicknameError = '';
    joining = true;
    try {
      await connectAsGuest(sessionId, name);
      phase = 'playing';
    } catch (e: unknown) {
      const err = e instanceof Error ? e.message : 'Could not connect.';
      nicknameError = err;
    } finally {
      joining = false;
    }
  }

  function onVolumeChange(e: Event) {
    volume = parseFloat((e.target as HTMLInputElement).value);
    setGuestVolume(volume);
  }

  onDestroy(() => {
    stopScrollLoop();
    if (userScrollTimer) clearTimeout(userScrollTimer);
    leaveSession();
  });
</script>

<svelte:head>
  <title>Listen Along – Orb</title>
</svelte:head>

<div class="guest-shell">
  {#if phase === 'loading'}
    <div class="center-card">
      <p class="muted">Loading session…</p>
    </div>

  {:else if phase === 'error'}
    <div class="center-card">
      <div class="orb-logo">Orb</div>
      <p class="error-msg">{errorMsg}</p>
    </div>

  {:else if phase === 'join'}
    <div class="center-card join-card">
      <div class="orb-logo">Orb</div>
      <h1 class="join-heading">You're invited to listen along</h1>
      {#if session}
        <p class="host-label">with <strong>{session.host_name}</strong></p>
      {/if}
      <div class="nickname-field">
        <label for="nickname-input" class="field-label">Your nickname</label>
        <input
          id="nickname-input"
          type="text"
          class="nickname-input"
          class:invalid={!!nicknameError}
          bind:value={nickname}
          placeholder="e.g. Alice"
          maxlength="32"
          onkeydown={(e) => e.key === 'Enter' && join()}
          disabled={joining}
          autofocus
        />
        {#if nicknameError}
          <p class="field-error">{nicknameError}</p>
        {/if}
      </div>
      <button class="join-btn" onclick={join} disabled={joining}>
        {joining ? 'Joining…' : 'Join'}
      </button>
    </div>

  {:else if phase === 'playing'}
    <div class="player-layout">
      <!-- Track cover with overlays -->
      <div class="cover-area">
        {#if $lpGuestTrack?.album_id && $lpGuestToken}
          <img
            class="cover-art"
            src="{getApiBase()}/listen/{sessionId}/cover/{$lpGuestTrack.album_id}?guest_token={encodeURIComponent($lpGuestToken)}"
            alt="Album art"
          />
        {:else}
          <div class="cover-placeholder">
            <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round" opacity="0.3">
              <circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="3"/>
            </svg>
          </div>
        {/if}
        <!-- Playback state overlay — top left -->
        <div class="cover-overlay-tl">
          {#if $lpGuestPlaying}
            <span class="overlay-badge playing">
              <span class="status-dot green"></span> Playing
            </span>
          {:else}
            <span class="overlay-badge paused">
              <span class="status-dot red"></span> Paused
            </span>
          {/if}
        </div>
        <!-- Format badge overlay — top right -->
        {#if $lpGuestTrack}
          {@const bd = $lpGuestTrack.bit_depth ? `${$lpGuestTrack.bit_depth}bit` : ''}
          {@const sr = `${($lpGuestTrack.sample_rate / 1000).toFixed(1)}kHz`}
          <div class="cover-overlay-tr">
            <span class="format-badge">{[bd, sr].filter(Boolean).join(' · ')}</span>
          </div>
        {/if}
      </div>

      <!-- Track info -->
      <div class="track-info">
        <div class="track-title">{$lpGuestTrack?.title ?? '—'}</div>
        <div class="track-artist">{$lpGuestTrack?.artist_name ?? ''}</div>
      </div>

      <!-- Progress bar (read-only) -->
      <div class="progress-area">
        <span class="time">{formatTime($lpGuestPositionMs)}</span>
        <div class="progress-track">
          <div class="progress-fill" style="width:{progress}%"></div>
        </div>
        <span class="time">{formatTime($lpGuestDurationMs)}</span>
      </div>

      <!-- Volume control (below progress) -->
      <div class="volume-row">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>
          <path d="M15.54 8.46a5 5 0 0 1 0 7.07"/>
        </svg>
        <input
          type="range"
          min="0"
          max="1"
          step="0.01"
          value={volume}
          oninput={onVolumeChange}
          class="volume-slider"
          aria-label="Volume"
        />
      </div>

      <!-- Lyrics -->
      {#if lyricsLines.length > 0}
        <div class="lyrics-section">
          <button class="lyrics-toggle" onclick={() => lyricsOpen = !lyricsOpen}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>
            </svg>
            Lyrics
            <span class="chevron" class:open={lyricsOpen}>{@html '&#9662;'}</span>
          </button>
          {#if lyricsOpen}
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div class="lyrics-list" bind:this={lyricsListEl} onwheel={onLyricsWheel}>
              {#each lyricsLines as line, i (line.time_ms + '-' + i)}
                <div
                  class="lyric-line"
                  class:active={i === activeLyricIndex}
                  class:past={i < activeLyricIndex}
                  class:next={i === activeLyricIndex + 1}
                  style={i === activeLyricIndex + 1 ? `opacity: ${0.55 + 0.45 * nextLineFade}` : ''}
                  data-idx={i}
                >
                  {line.text || '\u00A0'}
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/if}

      <!-- Participants -->
      <div class="participants-area">
        <p class="participants-heading">Listening along</p>
        <ul class="participants-list">
          <!-- Host entry -->
          {#if session?.host_name}
            <li class="participant-item host">
              <span class="avatar">{session.host_name[0]?.toUpperCase() ?? '?'}</span>
              <span class="pname">{session.host_name} <em>(host)</em></span>
            </li>
          {/if}
          <!-- Self entry -->
          <li class="participant-item self">
            <span class="avatar">{nickname[0]?.toUpperCase() ?? '?'}</span>
            <span class="pname">{nickname} <em>(you)</em></span>
          </li>
          <!-- Other guests — exclude own entry which the server also broadcasts -->
          {#each $lpParticipants.filter(p => p.id !== $lpGuestParticipantId) as p (p.id)}
            <li class="participant-item">
              <span class="avatar">{p.nickname[0].toUpperCase()}</span>
              <span class="pname">{p.nickname}</span>
            </li>
          {/each}
        </ul>
      </div>
    </div>

  {:else if phase === 'kicked'}
    <div class="center-card">
      <div class="orb-logo">Orb</div>
      <h2 class="status-heading">You've been removed</h2>
      <p class="muted">The host has removed you from this listen-along session.</p>
    </div>

  {:else if phase === 'ended'}
    <div class="center-card">
      <div class="orb-logo">Orb</div>
      <h2 class="status-heading">Session ended</h2>
      <p class="muted">The host has ended this listen-along session.</p>
    </div>
  {/if}
</div>

<style>
  /* Full-screen isolated layout — no sidebar / topbar / bottombar from the shell */
  .guest-shell {
    min-height: 100dvh;
    background: var(--bg, #111);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
    box-sizing: border-box;
  }

  /* Centered card for join / error / status screens */
  .center-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
    text-align: center;
    max-width: 380px;
    width: 100%;
  }

  .orb-logo {
    font-size: 1.6rem;
    font-weight: 800;
    color: var(--accent, #7c3aed);
    letter-spacing: -0.04em;
  }

  .join-card { gap: 20px; }

  .join-heading {
    font-size: 1.3rem;
    font-weight: 700;
    color: var(--text, #fff);
    margin: 0;
  }

  .host-label {
    font-size: 0.9rem;
    color: var(--text-muted, #888);
    margin: 0;
  }

  .nickname-field {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 6px;
    text-align: left;
  }

  .field-label {
    font-size: 0.8rem;
    color: var(--text-muted, #888);
    font-weight: 500;
  }

  .nickname-input {
    width: 100%;
    box-sizing: border-box;
    background: var(--bg-elevated, #1e1e1e);
    border: 1px solid var(--border, #333);
    border-radius: 8px;
    color: var(--text, #fff);
    font-size: 1rem;
    padding: 10px 14px;
    outline: none;
    transition: border-color 0.15s;
  }
  .nickname-input:focus { border-color: var(--accent, #7c3aed); }
  .nickname-input.invalid { border-color: #ef4444; }

  .field-error {
    font-size: 0.78rem;
    color: #ef4444;
    margin: 0;
  }

  .join-btn {
    width: 100%;
    padding: 12px;
    background: var(--accent, #7c3aed);
    border: none;
    border-radius: 8px;
    color: #fff;
    font-size: 1rem;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 0.15s;
  }
  .join-btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .join-btn:not(:disabled):hover { opacity: 0.88; }

  .error-msg, .muted { font-size: 0.9rem; color: var(--text-muted, #888); margin: 0; }
  .status-heading { font-size: 1.2rem; font-weight: 700; color: var(--text, #fff); margin: 0; }

  /* Player layout */
  .player-layout {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 20px;
    max-width: 420px;
    width: 100%;
    max-height: 100%;
    overflow-y: auto;
    padding: 8px 0;
  }

  .cover-area {
    position: relative;
    width: clamp(200px, 50vw, 300px);
    aspect-ratio: 1;
    border-radius: 12px;
    overflow: hidden;
    box-shadow: 0 8px 32px rgba(0,0,0,0.4);
  }
  .cover-art {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
  .cover-placeholder {
    width: 100%;
    height: 100%;
    background: var(--bg-elevated, #1e1e1e);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  /* Overlays on cover art */
  .cover-overlay-tl, .cover-overlay-tr {
    position: absolute;
    top: 8px;
    z-index: 2;
  }
  .cover-overlay-tl { left: 8px; }
  .cover-overlay-tr { right: 8px; }

  .overlay-badge {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 0.7rem;
    font-weight: 600;
    padding: 4px 8px;
    border-radius: 6px;
    background: rgba(0, 0, 0, 0.6);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
  }
  .overlay-badge.playing { color: #22c55e; }
  .overlay-badge.paused  { color: #ef4444; }

  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex-shrink: 0;
  }
  .status-dot.green {
    background: #22c55e;
    animation: pulse 1.2s ease-in-out infinite;
  }
  .status-dot.red {
    background: #ef4444;
    animation: pulse 1.2s ease-in-out infinite;
  }

  .format-badge {
    font-family: 'DM Mono', monospace;
    font-size: 9px;
    letter-spacing: 0.08em;
    color: #fff;
    background: rgba(0, 0, 0, 0.6);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border-radius: 6px;
    padding: 4px 8px;
    white-space: nowrap;
  }

  .track-info { text-align: center; width: 100%; display: flex; flex-direction: column; align-items: center; gap: 4px; }
  .track-title {
    font-size: 1.2rem;
    font-weight: 700;
    color: var(--text, #fff);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .track-artist {
    font-size: 0.9rem;
    color: var(--text-muted, #888);
    margin-top: 4px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .progress-area {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .time { font-size: 0.75rem; color: var(--text-muted, #888); width: 36px; flex-shrink: 0; text-align: center; }
  .progress-track {
    flex: 1;
    height: 4px;
    background: var(--bg-hover, #333);
    border-radius: 2px;
    overflow: hidden;
  }
  .progress-fill {
    height: 100%;
    background: var(--accent, #7c3aed);
    transition: width 0.25s linear;
  }

  @keyframes pulse {
    0%, 100% { transform: scale(1); opacity: 1; }
    50% { transform: scale(1.4); opacity: 0.6; }
  }

  .volume-row {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    max-width: 200px;
    color: var(--text-muted, #888);
  }
  .volume-slider { flex: 1; accent-color: var(--accent, #7c3aed); }

  .participants-area { width: 100%; }
  .participants-heading {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-muted, #888);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    margin: 0 0 10px;
  }
  .participants-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .participant-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 0;
  }
  .avatar {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--bg-elevated, #1e1e1e);
    color: var(--accent, #7c3aed);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.75rem;
    font-weight: 700;
    flex-shrink: 0;
    border: 1px solid var(--border, #333);
  }
  .pname { font-size: 0.85rem; color: var(--text, #fff); }
  .pname em { color: var(--text-muted, #888); font-style: normal; }
  .self .avatar { border-color: var(--accent, #7c3aed); }
  .host .avatar { border-color: #f59e0b; color: #f59e0b; }

  /* Lyrics — mirrors LyricsModal styling exactly */
  .lyrics-section {
    width: 100%;
    display: flex;
    flex-direction: column;
    background: var(--bg-elevated, #1a1a2e);
    border: 1px solid var(--border-2, #252530);
    border-radius: 10px;
    overflow: hidden;
  }
  .lyrics-toggle {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    border: none;
    border-bottom: 1px solid var(--border, #333);
    background: none;
    cursor: pointer;
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-muted, #888);
    width: 100%;
  }
  .lyrics-toggle:hover { background: var(--bg-hover, #ffffff08); color: var(--text, #fff); }
  .chevron {
    font-size: 0.7rem;
    transition: transform 0.2s;
    display: inline-block;
    margin-left: auto;
    color: var(--text-muted, #888);
  }
  .chevron.open { transform: rotate(180deg); }

  .lyrics-list {
    flex: 1;
    overflow-y: auto;
    padding: 16px 12px 20px;
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-height: 280px;
  }
  .lyrics-list::-webkit-scrollbar { width: 4px; }
  .lyrics-list::-webkit-scrollbar-track { background: transparent; }
  .lyrics-list::-webkit-scrollbar-thumb { background: var(--border, #333); border-radius: 2px; }

  .lyric-line {
    padding: 6px 10px;
    font-size: 0.9rem;
    line-height: 1.5;
    color: var(--text-muted, #888);
    border-radius: 6px;
    transition: color 0.3s ease, background 0.3s ease, opacity 0.3s ease, transform 0.3s ease;
    cursor: default;
    opacity: 0.55;
  }
  .lyric-line.past {
    color: var(--text-2, #9090a8);
    opacity: 0.35;
  }
  .lyric-line.active {
    font-size: 1rem;
    font-weight: 600;
    background: var(--accent-dim, rgba(192,132,252,0.12));
    color: var(--accent, #7c3aed);
    opacity: 1;
    transform: scale(1.01);
  }
  .lyric-line.next {
    color: var(--text, #fff);
  }
</style>
