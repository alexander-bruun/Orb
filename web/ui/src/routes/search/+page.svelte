<script lang="ts">
  import { onMount } from 'svelte';
  import { searchQuery, searchResults } from '$lib/stores/library';
  import { library as libApi } from '$lib/api/library';
  import TrackList from '$lib/components/library/TrackList.svelte';
  import AlbumGrid from '$lib/components/library/AlbumGrid.svelte';

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
</script>

<div class="search-page">
  {#if loading}
    <p class="muted">Searching…</p>
  {:else if $searchResults}
    {#if $searchResults?.tracks?.length}
      <section>
        <h2 class="section-title">Tracks</h2>
        <TrackList tracks={$searchResults.tracks} />
      </section>
    {/if}
    {#if $searchResults?.albums?.length}
      <section>
        <h2 class="section-title">Albums</h2>
        <AlbumGrid albums={$searchResults.albums} />
      </section>
    {/if}
    {#if !($searchResults?.tracks?.length) && !($searchResults?.albums?.length) && !($searchResults?.artists?.length)}
      <p class="muted">No results for "{$searchQuery}"</p>
    {/if}
  {:else}
    <p class="muted">Type to search your library</p>
  {/if}
</div>

<style>
  .section-title { font-size: 1rem; font-weight: 600; margin-bottom: 12px; color: var(--text-muted); }
  section { margin-bottom: 32px; }
  .muted { color: var(--text-muted); font-size: 0.875rem; }
</style>
