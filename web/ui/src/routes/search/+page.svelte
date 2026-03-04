<script lang="ts">
  import { onMount } from 'svelte';
  import { searchQuery, searchResults, searchFilters, savedFilters, saveFilter, deleteSavedFilter } from '$lib/stores/library';
  import { library as libApi } from '$lib/api/library';
  import TrackList from '$lib/components/library/TrackList.svelte';
  import AlbumCard from '$lib/components/library/AlbumCard.svelte';
  import ArtistList from '$lib/components/library/ArtistList.svelte';
  import type { SearchFilters } from '$lib/types';

  let loading = false;
  let filtersOpen = false;
  let saveName = '';
  let showSaveInput = false;

  // Local copies of filter fields for binding
  let genre = '';
  let yearFrom = '';
  let yearTo = '';
  let format = '';
  let bitrateMin = '';
  let bitrateMax = '';
  let sortTracks = '';
  let sortAlbums = '';
  let typesTracks = true;
  let typesAlbums = true;
  let typesArtists = true;

  function buildFilters(): SearchFilters {
    const f: SearchFilters = {};
    if (genre.trim()) f.genre = genre.trim();
    const yf = parseInt(yearFrom); if (!isNaN(yf) && yf > 0) f.year_from = yf;
    const yt = parseInt(yearTo);   if (!isNaN(yt) && yt > 0) f.year_to = yt;
    if (format) f.format = format;
    const bmin = parseInt(bitrateMin); if (!isNaN(bmin) && bmin > 0) f.bitrate_min = bmin;
    const bmax = parseInt(bitrateMax); if (!isNaN(bmax) && bmax > 0) f.bitrate_max = bmax;
    if (sortTracks) f.sort_tracks = sortTracks;
    if (sortAlbums) f.sort_albums = sortAlbums;
    const types: ('tracks' | 'albums' | 'artists')[] = [];
    if (typesTracks) types.push('tracks');
    if (typesAlbums) types.push('albums');
    if (typesArtists) types.push('artists');
    if (types.length < 3) f.types = types;
    return f;
  }

  function syncFromFilters(f: SearchFilters) {
    genre = f.genre ?? '';
    yearFrom = f.year_from ? String(f.year_from) : '';
    yearTo = f.year_to ? String(f.year_to) : '';
    format = f.format ?? '';
    bitrateMin = f.bitrate_min ? String(f.bitrate_min) : '';
    bitrateMax = f.bitrate_max ? String(f.bitrate_max) : '';
    sortTracks = f.sort_tracks ?? '';
    sortAlbums = f.sort_albums ?? '';
    typesTracks = !f.types || f.types.includes('tracks');
    typesAlbums = !f.types || f.types.includes('albums');
    typesArtists = !f.types || f.types.includes('artists');
  }

  function hasActiveFilters(f: SearchFilters): boolean {
    return !!(f.genre || f.year_from || f.year_to || f.format || f.bitrate_min || f.bitrate_max ||
              f.sort_tracks || f.sort_albums || (f.types && f.types.length < 3));
  }

  async function doSearch(q: string, filters: SearchFilters) {
    if (!q) return;
    loading = true;
    try {
      const res = await libApi.search(q, filters);
      searchResults.set(res);
    } finally {
      loading = false;
    }
  }

  function applyFilters() {
    const f = buildFilters();
    searchFilters.set(f);
    if ($searchQuery) doSearch($searchQuery, f);
  }

  function clearFilters() {
    genre = yearFrom = yearTo = format = bitrateMin = bitrateMax = sortTracks = sortAlbums = '';
    typesTracks = typesAlbums = typesArtists = true;
    searchFilters.set({});
    if ($searchQuery) doSearch($searchQuery, {});
  }

  function applySavedFilter(f: SearchFilters) {
    syncFromFilters(f);
    searchFilters.set(f);
    if ($searchQuery) doSearch($searchQuery, f);
  }

  function handleSave() {
    if (!saveName.trim()) return;
    saveFilter(saveName.trim(), buildFilters());
    saveName = '';
    showSaveInput = false;
  }

  onMount(() => {
    // Sync local state from existing store values
    syncFromFilters($searchFilters);

    const unsubQ = searchQuery.subscribe((q) => {
      if (q) doSearch(q, $searchFilters);
    });
    return unsubQ;
  });

  $: hasResults =
    $searchResults.tracks.length > 0 ||
    $searchResults.albums.length > 0 ||
    $searchResults.artists.length > 0;

  $: activeFilters = hasActiveFilters($searchFilters);
</script>

<div class="search-page">
  <!-- Filter panel toggle -->
  <div class="filter-bar">
    <button
      class="filter-toggle"
      class:active={activeFilters}
      on:click={() => (filtersOpen = !filtersOpen)}
      aria-expanded={filtersOpen}
    >
      <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true">
        <line x1="4" y1="6" x2="20" y2="6"/>
        <line x1="8" y1="12" x2="16" y2="12"/>
        <line x1="11" y1="18" x2="13" y2="18"/>
      </svg>
      Filters
      {#if activeFilters}<span class="filter-badge">●</span>{/if}
    </button>

    {#if activeFilters}
      <button class="clear-btn" on:click={clearFilters}>Clear filters</button>
    {/if}

    <!-- Saved filters dropdown -->
    {#if $savedFilters.length > 0}
      <div class="saved-filters">
        <span class="saved-label">Saved:</span>
        {#each $savedFilters as sf (sf.name)}
          <button class="saved-chip" on:click={() => applySavedFilter(sf.filters)}>
            {sf.name}
          </button>
          <button class="saved-del" title="Delete" on:click={() => deleteSavedFilter(sf.name)} aria-label="Delete saved filter {sf.name}">×</button>
        {/each}
      </div>
    {/if}
  </div>

  {#if filtersOpen}
    <div class="filter-panel">
      <div class="filter-grid">
        <!-- Types -->
        <fieldset class="filter-group filter-types">
          <legend class="filter-label">Results</legend>
          <label class="check-label"><input type="checkbox" bind:checked={typesTracks} /> Tracks</label>
          <label class="check-label"><input type="checkbox" bind:checked={typesAlbums} /> Albums</label>
          <label class="check-label"><input type="checkbox" bind:checked={typesArtists} /> Artists</label>
        </fieldset>

        <!-- Genre -->
        <div class="filter-group">
          <label class="filter-label" for="f-genre">Genre</label>
          <input id="f-genre" class="filter-input" type="text" placeholder="e.g. jazz" bind:value={genre} />
        </div>

        <!-- Year range -->
        <div class="filter-group">
          <span class="filter-label">Year</span>
          <div class="range-row">
            <input class="filter-input narrow" type="number" placeholder="from" min="1900" max="2100" bind:value={yearFrom} />
            <span class="range-sep">–</span>
            <input class="filter-input narrow" type="number" placeholder="to"   min="1900" max="2100" bind:value={yearTo} />
          </div>
        </div>

        <!-- Format -->
        <div class="filter-group">
          <label class="filter-label" for="f-format">Format</label>
          <select id="f-format" class="filter-select" bind:value={format}>
            <option value="">Any</option>
            <option value="flac">FLAC</option>
            <option value="mp3">MP3</option>
            <option value="wav">WAV</option>
            <option value="aac">AAC</option>
            <option value="ogg">OGG</option>
          </select>
        </div>

        <!-- Bitrate -->
        <div class="filter-group">
          <span class="filter-label">Bitrate (kbps)</span>
          <div class="range-row">
            <input class="filter-input narrow" type="number" placeholder="min" min="0" bind:value={bitrateMin} />
            <span class="range-sep">–</span>
            <input class="filter-input narrow" type="number" placeholder="max" min="0" bind:value={bitrateMax} />
          </div>
        </div>

        <!-- Sort tracks -->
        <div class="filter-group">
          <label class="filter-label" for="f-sort-tracks">Sort tracks</label>
          <select id="f-sort-tracks" class="filter-select" bind:value={sortTracks}>
            <option value="">Relevance</option>
            <option value="title">Title</option>
            <option value="year">Year</option>
            <option value="bitrate">Bitrate</option>
            <option value="duration">Duration</option>
          </select>
        </div>

        <!-- Sort albums -->
        <div class="filter-group">
          <label class="filter-label" for="f-sort-albums">Sort albums</label>
          <select id="f-sort-albums" class="filter-select" bind:value={sortAlbums}>
            <option value="">Relevance</option>
            <option value="title">Title</option>
            <option value="year">Year</option>
          </select>
        </div>
      </div>

      <div class="filter-actions">
        <button class="btn-primary" on:click={applyFilters}>Apply</button>
        <button class="btn-secondary" on:click={clearFilters}>Clear</button>

        {#if showSaveInput}
          <div class="save-row">
            <input class="filter-input save-input" type="text" placeholder="Filter preset name" bind:value={saveName} />
            <button class="btn-primary" on:click={handleSave}>Save</button>
            <button class="btn-secondary" on:click={() => (showSaveInput = false)}>Cancel</button>
          </div>
        {:else}
          <button class="btn-ghost" on:click={() => (showSaveInput = true)}>Save preset…</button>
        {/if}
      </div>
    </div>
  {/if}

  <!-- Results -->
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
      <p class="muted">No results for "<span class="query">{$searchQuery}</span>"
        {#if activeFilters}— try removing some filters{/if}
      </p>
    {/if}
  {:else}
    <p class="muted">Type to search your library</p>
  {/if}
</div>

<style>
  /* ── filter bar ─────────────────────────────────────────── */
  .filter-bar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 12px;
  }

  .filter-toggle {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text-muted);
    font-size: 0.8125rem;
    cursor: pointer;
    transition: border-color 0.15s, color 0.15s;
  }
  .filter-toggle:hover, .filter-toggle[aria-expanded="true"] {
    border-color: var(--accent);
    color: var(--text);
  }
  .filter-toggle.active {
    border-color: var(--accent);
    color: var(--accent);
  }

  .filter-badge {
    font-size: 0.5rem;
    color: var(--accent);
    line-height: 1;
  }

  .clear-btn {
    font-size: 0.75rem;
    color: var(--text-muted);
    background: none;
    border: none;
    cursor: pointer;
    padding: 4px 6px;
    border-radius: 4px;
  }
  .clear-btn:hover { color: var(--text); }

  .saved-filters {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: wrap;
  }
  .saved-label {
    font-size: 0.75rem;
    color: var(--text-muted);
    margin-right: 2px;
  }
  .saved-chip {
    font-size: 0.75rem;
    padding: 2px 8px;
    border-radius: 12px;
    border: 1px solid var(--border);
    background: var(--accent-dim);
    color: var(--accent);
    cursor: pointer;
    transition: background 0.12s;
  }
  .saved-chip:hover { background: var(--accent-glow); }
  .saved-del {
    font-size: 0.75rem;
    padding: 1px 4px;
    border: none;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    margin-left: -2px;
    line-height: 1;
  }
  .saved-del:hover { color: var(--text); }

  /* ── filter panel ───────────────────────────────────────── */
  .filter-panel {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 16px;
    margin-bottom: 20px;
  }

  .filter-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 12px 16px;
    margin-bottom: 14px;
  }

  .filter-group {
    display: flex;
    flex-direction: column;
    gap: 5px;
  }

  .filter-types {
    border: none;
    padding: 0;
    margin: 0;
  }

  .filter-label {
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-muted);
  }

  .filter-input {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 5px;
    padding: 5px 8px;
    font-size: 0.8125rem;
    color: var(--text);
    width: 100%;
    box-sizing: border-box;
  }
  .filter-input:focus { outline: none; border-color: var(--accent); }

  .filter-select {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 5px;
    padding: 5px 8px;
    font-size: 0.8125rem;
    color: var(--text);
    width: 100%;
  }
  .filter-select:focus { outline: none; border-color: var(--accent); }

  .narrow { width: 70px; flex: 0 0 70px; }

  .range-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .range-sep { color: var(--text-muted); font-size: 0.875rem; }

  .check-label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.8125rem;
    color: var(--text);
    cursor: pointer;
  }

  .filter-actions {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
  }

  .save-row {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }

  .save-input { max-width: 200px; }

  .btn-primary {
    padding: 5px 14px;
    border-radius: 6px;
    border: none;
    background: var(--accent);
    color: #fff;
    font-size: 0.8125rem;
    cursor: pointer;
    transition: opacity 0.12s;
  }
  .btn-primary:hover { opacity: 0.85; }

  .btn-secondary {
    padding: 5px 14px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text-muted);
    font-size: 0.8125rem;
    cursor: pointer;
  }
  .btn-secondary:hover { color: var(--text); }

  .btn-ghost {
    padding: 4px 8px;
    border: none;
    background: none;
    color: var(--text-muted);
    font-size: 0.8rem;
    cursor: pointer;
    text-decoration: underline;
    text-underline-offset: 2px;
  }
  .btn-ghost:hover { color: var(--text); }

  /* ── result sections ────────────────────────────────────── */
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
