<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { playlists as playlistApi } from '$lib/api/playlists';
  import { getPlaylistCoverGrid } from '$lib/api/playlists';
  import TrackList from '$lib/components/library/TrackList.svelte';
  import type { Playlist, Track } from '$lib/types';
  import { playTrack, shuffle } from '$lib/stores/player';

  let playlist: Playlist | null = null;
  let tracks: Track[] = [];

  let coverGrid: string[] = [];
  let loading = true;

  onMount(async () => {
    const id = String($page.params.id);
    try {
      const res = await playlistApi.get(id);
      playlist = res.playlist;
      tracks = res.tracks;
      // Fetch cover grid
      try {
        coverGrid = await getPlaylistCoverGrid(id);
      } catch (e) {
        coverGrid = [];
      }
    } finally {
      loading = false;
    }
  });

  function playAll() {
    if ((tracks?.length ?? 0) > 0) playTrack(tracks[0], tracks);
  }

  function shuffleAll() {
    if ((tracks?.length ?? 0) === 0) return;
    shuffle.set(true);
    const idx = Math.floor(Math.random() * tracks.length);
    playTrack(tracks[idx], tracks);
  }
</script>

{#if loading}
  <p class="muted">Loading…</p>
{:else if playlist}
  <div class="header">
    <div class="cover-placeholder cover-grid">
      {#if coverGrid.length > 0}
        <div class="grid">
          {#each Array(4) as _, i}
            {#if coverGrid[i]}
              <img src={coverGrid[i]} alt="cover" class="grid-img" />
            {:else}
              <span class="grid-fallback">♪</span>
            {/if}
          {/each}
        </div>
      {:else}
        <span class="grid-fallback">♪</span>
      {/if}
    </div>
    <div class="meta">
      <p class="type">Playlist</p>
      <h1 class="title">{playlist.name}</h1>
      {#if playlist.description}
        <p class="desc">{playlist.description}</p>
      {/if}
      <div class="actions">
        <button class="btn-play" on:click={playAll} disabled={(tracks?.length ?? 0) === 0}>▶ Play</button>
        <button class="btn-shuffle" on:click={shuffleAll} disabled={(tracks?.length ?? 0) === 0} title="Shuffle">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/>
            <polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/>
            <line x1="4" y1="4" x2="9" y2="9"/>
          </svg>
          Shuffle
        </button>
      </div>
    </div>
  </div>
  <TrackList {tracks} showCover={true} />
{/if}

<style>
  .cover-grid {
    position: relative;
    width: 180px;
    height: 180px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg-hover);
    border-radius: 8px;
    overflow: hidden;
  }
  .cover-grid .grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    grid-template-rows: 1fr 1fr;
    width: 100%;
    height: 100%;
    gap: 0;
  }
  .cover-grid .grid-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    border-radius: 0;
    display: block;
  }
  .cover-grid .grid-fallback {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 2.5rem;
    color: var(--text-muted);
    background: var(--bg-hover);
  }
  .header { display: flex; gap: 24px; align-items: flex-end; margin-bottom: 32px; }
  .cover-placeholder {
    width: 180px; height: 180px;
    background: var(--bg-hover);
    border-radius: 8px;
    display: flex; align-items: center; justify-content: center;
    font-size: 3rem; color: var(--text-muted);
    flex-shrink: 0;
  }
  .meta { display: flex; flex-direction: column; gap: 6px; }
  .type { font-size: 0.75rem; text-transform: uppercase; color: var(--text-muted); }
  .title { font-size: 2rem; font-weight: 700; margin: 0; }
  .desc { color: var(--text-muted); font-size: 0.875rem; }
  .actions { display: flex; gap: 8px; margin-top: 4px; align-items: center; }
  .btn-play {
    background: var(--accent);
    border: none;
    border-radius: 20px;
    padding: 8px 20px;
    color: #fff;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-play:hover { background: var(--accent-hover); }
  .btn-play:disabled { opacity: 0.6; cursor: not-allowed; }
  .btn-shuffle {
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
  .btn-shuffle:hover { color: var(--text); border-color: var(--text); }
  .btn-shuffle:disabled { opacity: 0.6; cursor: not-allowed; }
  .muted { color: var(--text-muted); }
</style>
