<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { library as libApi } from '$lib/api/library';
  import TrackList from '$lib/components/library/TrackList.svelte';
  import type { Album, Track, Genre } from '$lib/types';
  import { playTrack, shuffle } from '$lib/stores/player';

  const BASE = import.meta.env.VITE_API_BASE ?? '/api';

  let album: Album | null = null;
  let tracks: Track[] = [];
  let genres: Genre[] = [];
  let artistName: string | null = null;
  let artistId: string | null = null;
  let loading = true;

  onMount(async () => {
      const id = $page.params.id;
      if (!id) {
        loading = false;
        return;
      }
      try {
        const res = await libApi.album(id);
        album = res.album;
        tracks = res.tracks;
        genres = res.genres ?? [];
        if (res.artist) {
          artistName = res.artist.name;
          artistId = res.artist.id;
        }
      } finally {
        loading = false;
      }
  });

  function playAll() {
    if (tracks.length > 0) playTrack(tracks[0], tracks);
  }

  function shuffleAll() {
    if (tracks.length === 0) return;
    shuffle.set(true);
    const idx = Math.floor(Math.random() * tracks.length);
    playTrack(tracks[idx], tracks);
  }
</script>

{#if loading}
  <p class="muted">Loading…</p>
{:else if album}
  <div class="header">
    {#if album.cover_art_key}
      <img src="{BASE}/covers/{album.id}" alt={album.title} class="cover" />
    {:else}
      <div class="cover album-fallback">♪</div>
    {/if}
    <div class="meta">
      <p class="type">{album.album_type ?? 'Album'}</p>
      <h1 class="title">{album.title}</h1>
      {#if artistName}
        {#if artistId}
          <a href="/artists/{artistId}" class="artist">{artistName}</a>
        {:else}
          <p class="artist">{artistName}</p>
        {/if}
      {/if}
      <div class="meta-row">
        {#if album.release_year}
          <span class="year">{album.release_year}</span>
        {/if}
        {#if album.label}
          <span class="label-info">{album.label}</span>
        {/if}
      </div>
      {#if genres.length > 0}
        <div class="genre-pills">
          {#each genres as genre}
            <a href="/genres/{genre.id}" class="genre-pill">{genre.name}</a>
          {/each}
        </div>
      {/if}
      <div class="actions">
        <button class="btn-play" on:click={playAll} disabled={tracks.length === 0}>▶ Play</button>
        <button class="btn-shuffle" on:click={shuffleAll} disabled={tracks.length === 0} title="Shuffle">
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
  <TrackList {tracks} />
{/if}

<style>
  .header { display: flex; gap: 24px; align-items: flex-end; margin-bottom: 32px; }
  .cover {
    width: 180px;
    height: 180px;
    object-fit: cover;
    border-radius: 8px;
    flex-shrink: 0;
  }
  .album-fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 4.5rem;
    color: var(--text-muted);
    background: var(--bg-hover);
    user-select: none;
  }
  .meta { display: flex; flex-direction: column; gap: 4px; }
  .type { font-size: 0.75rem; text-transform: uppercase; color: var(--text-muted); }
  .title { font-size: 2rem; font-weight: 700; margin: 0; }
  .artist { color: var(--text-muted); font-size: 0.9rem; font-weight: 600; text-decoration: none; }
  a.artist:hover { text-decoration: underline; color: var(--text); }
  .meta-row { display: flex; gap: 10px; align-items: center; }
  .year { color: var(--text-muted); font-size: 0.875rem; }
  .label-info { color: var(--text-muted); font-size: 0.875rem; }
  .label-info::before { content: '·'; margin-right: 10px; }
  .genre-pills { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; }
  .genre-pill {
    display: inline-block;
    padding: 3px 10px;
    border-radius: 20px;
    border: 1px solid var(--border);
    color: var(--text-muted);
    font-size: 0.7rem;
    font-weight: 500;
    text-decoration: none;
    transition: color 0.15s, border-color 0.15s;
  }
  .genre-pill:hover { color: var(--text); border-color: var(--accent); }
  .actions { display: flex; gap: 8px; margin-top: 8px; align-items: center; }
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
