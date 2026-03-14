<script lang="ts">
  import { onMount } from 'svelte';
  import TrackList from '$lib/components/library/TrackList.svelte';
  import Skeleton from '$lib/components/ui/Skeleton.svelte';
  import { playTrack, shuffle } from '$lib/stores/player';
  import { downloadFavorites, downloads } from '$lib/stores/offline/downloads';
  import { isOffline } from '$lib/stores/offline/connectivity';
  import { favorites, favoriteTracks } from '$lib/stores/library/favorites';
  import type { Track } from '$lib/types';

  let loading = true;
  $: tracks = $favoriteTracks ?? [];

  // Derive playable Track stubs from the downloads store for offline mode.
  // The player streams via /api/stream/{id}, which the service worker serves
  // from IndexedDB when the track is downloaded — so only `id` is required.
  $: offlineTracks = [...$downloads.values()]
    .filter(e => e.status === 'done')
    .map(e => ({
      id: e.trackId,
      title: e.title,
      artist_name: e.artistName,
      album_name: e.albumName,
      album_id: e.albumId,
      disc_number: 0,
      duration_ms: 0,
      file_key: '',
      file_size: e.sizeBytes,
      format: 'flac' as const,
      sample_rate: 44100,
      channels: 2,
    } satisfies Track));

  $: displayTracks = $isOffline ? offlineTracks : tracks;

  onMount(async () => {
    // When offline, show downloaded tracks immediately — no API call needed.
    if ($isOffline) {
      loading = false;
      return;
    }
    // If the store is already populated (e.g. navigated back), skip fetch.
    if ($favoriteTracks !== null) {
      loading = false;
      return;
    }
    try {
      await favorites.loadTracks();
    } finally {
      loading = false;
    }
  });

  function playAll() {
    if (displayTracks.length > 0) playTrack(displayTracks[0], displayTracks);
  }

  function shuffleAll() {
    if (displayTracks.length === 0) return;
    shuffle.set(true);
    playTrack(
      displayTracks[Math.floor(Math.random() * displayTracks.length)],
      displayTracks,
    );
  }

  let downloading = false;
  $: dlDoneCount = tracks.filter(t => $downloads.get(t.id)?.status === 'done').length;
  $: allDownloaded = tracks.length > 0 && dlDoneCount === tracks.length;
  $: dlActiveCount = tracks.filter(t => $downloads.get(t.id)?.status === 'downloading').length;

  async function downloadAll() {
    if (downloading || tracks.length === 0) return;
    downloading = true;
    try {
      await downloadFavorites(tracks);
    } finally {
      downloading = false;
    }
  }
</script>

<div class="favorites-page">
  <div class="header">
    <h2 class="title">Favorites</h2>

    {#if $isOffline}
      <span class="offline-badge" title="Showing downloaded tracks">
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
    {/if}

    {#if !loading && displayTracks.length > 0}
      <div class="actions">
        <button class="btn-play" on:click={playAll}>▶ Play</button>
        <button class="btn-shuffle" on:click={shuffleAll} title="Shuffle">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/>
            <polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/>
            <line x1="4" y1="4" x2="9" y2="9"/>
          </svg>
          Shuffle
        </button>
        {#if !$isOffline}
          <button
            class="btn-download"
            on:click={downloadAll}
            disabled={tracks.length === 0 || allDownloaded || downloading}
            title="Download all tracks for offline playback"
          >
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>
            </svg>
            {#if allDownloaded}Downloaded{:else if downloading || dlActiveCount > 0}{dlDoneCount}/{tracks.length}{:else}Download{/if}
          </button>
        {/if}
      </div>
    {/if}
  </div>

  {#if loading}
    <!-- Skeleton rows while loading -->
    <div class="skeleton-list" aria-label="Loading favorites…">
      {#each { length: 8 } as _}
        <div class="skeleton-row">
          <Skeleton width="36px" height="36px" radius="4px" />
          <div class="skeleton-text">
            <Skeleton width="55%" height="0.85rem" />
            <Skeleton width="35%" height="0.75rem" />
          </div>
          <Skeleton width="40px" height="0.75rem" class="skeleton-dur" />
        </div>
      {/each}
    </div>
  {:else if $isOffline && offlineTracks.length === 0}
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
  {:else if !$isOffline && tracks.length === 0}
    <p class="muted">No favorites yet — right-click a track to add one.</p>
  {:else}
    <TrackList tracks={displayTracks} showCover={true} />
  {/if}
</div>

<style>
  .header {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 20px;
    flex-wrap: wrap;
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

  .actions { display: flex; gap: 8px; align-items: center; margin-left: auto; }

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

  .btn-download {
    display: flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 7px 16px;
    color: var(--text-muted);
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-download:hover { color: var(--text); border-color: var(--text); }
  .btn-download:disabled { opacity: 0.6; cursor: not-allowed; }

  /* ── Skeleton rows ── */
  .skeleton-list { display: flex; flex-direction: column; gap: 2px; }
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
  :global(.skeleton-dur) { margin-left: auto; }

  /* ── Offline empty state ── */
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

  .muted { color: var(--text-muted); font-size: 0.875rem; }
</style>
