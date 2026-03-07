<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { library as libApi } from '$lib/api/library';
  import AlbumCard from '$lib/components/library/AlbumCard.svelte';
  import type { Artist, Album, Genre } from '$lib/types';

  let genre: Genre | null = null;
  let artists: Artist[] = [];
  let albums: Album[] = [];
  let loading = true;

  onMount(async () => {
    const id = $page.params.id ?? '';
    try {
      const [g, a, al] = await Promise.all([
        libApi.genreDetail(id),
        libApi.genreArtists(id, 100),
        libApi.genreAlbums(id, 100)
      ]);
      genre = g;
      artists = a;
      albums = al;
    } catch {
      // genre not found
    } finally {
      loading = false;
    }
  });
</script>

{#if loading}
  <p class="muted">Loading…</p>
{:else if genre}
  <h1 class="title">{genre.name}</h1>

  {#if artists.length > 0}
    <h2 class="section">Artists</h2>
    <div class="artist-list">
      {#each artists as artist}
        <a href="/artists/{artist.id}" class="artist-card">
          {artist.name}
        </a>
      {/each}
    </div>
  {/if}

  {#if albums.length > 0}
    <h2 class="section" style="margin-top: 32px;">Albums</h2>
    <div class="album-grid">
      {#each albums as album}
        <AlbumCard {album} />
      {/each}
    </div>
  {/if}
{:else}
  <p class="muted">Genre not found.</p>
{/if}

<style>
  .title { font-size: 2rem; font-weight: 700; margin-bottom: 24px; text-transform: capitalize; }
  .section { font-size: 1rem; font-weight: 600; color: var(--text-muted); margin-bottom: 16px; }
  .muted { color: var(--text-muted); }
  .artist-list { display: flex; flex-wrap: wrap; gap: 8px; }
  .artist-card {
    padding: 8px 16px;
    border-radius: 8px;
    border: 1px solid var(--border);
    color: var(--text);
    font-size: 0.875rem;
    font-weight: 600;
    text-decoration: none;
    transition: border-color 0.15s;
  }
  .artist-card:hover { border-color: var(--accent); }
  .album-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 16px;
  }
</style>
