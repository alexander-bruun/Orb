<script lang="ts">
  import { onMount } from 'svelte';
  import { library as libApi } from '$lib/api/library';
  import type { Track, Album } from '$lib/types';
  import TrackList from '$lib/components/library/TrackList.svelte';
  import AlbumCard from '$lib/components/library/AlbumCard.svelte';

  const PAGE_SIZE = 10;

  type Interval = 'today' | 'week' | 'month' | 'all' | 'custom';

  let interval: Interval = 'all';
  let customFrom = '';
  let customTo = '';

  let recentTracks: Track[] = [];
  let mostTracks: Track[] = [];
  let recentAlbums: Album[] = [];
  let newAlbums: Album[] = [];
  let loading = true;
  let playsLoading = false;

  let recentPage = 1;
  let mostPage = 1;

  $: recentPages = Math.max(1, Math.ceil(recentTracks.length / PAGE_SIZE));
  $: mostPages = Math.max(1, Math.ceil(mostTracks.length / PAGE_SIZE));
  $: pagedRecent = recentTracks.slice((recentPage - 1) * PAGE_SIZE, recentPage * PAGE_SIZE);
  $: pagedMost = mostTracks.slice((mostPage - 1) * PAGE_SIZE, mostPage * PAGE_SIZE);

  const INTERVALS: { key: Interval; label: string }[] = [
    { key: 'today', label: 'Today' },
    { key: 'week', label: 'Week' },
    { key: 'month', label: 'Month' },
    { key: 'all', label: 'All time' },
    { key: 'custom', label: 'Custom' }
  ];

  function getDateRange(): { from?: string; to?: string } {
    const now = new Date();
    switch (interval) {
      case 'today': {
        const from = new Date(now);
        from.setHours(0, 0, 0, 0);
        return { from: from.toISOString() };
      }
      case 'week': {
        const from = new Date(now);
        from.setDate(from.getDate() - 7);
        return { from: from.toISOString() };
      }
      case 'month': {
        const from = new Date(now);
        from.setDate(from.getDate() - 30);
        return { from: from.toISOString() };
      }
      case 'custom': {
        const range: { from?: string; to?: string } = {};
        if (customFrom) range.from = new Date(customFrom).toISOString();
        if (customTo) {
          const to = new Date(customTo);
          to.setDate(to.getDate() + 1);
          range.to = to.toISOString();
        }
        return range;
      }
      default:
        return {};
    }
  }

  async function loadPlays() {
    playsLoading = true;
    const { from, to } = getDateRange();
    try {
      [recentTracks, mostTracks] = await Promise.all([
        libApi.recentlyPlayed(100, from, to).then((r) => r ?? []),
        libApi.mostPlayed(100, from, to).then((r) => r ?? [])
      ]);
      recentPage = 1;
      mostPage = 1;
    } catch {
      // ignore
    } finally {
      playsLoading = false;
    }
  }

  function selectInterval(iv: Interval) {
    interval = iv;
    if (iv === 'custom') {
      if (customFrom) loadPlays();
    } else {
      loadPlays();
    }
  }

  function handleCustomDateChange() {
    loadPlays();
  }

  onMount(async () => {
    try {
      [recentTracks, mostTracks, recentAlbums, newAlbums] = await Promise.all([
        libApi.recentlyPlayed(100).then((r) => r ?? []),
        libApi.mostPlayed(100).then((r) => r ?? []),
        libApi.recentlyPlayedAlbums().then((r) => r ?? []),
        libApi.recentlyAddedAlbums(20).then((r) => r ?? [])
      ]);
    } catch {
      // ignore — user may not be logged in
    } finally {
      loading = false;
    }
  });
</script>

{#if loading}
  <p class="muted">Loading…</p>
{:else}
  {#if recentTracks.length > 0 || mostTracks.length > 0}
    <section class="home-section">
      <div class="plays-controls">
        <div class="interval-tabs">
          {#each INTERVALS as iv}
            <button
              class="interval-tab"
              class:active={interval === iv.key}
              on:click={() => selectInterval(iv.key)}
            >
              {iv.label}
            </button>
          {/each}
        </div>
        {#if interval === 'custom'}
          <div class="date-range">
            <input
              type="date"
              class="date-input"
              bind:value={customFrom}
              on:change={handleCustomDateChange}
            />
            <span class="date-sep">–</span>
            <input
              type="date"
              class="date-input"
              bind:value={customTo}
              on:change={handleCustomDateChange}
            />
          </div>
        {/if}
      </div>

      <div class="plays-columns">
        <div class="plays-col">
          <div class="col-header">
            <h2 class="col-title">Recently Played</h2>
            {#if recentPages > 1}
              <label class="page-label">
                Page
                <select class="page-select" bind:value={recentPage}>
                  {#each Array.from({ length: recentPages }, (_, i) => i + 1) as p}
                    <option value={p}>{p} / {recentPages}</option>
                  {/each}
                </select>
              </label>
            {/if}
          </div>
          {#if playsLoading}
            <p class="muted">Loading…</p>
          {:else if pagedRecent.length > 0}
            <TrackList tracks={pagedRecent} showCover />
          {:else}
            <p class="muted">No plays in this period.</p>
          {/if}
        </div>

        <div class="plays-col">
          <div class="col-header">
            <h2 class="col-title">Most Played</h2>
            {#if mostPages > 1}
              <label class="page-label">
                Page
                <select class="page-select" bind:value={mostPage}>
                  {#each Array.from({ length: mostPages }, (_, i) => i + 1) as p}
                    <option value={p}>{p} / {mostPages}</option>
                  {/each}
                </select>
              </label>
            {/if}
          </div>
          {#if playsLoading}
            <p class="muted">Loading…</p>
          {:else if pagedMost.length > 0}
            <TrackList tracks={pagedMost} showCover />
          {:else}
            <p class="muted">No plays in this period.</p>
          {/if}
        </div>
      </div>
    </section>
  {/if}

  {#if recentAlbums.length > 0}
    <section class="home-section">
      <h2 class="section-title" style="margin-bottom: 16px;">Recently Played Albums</h2>
      <div class="album-slider">
        {#each recentAlbums as album (album.id)}
          <div class="slider-item">
            <AlbumCard {album} />
          </div>
        {/each}
      </div>
    </section>
  {/if}

  {#if newAlbums.length > 0}
    <section class="home-section">
      <div class="section-header">
        <h2 class="section-title">Recently Added</h2>
        <a href="/library" class="view-all">View all</a>
      </div>
      <div class="album-slider">
        {#each newAlbums as album (album.id)}
          <div class="slider-item">
            <AlbumCard {album} />
          </div>
        {/each}
      </div>
    </section>
  {/if}

  {#if recentTracks.length === 0 && mostTracks.length === 0 && recentAlbums.length === 0 && newAlbums.length === 0}
    <p class="muted">Nothing here yet. Go find some music!</p>
  {/if}
{/if}

<style>
  .home-section { margin-bottom: 40px; }

  .section-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 16px;
  }

  .section-title { font-size: 1.125rem; font-weight: 600; margin: 0; }

  .view-all {
    font-size: 0.8rem;
    color: var(--text-muted);
    text-decoration: none;
    letter-spacing: 0.02em;
  }
  .view-all:hover { color: var(--text); }

  /* --- interval filter --- */
  .plays-controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    margin-bottom: 20px;
  }

  .interval-tabs {
    display: flex;
    gap: 4px;
  }

  .interval-tab {
    background: none;
    border: 1px solid var(--border);
    border-radius: 20px;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 0.8rem;
    padding: 4px 12px;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
  }
  .interval-tab:hover { color: var(--text); border-color: var(--text-muted); }
  .interval-tab.active {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
  }

  .date-range {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .date-input {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text);
    font-size: 0.8rem;
    padding: 3px 8px;
    cursor: pointer;
  }
  .date-input:focus { outline: none; border-color: var(--accent); }

  .date-sep { color: var(--text-muted); font-size: 0.8rem; }

  /* --- 2-column layout --- */
  .plays-columns {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 32px;
  }

  @media (max-width: 640px) {
    .plays-columns { grid-template-columns: 1fr; }
  }

  .col-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 12px;
  }

  .col-title { font-size: 1.125rem; font-weight: 600; margin: 0; }

  /* --- pagination --- */
  .page-label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.8rem;
    color: var(--text-muted);
  }

  .page-select {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text-muted);
    font-size: 0.8rem;
    padding: 2px 6px;
    cursor: pointer;
  }
  .page-select:focus { outline: none; border-color: var(--accent); }

  .muted { color: var(--text-muted); font-size: 0.875rem; }

  /* --- album slider --- */
  .album-slider {
    display: flex;
    gap: 16px;
    overflow-x: auto;
    padding-bottom: 8px;
    scrollbar-width: thin;
    scrollbar-color: var(--border) transparent;
  }
  .album-slider::-webkit-scrollbar { height: 4px; }
  .album-slider::-webkit-scrollbar-track { background: transparent; }
  .album-slider::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }

  .slider-item {
    flex: 0 0 160px;
    min-width: 160px;
    max-width: 160px;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  .slider-item :global(.album-card) {
    width: 160px;
    max-width: 160px;
    box-sizing: border-box;
  }
  .slider-item :global(.cover-wrap) {
    width: 134px;
    height: 134px;
    padding-bottom: 0;
  }
</style>
