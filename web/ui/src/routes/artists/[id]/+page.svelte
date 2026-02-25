<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { library as libApi } from '$lib/api/library';
  import AlbumGrid from '$lib/components/library/AlbumGrid.svelte';
  import type { Artist, Album } from '$lib/types';

  let artist: Artist | null = null;
  let albums: Album[] = [];
  let loading = true;

  onMount(async () => {
    const id = $page.params.id;
    try {
      const res = await libApi.artist(id);
      artist = res.artist;
      albums = res.albums;
    } finally {
      loading = false;
    }
  });
</script>

{#if loading}
  <p class="muted">Loading…</p>
{:else if artist}
  <div class="header">
    <h1 class="title">{artist.name}</h1>
  </div>
  <h2 class="section">Albums</h2>
  <AlbumGrid {albums} />
{/if}

<style>
  .header { margin-bottom: 32px; }
  .title { font-size: 2.5rem; font-weight: 700; }
  .section { font-size: 1rem; font-weight: 600; color: var(--text-muted); margin-bottom: 16px; }
  .muted { color: var(--text-muted); }
</style>
