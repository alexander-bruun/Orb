<script lang="ts">
  import { onMount } from 'svelte';
  import { searchQuery, searchResults } from '$lib/stores/library';
  import { library as libApi } from '$lib/api/library';
  import TrackList from '$lib/components/library/TrackList.svelte';
  import AlbumCard from '$lib/components/library/AlbumCard.svelte';
  import ArtistList from '$lib/components/library/ArtistList.svelte';

  let loading = false;

  async function doSearch(q: string) {
    if (!q) return;
    loading = true;
    try {
      const res = await libApi.search(q);
      searchResults.set(res);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    const unsubscribe = searchQuery.subscribe((q) => {
      if (q) doSearch(q);
    });
    return unsubscribe;
  });

  $: hasResults =
    $searchResults.tracks.length > 0 ||
    $searchResults.albums.length > 0 ||
    $searchResults.artists.length > 0;
</script>

<div class="search-page">
  {#if loading}
    <p class="muted">Searching…</p>
  {:else if $searchQuery}
    {#if hasResults}
      {#if $searchResults.artists.length}
        <section>
          <h2 class="section-title">
            Artists
            <span class="count">{$searchResults.artists.length}</span>
          </h2>
          <ArtistList artists={$searchResults.artists} />
        </section>
      {/if}

      {#if $searchResults.albums.length}
        <section>
          <h2 class="section-title">
            Albums
            <span class="count">{$searchResults.albums.length}</span>
          </h2>
          <div class="album-grid">
            {#each $searchResults.albums as album (album.id)}
              <AlbumCard {album} />
            {/each}
          </div>
        </section>
      {/if}

      {#if $searchResults.tracks.length}
        <section>
          <h2 class="section-title">
            Tracks
            <span class="count">{$searchResults.tracks.length}</span>
          </h2>
          <TrackList tracks={$searchResults.tracks} />
        </section>
      {/if}
    {:else}
      <p class="muted">No results for "<span class="query">{$searchQuery}</span>"</p>
    {/if}
  {:else}
    <p class="muted">Type to search your library</p>
  {/if}
</div>

<style>
  .section-title {
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-muted);
    margin-bottom: 12px;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .count {
    font-size: 0.6875rem;
    color: var(--accent);
    background: var(--accent-dim);
    border: 1px solid var(--accent-glow);
    border-radius: 4px;
    padding: 1px 6px;
    letter-spacing: 0;
  }

  section {
    margin-bottom: 36px;
  }

  .album-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 16px;
  }

  .muted {
    color: var(--text-muted);
    font-size: 0.875rem;
  }

  .query {
    color: var(--text);
  }
</style>
