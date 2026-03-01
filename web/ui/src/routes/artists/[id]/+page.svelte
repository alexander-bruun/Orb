<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { library as libApi } from '$lib/api/library';
  import AlbumGrid from '$lib/components/library/AlbumGrid.svelte';
  import type { Artist, Album, Genre, RelatedArtist } from '$lib/types';
  import { playTrack, shuffle } from '$lib/stores/player';

  import { getApiBase } from '$lib/api/base';

  let artist: Artist | null = null;
  let albums: Album[] = [];
  let genres: Genre[] = [];
  let relatedArtists: RelatedArtist[] = [];
  let grouped: Map<string, Album[]> = new Map();
  let keys: string[] = [];
  let loading = true;
  let shuffling = false;

  onMount(async () => {
    const id = $page.params.id ?? '';
    try {
      const res = await libApi.artist(id);
      artist = res.artist;
      albums = res.albums;
      genres = res.genres ?? [];
      relatedArtists = res.related_artists ?? [];

      // Group albums by first letter of title
      grouped = new Map();
      for (const album of albums) {
        const key = album.title?.[0]?.toUpperCase() ?? '#';
        if (!grouped.has(key)) grouped.set(key, []);
        grouped.get(key)?.push(album);
      }
      keys = Array.from(grouped.keys()).sort();
    } finally {
      loading = false;
    }
  });

  async function shuffleAll() {
    if (albums.length === 0 || shuffling) return;
    shuffling = true;
    try {
      const results = await Promise.all(albums.map((a) => libApi.album(a.id)));
      const tracks = results.flatMap((r) => r.tracks);
      if (tracks.length === 0) return;
      shuffle.set(true);
      const idx = Math.floor(Math.random() * tracks.length);
      await playTrack(tracks[idx], tracks);
    } finally {
      shuffling = false;
    }
  }

  function formatDates(artist: Artist): string {
    if (!artist.begin_date) return '';
    let s = artist.begin_date.substring(0, 4);
    if (artist.end_date) {
      s += ' – ' + artist.end_date.substring(0, 4);
    } else {
      s += ' – present';
    }
    return s;
  }
</script>

{#if loading}
  <p class="muted">Loading…</p>
{:else if artist}
  <div class="header">
    {#if artist.image_key}
      <img src="{getApiBase()}/covers/artist/{artist.id}" alt={artist.name} class="artist-photo" />
    {/if}
    <div class="header-text">
      <h1 class="title">{artist.name}</h1>
      {#if artist.artist_type || artist.country || artist.begin_date}
        <div class="artist-meta">
          {#if artist.artist_type}
            <span class="meta-item">{artist.artist_type}</span>
          {/if}
          {#if artist.country}
            <span class="meta-item">{artist.country}</span>
          {/if}
          {#if artist.begin_date}
            <span class="meta-item">{formatDates(artist)}</span>
          {/if}
        </div>
      {/if}
      {#if artist.disambiguation}
        <p class="disambiguation">{artist.disambiguation}</p>
      {/if}
      {#if genres.length > 0}
        <div class="genre-pills">
          {#each genres as genre}
            <a href="/genres/{genre.id}" class="genre-pill">{genre.name}</a>
          {/each}
        </div>
      {/if}
    </div>
    <button class="btn-shuffle" on:click={shuffleAll} disabled={albums.length === 0 || shuffling} title="Shuffle all songs">
      <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/>
        <polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/>
        <line x1="4" y1="4" x2="9" y2="9"/>
      </svg>
      {shuffling ? 'Loading…' : 'Shuffle All'}
    </button>
  </div>
  <h2 class="section">Albums</h2>
  <AlbumGrid {grouped} {keys} />

  {#if relatedArtists.length > 0}
    <h2 class="section" style="margin-top: 32px;">Related Artists</h2>
    <div class="related-list">
      {#each relatedArtists as rel}
        <a href="/artists/{rel.related_id}" class="related-artist">
          <span class="related-name">{rel.artist_name}</span>
          <span class="related-type">{rel.rel_type}</span>
        </a>
      {/each}
    </div>
  {/if}
{/if}

<style>
  .header { display: flex; align-items: flex-start; gap: 20px; margin-bottom: 32px; }
  .artist-photo {
    width: 160px;
    height: 160px;
    border-radius: 50%;
    object-fit: cover;
    flex-shrink: 0;
    box-shadow: 0 4px 24px rgba(0,0,0,0.25);
  }
  .header-text { flex: 1; }
  .title { font-size: 2.5rem; font-weight: 700; margin: 0; }
  .artist-meta { display: flex; gap: 12px; align-items: center; margin-top: 6px; }
  .meta-item { font-size: 0.8rem; color: var(--text-muted); }
  .meta-item + .meta-item::before { content: '·'; margin-right: 12px; }
  .disambiguation { font-size: 0.85rem; color: var(--text-muted); font-style: italic; margin: 4px 0 0; }
  .genre-pills { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }
  .genre-pill {
    display: inline-block;
    padding: 4px 12px;
    border-radius: 20px;
    border: 1px solid var(--border);
    color: var(--text-muted);
    font-size: 0.75rem;
    font-weight: 500;
    text-decoration: none;
    transition: color 0.15s, border-color 0.15s;
  }
  .genre-pill:hover { color: var(--text); border-color: var(--accent); }
  .btn-shuffle {
    display: flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 8px 18px;
    color: var(--text-muted);
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    flex-shrink: 0;
    margin-top: 8px;
  }
  .btn-shuffle:hover { color: var(--text); border-color: var(--text); }
  .btn-shuffle:disabled { opacity: 0.6; cursor: not-allowed; }
  .section { font-size: 1rem; font-weight: 600; color: var(--text-muted); margin-bottom: 16px; }
  .muted { color: var(--text-muted); }
  .related-list { display: flex; flex-wrap: wrap; gap: 8px; }
  .related-artist {
    display: flex;
    flex-direction: column;
    padding: 10px 16px;
    border-radius: 8px;
    border: 1px solid var(--border);
    text-decoration: none;
    transition: border-color 0.15s;
  }
  .related-artist:hover { border-color: var(--accent); }
  .related-name { font-size: 0.875rem; font-weight: 600; color: var(--text); }
  .related-type { font-size: 0.7rem; color: var(--text-muted); }
</style>
