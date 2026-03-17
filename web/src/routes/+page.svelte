<script lang="ts">
  import { onMount } from "svelte";
  import { library as libApi } from "$lib/api/library";
  import { smartPlaylists as spApi } from "$lib/api/smartPlaylists";
  import { audiobooks as abApi } from "$lib/api/audiobooks";
  import type { Track, Album, SmartPlaylist, Audiobook } from "$lib/types";
  import TrackList from "$lib/components/library/TrackList.svelte";
  import AlbumCard from "$lib/components/library/AlbumCard.svelte";
  import Skeleton from "$lib/components/ui/Skeleton.svelte";
  import { playTrack, shuffle as shuffleStore } from "$lib/stores/player";
  import { playAudiobook } from "$lib/stores/audiobookPlayer";
  import { downloads } from "$lib/stores/offline/downloads";
  import { isOffline } from "$lib/stores/offline/connectivity";
  import { getApiBase } from "$lib/api/base";

  const PAGE_SIZE = 10;

  type Interval = "today" | "week" | "month" | "all" | "custom";

  let interval: Interval = "all";
  let customFrom = "";
  let customTo = "";

  let recentTracks: Track[] = [];
  let mostTracks: Track[] = [];
  let recentAlbums: Album[] = [];
  let newAlbums: Album[] = [];
  let smartPls: SmartPlaylist[] = [];
  type InProgressBook = Audiobook & { position_ms: number; progress_updated_at: string };
  let inProgressBooks: InProgressBook[] = [];
  let loading = true;
  let playsLoading = false;

  let recentPage = 1;
  let mostPage = 1;

  $: recentPages = Math.max(1, Math.ceil(recentTracks.length / PAGE_SIZE));
  $: mostPages = Math.max(1, Math.ceil(mostTracks.length / PAGE_SIZE));
  $: pagedRecent = recentTracks.slice(
    (recentPage - 1) * PAGE_SIZE,
    recentPage * PAGE_SIZE,
  );
  $: pagedMost = mostTracks.slice(
    (mostPage - 1) * PAGE_SIZE,
    mostPage * PAGE_SIZE,
  );

  // ── Offline: derive playable stubs from the downloads store ─────────────────
  // The player streams via /api/stream/{id}, intercepted by the service worker
  // from IndexedDB when the track is downloaded — only `id` is required.
  $: offlineTracks = [...$downloads.values()]
    .filter((e) => e.status === "done")
    .map(
      (e) =>
        ({
          id: e.trackId,
          title: e.title,
          artist_name: e.artistName,
          album_name: e.albumName,
          album_id: e.albumId,
          disc_number: 0,
          duration_ms: 0,
          file_key: "",
          file_size: e.sizeBytes,
          format: "flac" as const,
          sample_rate: 44100,
          channels: 2,
        }) satisfies Track,
    );

  function playAllOffline() {
    if (offlineTracks.length > 0) playTrack(offlineTracks[0], offlineTracks);
  }

  function shuffleOffline() {
    if (offlineTracks.length === 0) return;
    shuffleStore.set(true);
    playTrack(
      offlineTracks[Math.floor(Math.random() * offlineTracks.length)],
      offlineTracks,
    );
  }

  // ── Online data loading ──────────────────────────────────────────────────────
  const INTERVALS: { key: Interval; label: string }[] = [
    { key: "today", label: "Today" },
    { key: "week", label: "Week" },
    { key: "month", label: "Month" },
    { key: "all", label: "All time" },
    { key: "custom", label: "Custom" },
  ];

  function getDateRange(): { from?: string; to?: string } {
    const now = new Date();
    switch (interval) {
      case "today": {
        const from = new Date(now);
        from.setHours(0, 0, 0, 0);
        return { from: from.toISOString() };
      }
      case "week": {
        const from = new Date(now);
        from.setDate(from.getDate() - 7);
        return { from: from.toISOString() };
      }
      case "month": {
        const from = new Date(now);
        from.setDate(from.getDate() - 30);
        return { from: from.toISOString() };
      }
      case "custom": {
        const range: { from?: string; to?: string } = {};
        if (customFrom) range.from = new Date(customFrom).toISOString();
        if (customTo) {
          const to = new Date(customTo);
          to.setDate(to.getDate() + 1);
          range.to = to.toISOString();
        }
        return range;
      }
      default:
        return {};
    }
  }

  async function loadPlays() {
    playsLoading = true;
    const { from, to } = getDateRange();
    try {
      [recentTracks, mostTracks] = await Promise.all([
        libApi.recentlyPlayed(100, from, to).then((r) => r ?? []),
        libApi.mostPlayed(100, from, to).then((r) => r ?? []),
      ]);
      recentPage = 1;
      mostPage = 1;
    } catch {
      // ignore
    } finally {
      playsLoading = false;
    }
  }

  function selectInterval(iv: Interval) {
    interval = iv;
    if (iv === "custom") {
      if (customFrom) loadPlays();
    } else {
      loadPlays();
    }
  }

  function handleCustomDateChange() {
    loadPlays();
  }

  onMount(async () => {
    if ($isOffline) {
      loading = false;
      return;
    }
    try {
      [recentTracks, mostTracks, recentAlbums, newAlbums, smartPls, inProgressBooks] = await Promise.all([
        libApi.recentlyPlayed(100).then((r) => r ?? []),
        libApi.mostPlayed(100).then((r) => r ?? []),
        libApi.recentlyPlayedAlbums().then((r) => r ?? []),
        libApi.recentlyAddedAlbums(20).then((r) => r ?? []),
        spApi.list().then((r) => r ?? []),
        abApi.inProgress(10).then((r) => r.audiobooks ?? []).catch(() => []),
      ]);
    } catch {
      // ignore — user may not be logged in
    } finally {
      loading = false;
    }
  });
</script>

<!-- ── Offline view ─────────────────────────────────────────────────────────── -->
{#if $isOffline}
  <div class="offline-view">
    <div class="offline-header">
      <div class="offline-title-row">
        <h2 class="title">Downloads</h2>
        <span class="offline-badge">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <line x1="1" y1="1" x2="23" y2="23"/>
            <path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55"/>
            <path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39"/>
            <path d="M10.71 5.05A16 16 0 0 1 22.56 9"/>
            <path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88"/>
            <path d="M8.53 16.11a6 6 0 0 1 6.95 0"/>
            <line x1="12" y1="20" x2="12.01" y2="20"/>
          </svg>
          Offline
        </span>
      </div>

      {#if offlineTracks.length > 0}
        <div class="offline-actions">
          <button class="btn-play" on:click={playAllOffline}>▶ Play</button>
          <button class="btn-shuffle" on:click={shuffleOffline} title="Shuffle">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/>
              <polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/>
              <line x1="4" y1="4" x2="9" y2="9"/>
            </svg>
            Shuffle
          </button>
          <span class="track-count">{offlineTracks.length} track{offlineTracks.length === 1 ? "" : "s"}</span>
        </div>
      {/if}
    </div>

    {#if offlineTracks.length === 0}
      <div class="empty-offline">
        <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <line x1="1" y1="1" x2="23" y2="23"/>
          <path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55"/>
          <path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39"/>
          <path d="M10.71 5.05A16 16 0 0 1 22.56 9"/>
          <path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88"/>
          <path d="M8.53 16.11a6 6 0 0 1 6.95 0"/>
          <line x1="12" y1="20" x2="12.01" y2="20"/>
        </svg>
        <p>You're offline and have no downloaded tracks.</p>
        <p class="muted">Download your favorites while connected to listen without a network.</p>
      </div>
    {:else}
      <TrackList tracks={offlineTracks} showCover={true} />
    {/if}
  </div>

<!-- ── Online loading skeleton ─────────────────────────────────────────────── -->
{:else if loading}
  <div class="skeleton-home">
    <!-- Album slider skeleton -->
    <div class="skeleton-section">
      <Skeleton width="160px" height="1.1rem" radius="4px" />
      <div class="skeleton-slider">
        {#each { length: 6 } as _}
          <div class="skeleton-album">
            <Skeleton width="134px" height="134px" radius="6px" />
            <Skeleton width="90px" height="0.8rem" />
            <Skeleton width="60px" height="0.75rem" />
          </div>
        {/each}
      </div>
    </div>
    <!-- Track list skeleton -->
    <div class="skeleton-section">
      <Skeleton width="140px" height="1.1rem" radius="4px" />
      <div class="skeleton-tracks">
        {#each { length: 8 } as _}
          <div class="skeleton-row">
            <Skeleton width="36px" height="36px" radius="4px" />
            <div class="skeleton-text">
              <Skeleton width="50%" height="0.85rem" />
              <Skeleton width="32%" height="0.75rem" />
            </div>
          </div>
        {/each}
      </div>
    </div>
  </div>

<!-- ── Normal online home ───────────────────────────────────────────────────── -->
{:else}
  {#if inProgressBooks.length > 0}
    <section class="home-section">
      <div class="section-header">
        <h2 class="section-title">Continue Listening</h2>
        <a href="/audiobooks" class="view-all">View all</a>
      </div>
      <div class="ab-slider">
        {#each inProgressBooks as book (book.id)}
          {@const pct = book.duration_ms > 0 ? Math.min(100, (book.position_ms / book.duration_ms) * 100) : 0}
          <!-- svelte-ignore a11y-click-events-have-key-events -->
          <!-- svelte-ignore a11y-no-static-element-interactions -->
          <a class="ab-card" href="/audiobooks/{book.id}">
            <div class="ab-cover-wrap">
              {#if book.cover_art_key}
                <img src="{getApiBase()}/covers/audiobook/{book.id}" alt={book.title} class="ab-cover" loading="lazy" />
              {:else}
                <div class="ab-cover ab-placeholder">
                  <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                    <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"/>
                    <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"/>
                  </svg>
                </div>
              {/if}
              <!-- progress strip at bottom of cover -->
              <div class="ab-progress-strip">
                <div class="ab-progress-fill" style="width:{pct}%"></div>
              </div>
              <button class="ab-play-btn" aria-label="Resume {book.title}" on:click|preventDefault|stopPropagation={() => playAudiobook(book, book.position_ms)}>
                <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M4 2.5l10 5.5-10 5.5V2.5z"/></svg>
              </button>
            </div>
            <div class="ab-info">
              <span class="ab-title" title={book.title}>{book.title}</span>
              {#if book.author_name}<span class="ab-author">{book.author_name}</span>{/if}
            </div>
          </a>
        {/each}
      </div>
    </section>
  {/if}

  {#if recentTracks.length > 0 || mostTracks.length > 0}
    <section class="home-section">
      <div class="plays-controls">
        <div class="interval-tabs">
          {#each INTERVALS as iv}
            <button
              class="interval-tab"
              class:active={interval === iv.key}
              on:click={() => selectInterval(iv.key)}
            >
              {iv.label}
            </button>
          {/each}
        </div>
        {#if interval === "custom"}
          <div class="date-range">
            <input
              type="date"
              class="date-input"
              bind:value={customFrom}
              on:change={handleCustomDateChange}
            />
            <span class="date-sep">–</span>
            <input
              type="date"
              class="date-input"
              bind:value={customTo}
              on:change={handleCustomDateChange}
            />
          </div>
        {/if}
      </div>

      <div class="plays-columns">
        <div class="plays-col">
          <div class="col-header">
            <h2 class="col-title">Recently Played</h2>
            {#if recentPages > 1}
              <label class="page-label">
                Page
                <select class="page-select" bind:value={recentPage}>
                  {#each Array.from({ length: recentPages }, (_, i) => i + 1) as p}
                    <option value={p}>{p} / {recentPages}</option>
                  {/each}
                </select>
              </label>
            {/if}
          </div>
          {#if playsLoading}
            <p class="muted">Loading…</p>
          {:else if pagedRecent.length > 0}
            <TrackList tracks={pagedRecent} showCover showDiscNumbers={false} />
          {:else}
            <p class="muted">No plays in this period.</p>
          {/if}
        </div>

        <div class="plays-col">
          <div class="col-header">
            <h2 class="col-title">Most Played</h2>
            {#if mostPages > 1}
              <label class="page-label">
                Page
                <select class="page-select" bind:value={mostPage}>
                  {#each Array.from({ length: mostPages }, (_, i) => i + 1) as p}
                    <option value={p}>{p} / {mostPages}</option>
                  {/each}
                </select>
              </label>
            {/if}
          </div>
          {#if playsLoading}
            <p class="muted">Loading…</p>
          {:else if pagedMost.length > 0}
            <TrackList tracks={pagedMost} showCover showDiscNumbers={false} />
          {:else}
            <p class="muted">No plays in this period.</p>
          {/if}
        </div>
      </div>
    </section>
  {/if}

  {#if recentAlbums.length > 0}
    <section class="home-section">
      <h2 class="section-title" style="margin-bottom: 16px;">
        Recently Played Albums
      </h2>
      <div class="album-slider">
        {#each recentAlbums as album (album.id)}
          <div class="slider-item">
            <AlbumCard {album} />
          </div>
        {/each}
      </div>
    </section>
  {/if}

  {#if newAlbums.length > 0}
    <section class="home-section">
      <div class="section-header">
        <h2 class="section-title">Recently Added</h2>
        <a href="/library" class="view-all">View all</a>
      </div>
      <div class="album-slider">
        {#each newAlbums as album (album.id)}
          <div class="slider-item">
            <AlbumCard {album} />
          </div>
        {/each}
      </div>
    </section>
  {/if}

  {#if smartPls.length > 0}
    <section class="home-section">
      <div class="section-header">
        <h2 class="section-title">Smart Playlists</h2>
        <a href="/smart-playlists" class="view-all">View all</a>
      </div>
      <div class="sp-grid">
        {#each smartPls.slice(0, 6) as sp (sp.id)}
          <a class="sp-card" href="/smart-playlists/{sp.id}">
            <div class="sp-icon" aria-hidden="true">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/>
                <line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/>
              </svg>
            </div>
            <div class="sp-info">
              <span class="sp-name">{sp.name}</span>
              <span class="sp-meta">{sp.rules.length} rule{sp.rules.length === 1 ? '' : 's'}</span>
            </div>
          </a>
        {/each}
      </div>
    </section>
  {/if}

  {#if recentTracks.length === 0 && mostTracks.length === 0 && recentAlbums.length === 0 && newAlbums.length === 0 && smartPls.length === 0 && inProgressBooks.length === 0}
    <p class="muted">Nothing here yet. Go find some music!</p>
  {/if}
{/if}

<svelte:head><title>Home – Orb</title></svelte:head>

<style>
  /* ── Offline view ── */
  .offline-view { padding-top: 4px; }

  .offline-header { margin-bottom: 20px; }

  .offline-title-row {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
  }
  .title { font-size: 1.25rem; font-weight: 600; margin: 0; }

  .offline-badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-muted);
    background: var(--bg-3);
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 3px 10px;
  }

  .offline-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .btn-play {
    background: var(--accent);
    border: none;
    border-radius: 20px;
    padding: 7px 18px;
    color: #fff;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-play:hover { background: var(--accent-hover); }

  .btn-shuffle {
    display: flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 6px 14px;
    color: var(--text-muted);
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-shuffle:hover { color: var(--text); border-color: var(--text); }

  .track-count {
    font-size: 0.8rem;
    color: var(--text-muted);
    margin-left: 4px;
  }

  .empty-offline {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 48px 16px;
    color: var(--text-muted);
    text-align: center;
  }
  .empty-offline p { margin: 0; font-size: 0.9rem; }
  .empty-offline svg { opacity: 0.4; }

  /* ── Loading skeleton ── */
  .skeleton-home { display: flex; flex-direction: column; gap: 40px; }
  .skeleton-section { display: flex; flex-direction: column; gap: 16px; }

  .skeleton-slider {
    display: flex;
    gap: 16px;
    overflow: hidden;
  }
  .skeleton-album {
    display: flex;
    flex-direction: column;
    gap: 8px;
    flex: 0 0 134px;
  }

  .skeleton-tracks { display: flex; flex-direction: column; gap: 2px; }
  .skeleton-row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 6px 8px;
    border-radius: 6px;
  }
  .skeleton-text {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  /* ── Normal home ── */
  .home-section { margin-bottom: 40px; }

  .section-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 16px;
  }

  .section-title { font-size: 1.125rem; font-weight: 600; margin: 0; }

  .view-all {
    font-size: 0.8rem;
    color: var(--text-muted);
    text-decoration: none;
    letter-spacing: 0.02em;
  }
  .view-all:hover { color: var(--text); }

  .plays-controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    margin-bottom: 20px;
  }

  .interval-tabs { display: flex; gap: 4px; }

  .interval-tab {
    background: none;
    border: 1px solid var(--border);
    border-radius: 20px;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 0.8rem;
    padding: 4px 12px;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
  }
  .interval-tab:hover { color: var(--text); border-color: var(--text-muted); }
  .interval-tab.active { background: var(--accent); border-color: var(--accent); color: #fff; }

  .date-range { display: flex; align-items: center; gap: 8px; }

  .date-input {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text);
    font-size: 0.8rem;
    padding: 3px 8px;
    cursor: pointer;
  }
  .date-input:focus { outline: none; border-color: var(--accent); }

  .date-sep { color: var(--text-muted); font-size: 0.8rem; }

  .plays-columns {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 32px;
  }
  @media (max-width: 900px) {
    .plays-columns { grid-template-columns: 1fr; }
  }

  .col-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 12px;
  }

  .col-title { font-size: 1.125rem; font-weight: 600; margin: 0; }

  .page-label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.8rem;
    color: var(--text-muted);
  }

  .page-select {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text-muted);
    font-size: 0.8rem;
    padding: 2px 6px;
    cursor: pointer;
  }
  .page-select:focus { outline: none; border-color: var(--accent); }

  .muted { color: var(--text-muted); font-size: 0.875rem; }

  .sp-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
    gap: 10px;
  }
  .sp-card {
    display: flex;
    align-items: center;
    gap: 10px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px;
    text-decoration: none;
    color: inherit;
    transition: background 0.15s, border-color 0.15s;
  }
  .sp-card:hover { background: var(--bg-hover); border-color: var(--text-muted); }
  .sp-icon {
    width: 36px;
    height: 36px;
    background: var(--bg-3, var(--bg-hover));
    border-radius: 6px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    color: var(--accent);
  }
  .sp-info { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .sp-name { font-size: 0.85rem; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .sp-meta { font-size: 0.72rem; color: var(--text-muted); }

  /* ── Continue Listening audiobook cards ── */
  .ab-slider {
    display: flex;
    gap: 16px;
    overflow-x: auto;
    padding-bottom: 8px;
    scrollbar-width: thin;
    scrollbar-color: var(--border) transparent;
  }
  .ab-slider::-webkit-scrollbar { height: 4px; }
  .ab-slider::-webkit-scrollbar-track { background: transparent; }
  .ab-slider::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }

  .ab-card {
    flex: 0 0 140px;
    display: flex;
    flex-direction: column;
    gap: 7px;
    cursor: pointer;
    text-decoration: none;
    color: inherit;
  }

  .ab-cover-wrap {
    position: relative;
    width: 140px;
    height: 140px;
    border-radius: 8px;
    overflow: hidden;
    background: var(--bg-elevated);
    flex-shrink: 0;
  }

  .ab-cover {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.25s;
  }
  .ab-card:hover .ab-cover { transform: scale(1.03); }

  .ab-placeholder {
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    opacity: 0.35;
  }

  .ab-progress-strip {
    position: absolute;
    bottom: 0;
    left: 0;
    right: 0;
    height: 3px;
    background: rgba(255,255,255,0.15);
  }
  .ab-progress-fill {
    height: 100%;
    background: var(--accent);
    border-radius: 0 2px 2px 0;
  }

  .ab-play-btn {
    position: absolute;
    bottom: 8px;
    right: 8px;
    width: 30px;
    height: 30px;
    border-radius: 50%;
    background: var(--accent);
    border: none;
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    opacity: 0;
    transform: translateY(3px);
    transition: opacity 0.2s, transform 0.2s;
    box-shadow: 0 2px 8px rgba(0,0,0,0.4);
  }
  .ab-card:hover .ab-play-btn { opacity: 1; transform: translateY(0); }
  .ab-play-btn:hover { filter: brightness(1.1); }

  .ab-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .ab-title {
    font-size: 0.8rem;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--text);
    max-width: 140px;
  }
  .ab-author {
    font-size: 0.72rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 140px;
  }

  .album-slider {
    display: flex;
    gap: 16px;
    overflow-x: auto;
    padding-bottom: 8px;
    scrollbar-width: thin;
    scrollbar-color: var(--border) transparent;
  }
  .album-slider::-webkit-scrollbar { height: 4px; }
  .album-slider::-webkit-scrollbar-track { background: transparent; }
  .album-slider::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }

  .slider-item {
    flex: 0 0 160px;
    min-width: 160px;
    max-width: 160px;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  .slider-item :global(.album-card) { width: 160px; max-width: 160px; box-sizing: border-box; }
  .slider-item :global(.cover-wrap) { width: 134px; height: 134px; padding-bottom: 0; }
</style>
