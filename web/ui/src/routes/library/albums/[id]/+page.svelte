<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { library as libApi } from '$lib/api/library';
  import TrackList from '$lib/components/library/TrackList.svelte';
  import type { Album, Track } from '$lib/types';

  const BASE = import.meta.env.VITE_API_BASE ?? '/api';

  let album: Album | null = null;
  let tracks: Track[] = [];
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
        if (res.artist) {
          artistName = res.artist.name;
          artistId = res.artist.id;
        }
      } finally {
        loading = false;
      }
  });
</script>

{#if loading}
  <p class="muted">Loading…</p>
{:else if album}
  <div class="header">
    <img src="{BASE}/covers/{album.id}" alt={album.title} class="cover" />
    <div class="meta">
      <p class="type">Album</p>
      <h1 class="title">{album.title}</h1>
      {#if artistName}
        {#if artistId}
          <a href="/artists/{artistId}" class="artist">{artistName}</a>
        {:else}
          <p class="artist">{artistName}</p>
        {/if}
      {/if}
      {#if album.release_year}
        <p class="year">{album.release_year}</p>
      {/if}
    </div>
  </div>
  <TrackList {tracks} />
{/if}

<style>
  .header { display: flex; gap: 24px; align-items: flex-end; margin-bottom: 32px; }
  .cover { width: 180px; height: 180px; object-fit: cover; border-radius: 8px; flex-shrink: 0; }
  .meta { display: flex; flex-direction: column; gap: 4px; }
  .type { font-size: 0.75rem; text-transform: uppercase; color: var(--text-muted); }
  .title { font-size: 2rem; font-weight: 700; margin: 0; }
  .artist { color: var(--text-muted); font-size: 0.9rem; font-weight: 600; text-decoration: none; }
  a.artist:hover { text-decoration: underline; color: var(--text); }
  .year { color: var(--text-muted); font-size: 0.875rem; }
  .muted { color: var(--text-muted); }
</style>
