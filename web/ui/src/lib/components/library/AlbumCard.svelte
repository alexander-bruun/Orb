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
  {#if album.cover_art_key}
    <img src="{BASE}/covers/{album.id}" alt={album.title} class="cover" />
  {:else}
    <div class="cover placeholder" />
  {/if}
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
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px;
    cursor: pointer;
    text-align: left;
    transition: background 0.15s, border-color 0.15s;
  }
  .album-card:hover { background: var(--bg-hover); border-color: var(--accent); }
  .cover {
    width: 100%;
    aspect-ratio: 1;
    object-fit: cover;
    border-radius: 4px;
  }
  .placeholder { width: 100%; aspect-ratio: 1; background: var(--bg-hover); border-radius: 4px; }
  .info { display: flex; flex-direction: column; gap: 2px; }
  .title { font-size: 0.875rem; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--text); }
  .meta { display: flex; align-items: baseline; justify-content: space-between; gap: 4px; min-width: 0; }
  .artist { font-size: 0.75rem; color: var(--text-muted); flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .year { font-size: 0.75rem; color: var(--text-muted); flex-shrink: 0; }
</style>
