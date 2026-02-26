<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { library as libApi } from '$lib/api/library';
  import AlbumGrid from '$lib/components/library/AlbumGrid.svelte';
  import type { Artist, Album } from '$lib/types';
  import { playTrack, shuffle } from '$lib/stores/player';

  let artist: Artist | null = null;
  let albums: Album[] = [];
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
</script>

{#if loading}
  <p class="muted">Loading…</p>
{:else if artist}
  <div class="header">
    <h1 class="title">{artist.name}</h1>
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
{/if}

<style>
  .header { display: flex; align-items: center; gap: 20px; margin-bottom: 32px; }
  .title { font-size: 2.5rem; font-weight: 700; }
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
  }
  .btn-shuffle:hover { color: var(--text); border-color: var(--text); }
  .btn-shuffle:disabled { opacity: 0.6; cursor: not-allowed; }
  .section { font-size: 1rem; font-weight: 600; color: var(--text-muted); margin-bottom: 16px; }
  .muted { color: var(--text-muted); }
</style>
