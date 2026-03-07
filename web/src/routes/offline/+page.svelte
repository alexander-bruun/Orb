<script lang="ts">
  import {
    downloads,
    restoreDownloads,
    getOfflineBlobUrl,
    type DownloadEntry,
  } from "$lib/stores/offline/downloads";
  import { isOffline, checkConnectivity } from "$lib/stores/offline/connectivity";
  import { onMount, onDestroy } from "svelte";
  import { goto } from "$app/navigation";

  // Ensure downloads metadata is loaded from localStorage
  onMount(() => {
    restoreDownloads();
  });

  // Build a list of completed downloads
  $: doneEntries = [...$downloads.values()].filter((e) => e.status === "done");

  // Group by album
  $: albumGroups = (() => {
    const map = new Map<string, DownloadEntry[]>();
    for (const entry of doneEntries) {
      const key = entry.albumName || "Unknown Album";
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(entry);
    }
    return [...map.entries()].sort(([a], [b]) => a.localeCompare(b));
  })();

  // Search
  let search = "";
  $: filtered = search.trim()
    ? albumGroups
        .map(([album, tracks]) => {
          const q = search.toLowerCase();
          const matchedTracks = tracks.filter(
            (t) =>
              t.title.toLowerCase().includes(q) ||
              t.artistName.toLowerCase().includes(q) ||
              t.albumName.toLowerCase().includes(q),
          );
          return [album, matchedTracks] as [string, DownloadEntry[]];
        })
        .filter(([, tracks]) => tracks.length > 0)
    : albumGroups;

  // ── Playback state ────────────────────────────────────────────
  let playingId: string | null = null;
  let audioEl: HTMLAudioElement | null = null;
  let queue: DownloadEntry[] = [];
  let queueIndex = -1;
  let isShuffled = false;
  let progress = 0;
  let duration = 0;
  let progressInterval: ReturnType<typeof setInterval> | null = null;

  function clearProgressInterval() {
    if (progressInterval) {
      clearInterval(progressInterval);
      progressInterval = null;
    }
  }

  function startProgressTracking() {
    clearProgressInterval();
    progressInterval = setInterval(() => {
      if (audioEl) {
        progress = audioEl.currentTime;
        duration = audioEl.duration || 0;
      }
    }, 250);
  }

  onDestroy(() => {
    stopPlayback();
    clearProgressInterval();
  });

  function shuffleArray<T>(arr: T[]): T[] {
    const a = [...arr];
    for (let i = a.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [a[i], a[j]] = [a[j], a[i]];
    }
    return a;
  }

  async function playTrack(entry: DownloadEntry) {
    // Stop current playback
    if (audioEl) {
      audioEl.pause();
      if (audioEl.src.startsWith("blob:")) URL.revokeObjectURL(audioEl.src);
      audioEl = null;
    }
    clearProgressInterval();

    const blobUrl = await getOfflineBlobUrl(entry.trackId);
    if (!blobUrl) return;

    playingId = entry.trackId;
    progress = 0;
    duration = 0;
    audioEl = new Audio(blobUrl);
    audioEl.addEventListener("ended", () => {
      if (audioEl?.src.startsWith("blob:")) URL.revokeObjectURL(audioEl.src);
      playNext();
    });
    startProgressTracking();
    audioEl.play();
  }

  /** Play a specific track and set it as the active queue position. */
  async function playFromQueue(entry: DownloadEntry) {
    // If we have no queue yet, build one from all done entries in order
    if (queue.length === 0) {
      queue = doneEntries;
      isShuffled = false;
    }
    // Find the entry in the current queue
    const idx = queue.findIndex((e) => e.trackId === entry.trackId);
    if (idx >= 0) queueIndex = idx;
    await playTrack(entry);
  }

  function playNext() {
    if (queue.length === 0) {
      stopPlayback();
      return;
    }
    queueIndex = (queueIndex + 1) % queue.length;
    playTrack(queue[queueIndex]);
  }

  function playPrev() {
    if (queue.length === 0) return;
    // If we're more than 3 s into the track, restart it
    if (audioEl && audioEl.currentTime > 3) {
      audioEl.currentTime = 0;
      return;
    }
    queueIndex = (queueIndex - 1 + queue.length) % queue.length;
    playTrack(queue[queueIndex]);
  }

  function toggleShuffle() {
    if (doneEntries.length === 0) return;

    if (isShuffled && queue.length > 0) {
      // Un-shuffle: restore album order, keep current track playing
      const currentEntry = playingId ? queue[queueIndex] : null;
      queue = doneEntries;
      isShuffled = false;
      if (currentEntry) {
        queueIndex = queue.findIndex((e) => e.trackId === currentEntry.trackId);
        if (queueIndex < 0) queueIndex = 0;
      }
    } else {
      // Shuffle all tracks
      const currentEntry = playingId ? queue[queueIndex] : null;
      queue = shuffleArray(doneEntries);
      isShuffled = true;
      if (currentEntry) {
        // Move current track to front so playback isn't interrupted
        const idx = queue.findIndex((e) => e.trackId === currentEntry.trackId);
        if (idx > 0) {
          [queue[0], queue[idx]] = [queue[idx], queue[0]];
        }
        queueIndex = 0;
      }
    }
  }

  /** Shuffle all and start playing immediately. */
  function shufflePlay() {
    if (doneEntries.length === 0) return;
    queue = shuffleArray(doneEntries);
    isShuffled = true;
    queueIndex = 0;
    playTrack(queue[0]);
  }

  function stopPlayback() {
    if (audioEl) {
      audioEl.pause();
      if (audioEl.src.startsWith("blob:")) URL.revokeObjectURL(audioEl.src);
      audioEl = null;
    }
    playingId = null;
    progress = 0;
    duration = 0;
    clearProgressInterval();
  }

  function togglePause() {
    if (!audioEl) return;
    if (audioEl.paused) {
      audioEl.play();
    } else {
      audioEl.pause();
    }
    // Force reactivity
    playingId = playingId;
  }

  $: currentEntry = playingId
    ? (doneEntries.find((e) => e.trackId === playingId) ?? null)
    : null;
  $: isPaused = audioEl ? audioEl.paused : true;
  // Re-check paused state when playingId changes
  $: if (playingId) {
    isPaused = audioEl?.paused ?? true;
  }

  async function retryConnection() {
    const stillOffline = await checkConnectivity();
    if (!stillOffline) {
      goto("/");
    }
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024)
      return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
  }

  function formatTime(s: number): string {
    if (!s || !isFinite(s)) return "0:00";
    const m = Math.floor(s / 60);
    const sec = Math.floor(s % 60);
    return `${m}:${sec.toString().padStart(2, "0")}`;
  }
</script>

<div class="offline-scroll">
  <div class="offline-page">
    <header class="offline-header">
      <div class="offline-icon">
        <svg
          width="48"
          height="48"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <line x1="1" y1="1" x2="23" y2="23" />
          <path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55" />
          <path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39" />
          <path d="M10.71 5.05A16 16 0 0 1 22.56 9" />
          <path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88" />
          <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
          <line x1="12" y1="20" x2="12.01" y2="20" />
        </svg>
      </div>
      <h1>Offline Mode</h1>
      <p class="subtitle">
        The server is unreachable. You can still play your downloaded music.
      </p>
      <button class="btn-retry" on:click={retryConnection}>
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
          <polyline points="23 4 23 10 17 10" />
          <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" />
        </svg>
        Retry Connection
      </button>
    </header>

    {#if doneEntries.length === 0}
      <div class="empty">
        <p>No downloaded tracks available.</p>
        <p class="hint">Download music while connected to play it offline.</p>
      </div>
    {:else}
      <div class="stats">
        <span
          >{doneEntries.length} track{doneEntries.length === 1 ? "" : "s"}</span
        >
        <span class="dot">·</span>
        <span
          >{albumGroups.length} album{albumGroups.length === 1 ? "" : "s"}</span
        >
        <span class="dot">·</span>
        <span
          >{formatSize(doneEntries.reduce((s, e) => s + e.sizeBytes, 0))}</span
        >
      </div>

      <div class="controls-row">
        <button class="btn-shuffle-play" on:click={shufflePlay}>
          <svg
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <polyline points="16 3 21 3 21 8" /><line
              x1="4"
              y1="20"
              x2="21"
              y2="3"
            />
            <polyline points="21 16 21 21 16 21" /><line
              x1="15"
              y1="15"
              x2="21"
              y2="21"
            />
            <line x1="4" y1="4" x2="9" y2="9" />
          </svg>
          Shuffle All
        </button>
      </div>

      {#if doneEntries.length > 5}
        <div class="search-bar">
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
            <circle cx="11" cy="11" r="8" /><path d="m21 21-4.35-4.35" />
          </svg>
          <input
            type="text"
            placeholder="Search downloaded tracks…"
            bind:value={search}
          />
        </div>
      {/if}

      <div class="album-list">
        {#each filtered as [albumName, tracks]}
          <div class="album-group">
            <div class="album-header">
              <span class="album-name">{albumName}</span>
              <span class="album-count"
                >{tracks.length} track{tracks.length === 1 ? "" : "s"}</span
              >
            </div>
            <div class="track-list">
              {#each tracks as entry (entry.trackId)}
                <button
                  class="track-row"
                  class:playing={playingId === entry.trackId}
                  on:click={() =>
                    playingId === entry.trackId
                      ? togglePause()
                      : playFromQueue(entry)}
                >
                  <div class="track-play-icon">
                    {#if playingId === entry.trackId}
                      <svg
                        width="16"
                        height="16"
                        viewBox="0 0 24 24"
                        fill="currentColor"
                      >
                        <rect x="6" y="4" width="4" height="16" /><rect
                          x="14"
                          y="4"
                          width="4"
                          height="16"
                        />
                      </svg>
                    {:else}
                      <svg
                        width="16"
                        height="16"
                        viewBox="0 0 24 24"
                        fill="currentColor"
                      >
                        <polygon points="5 3 19 12 5 21 5 3" />
                      </svg>
                    {/if}
                  </div>
                  <div class="track-info">
                    <span class="track-title">{entry.title}</span>
                    <span class="track-artist">{entry.artistName}</span>
                  </div>
                  <span class="track-size">{formatSize(entry.sizeBytes)}</span>
                </button>
              {/each}
            </div>
          </div>
        {/each}
      </div>
    {/if}

    <!-- Now-playing bar -->
    {#if currentEntry}
      <div class="now-playing-bar">
        <div class="np-progress-bar">
          <div
            class="np-progress-fill"
            style="width: {duration > 0 ? (progress / duration) * 100 : 0}%"
          ></div>
        </div>
        <div class="np-content">
          <div class="np-info">
            <span class="np-title">{currentEntry.title}</span>
            <span class="np-artist">{currentEntry.artistName}</span>
          </div>
          <div class="np-time">
            {formatTime(progress)} / {formatTime(duration)}
          </div>
          <div class="np-controls">
            <button
              class="np-btn"
              on:click={toggleShuffle}
              class:active={isShuffled}
              title={isShuffled ? "Disable shuffle" : "Enable shuffle"}
            >
              <svg
                width="18"
                height="18"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <polyline points="16 3 21 3 21 8" /><line
                  x1="4"
                  y1="20"
                  x2="21"
                  y2="3"
                />
                <polyline points="21 16 21 21 16 21" /><line
                  x1="15"
                  y1="15"
                  x2="21"
                  y2="21"
                />
                <line x1="4" y1="4" x2="9" y2="9" />
              </svg>
            </button>
            <button class="np-btn" on:click={playPrev} title="Previous">
              <svg
                width="20"
                height="20"
                viewBox="0 0 24 24"
                fill="currentColor"
              >
                <path d="M6 6h2v12H6zm3.5 6 8.5 6V6z" />
              </svg>
            </button>
            <button
              class="np-btn np-btn-play"
              on:click={togglePause}
              title={isPaused ? "Play" : "Pause"}
            >
              {#if isPaused}
                <svg
                  width="22"
                  height="22"
                  viewBox="0 0 24 24"
                  fill="currentColor"
                >
                  <polygon points="5 3 19 12 5 21 5 3" />
                </svg>
              {:else}
                <svg
                  width="22"
                  height="22"
                  viewBox="0 0 24 24"
                  fill="currentColor"
                >
                  <rect x="6" y="4" width="4" height="16" /><rect
                    x="14"
                    y="4"
                    width="4"
                    height="16"
                  />
                </svg>
              {/if}
            </button>
            <button class="np-btn" on:click={playNext} title="Next">
              <svg
                width="20"
                height="20"
                viewBox="0 0 24 24"
                fill="currentColor"
              >
                <path d="M6 18l8.5-6L6 6v12zM16 6v12h2V6h-2z" />
              </svg>
            </button>
            <button class="np-btn" on:click={stopPlayback} title="Stop">
              <svg
                width="18"
                height="18"
                viewBox="0 0 24 24"
                fill="currentColor"
              >
                <rect x="5" y="5" width="14" height="14" rx="2" />
              </svg>
            </button>
          </div>
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .offline-scroll {
    height: 100dvh;
    overflow-y: auto;
  }

  .offline-page {
    max-width: 700px;
    margin: 0 auto;
    padding: 40px 20px 120px;
  }

  .offline-header {
    text-align: center;
    margin-bottom: 32px;
  }

  .offline-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 80px;
    height: 80px;
    border-radius: 50%;
    background: color-mix(in srgb, var(--accent) 12%, transparent);
    color: var(--accent);
    margin-bottom: 16px;
  }

  h1 {
    font-size: 1.5rem;
    font-weight: 700;
    margin: 0 0 8px;
  }

  .subtitle {
    color: var(--text-muted);
    font-size: 0.9rem;
    margin: 0 0 16px;
  }

  .btn-retry {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 18px;
    border-radius: 20px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text-muted);
    font-size: 0.85rem;
    font-weight: 600;
    cursor: pointer;
    transition:
      color 0.15s,
      border-color 0.15s;
  }

  .btn-retry:hover {
    color: var(--text);
    border-color: var(--text);
  }

  /* ── Empty state ───────────────────────────── */

  .empty {
    text-align: center;
    padding: 48px 20px;
    color: var(--text-muted);
  }

  .empty p {
    margin: 4px 0;
  }
  .hint {
    font-size: 0.85rem;
    opacity: 0.7;
  }

  /* ── Stats ──────────────────────────────────── */

  .stats {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.85rem;
    color: var(--text-muted);
    margin-bottom: 16px;
  }

  .dot {
    opacity: 0.4;
  }

  /* ── Search ─────────────────────────────────── */

  .search-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    margin-bottom: 20px;
    color: var(--text-muted);
  }

  .search-bar input {
    flex: 1;
    background: none;
    border: none;
    outline: none;
    color: var(--text);
    font-size: 0.875rem;
  }

  .search-bar input::placeholder {
    color: var(--text-muted);
    opacity: 0.6;
  }

  /* ── Album groups ──────────────────────────── */

  .album-list {
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .album-header {
    display: flex;
    align-items: baseline;
    gap: 10px;
    margin-bottom: 6px;
    padding: 0 4px;
  }

  .album-name {
    font-weight: 600;
    font-size: 0.95rem;
    color: var(--text);
  }

  .album-count {
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  /* ── Track list ────────────────────────────── */

  .track-list {
    display: flex;
    flex-direction: column;
    gap: 1px;
    background: var(--border);
    border-radius: 8px;
    overflow: hidden;
  }

  .track-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    background: var(--bg-elevated);
    border: none;
    cursor: pointer;
    text-align: left;
    transition: background 0.12s;
    width: 100%;
    color: var(--text);
    font-family: inherit;
  }

  .track-row:hover {
    background: var(--bg-hover);
  }
  .track-row.playing {
    background: color-mix(in srgb, var(--accent) 10%, var(--bg-elevated));
  }

  .track-play-icon {
    flex-shrink: 0;
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    background: color-mix(in srgb, var(--accent) 15%, transparent);
    color: var(--accent);
  }

  .track-row.playing .track-play-icon {
    background: var(--accent);
    color: #fff;
  }

  .track-info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .track-title {
    font-size: 0.875rem;
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .track-artist {
    font-size: 0.75rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .track-size {
    flex-shrink: 0;
    font-size: 0.75rem;
    color: var(--text-muted);
    opacity: 0.6;
  }

  /* ── Mobile ─────────────────────────────────── */

  @media (max-width: 640px) {
    .offline-page {
      padding: 24px 16px 200px;
    }
    h1 {
      font-size: 1.25rem;
    }
    .offline-icon {
      width: 64px;
      height: 64px;
    }
    .offline-icon svg {
      width: 36px;
      height: 36px;
    }
    .np-content {
      flex-wrap: wrap;
      gap: 8px;
    }
    .np-info {
      width: 100%;
    }
    .np-time {
      display: none;
    }
    .controls-row {
      justify-content: center;
    }
  }

  /* ── Controls row (shuffle button) ────────── */

  .controls-row {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 16px;
  }

  .btn-shuffle-play {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 20px;
    border-radius: 20px;
    border: none;
    background: var(--accent);
    color: #fff;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 0.15s;
  }

  .btn-shuffle-play:hover {
    opacity: 0.85;
  }

  /* ── Now-playing bar ───────────────────────── */

  .now-playing-bar {
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
    background: var(--bg-elevated);
    border-top: 1px solid var(--border);
    z-index: 100;
  }

  .np-progress-bar {
    height: 3px;
    background: var(--border);
  }

  .np-progress-fill {
    height: 100%;
    background: var(--accent);
    transition: width 0.25s linear;
    min-width: 0;
  }

  .np-content {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 10px 20px;
    max-width: 700px;
    margin: 0 auto;
  }

  .np-info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .np-title {
    font-size: 0.85rem;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .np-artist {
    font-size: 0.75rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .np-time {
    flex-shrink: 0;
    font-size: 0.75rem;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }

  .np-controls {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-shrink: 0;
  }

  .np-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    border: none;
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
    transition:
      color 0.15s,
      background 0.15s;
    padding: 0;
    font-family: inherit;
  }

  .np-btn:hover {
    color: var(--text);
    background: var(--bg-hover);
  }
  .np-btn.active {
    color: var(--accent);
  }

  .np-btn-play {
    width: 40px;
    height: 40px;
    background: var(--accent);
    color: #fff;
  }

  .np-btn-play:hover {
    background: var(--accent);
    opacity: 0.85;
    color: #fff;
  }
</style>
