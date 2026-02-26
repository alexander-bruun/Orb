<script lang="ts">
  import type { Album } from '$lib/types';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { getArtistName } from '$lib/stores/artists';

  export let album: Album;

  let artistName: string = '';

  const BASE = import.meta.env.VITE_API_BASE ?? '/api';

  onMount(async () => {
    if (album.artist_name) {
      artistName = album.artist_name;
      return;
    }
    if (album.artist_id) {
      artistName = await getArtistName(album.artist_id);
    }
  });
</script>

<button class="album-card" on:click={() => goto(`/library/albums/${album.id}`)}>
  <div class="cover-wrap">
    {#if album.cover_art_key}
      <img src="{BASE}/covers/{album.id}" alt={album.title} class="cover" />
    {:else}
      <div class="cover placeholder album-fallback">♪</div>
    {/if}
    {#if album.track_count === 1}
      <span class="badge-single">Single</span>
    {/if}
  </div>
  <div class="info">
    <span class="title">{album.title}</span>
    <div class="meta">
      {#if artistName}<span class="artist">{artistName}</span>{/if}
      {#if album.release_year}<span class="year">{album.release_year}</span>{/if}
    </div>
  </div>
</button>

<style>
  .album-card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: 100%;
    max-width: 240px;
    box-sizing: border-box;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px;
    cursor: pointer;
    text-align: left;
    transition: background 0.15s, border-color 0.15s;
  }
  .album-card:hover { background: var(--bg-hover); border-color: var(--accent); }
  .cover-wrap {
    position: relative;
    width: 100%;
    height: 0;
    padding-bottom: 100%;
    overflow: hidden;
    border-radius: 4px;
  }
  .cover {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .placeholder {
    position: absolute;
    inset: 0;
    background: var(--bg-hover);
  }
  .album-fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 2.5rem;
    color: var(--text-muted);
    user-select: none;
  }
  .badge-single {
    position: absolute;
    top: 6px;
    right: 6px;
    background: var(--accent);
    color: var(--bg);
    font-size: 0.625rem;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    padding: 2px 6px;
    border-radius: 3px;
    pointer-events: none;
  }
  .info { display: flex; flex-direction: column; gap: 2px; }
  .title { font-size: 0.875rem; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--text); }
  .meta { display: flex; align-items: baseline; justify-content: space-between; gap: 4px; min-width: 0; }
  .artist { font-size: 0.75rem; color: var(--text-muted); flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .year { font-size: 0.75rem; color: var(--text-muted); flex-shrink: 0; }
</style>
