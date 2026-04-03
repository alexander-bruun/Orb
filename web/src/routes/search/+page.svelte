<script lang="ts">
  import { onMount } from "svelte";
  import {
    searchQuery,
    searchResults,
    searchFilters,
    savedFilters,
    saveFilter,
    deleteSavedFilter,
  } from "$lib/stores/library";
  import { library as libApi } from "$lib/api/library";
  import TrackList from "$lib/components/library/TrackList.svelte";
  import AlbumCard from "$lib/components/library/AlbumCard.svelte";
  import ArtistList from "$lib/components/library/ArtistList.svelte";
  import type { SearchFilters } from "$lib/types";

  let loading = false;
  let saveName = "";
  let showSaveInput = false;
  let searchDebounce: ReturnType<typeof setTimeout> | null = null;
  let localQuery = "";
  let searchInputEl: HTMLInputElement;

  // Local copies of filter fields for binding
  let genre = "";
  let yearFrom = "";
  let yearTo = "";
  let format = "";
  let bitrateMin = "";
  let bitrateMax = "";
  let bpmMin = "";
  let bpmMax = "";
  let sortTracks = "";
  let sortAlbums = "";
  let typesTracks = true;
  let typesAlbums = true;
  let typesArtists = true;
  let typesAudiobooks = false;
  let typesPodcasts = false;

  function isDefaultMusicTypes(
    types: ("tracks" | "albums" | "artists" | "audiobooks" | "podcasts")[]
  ): boolean {
    return (
      types.length === 3 &&
      types.includes("tracks") &&
      types.includes("albums") &&
      types.includes("artists")
    );
  }

  function buildFilters(): SearchFilters {
    const f: SearchFilters = {};
    if (genre.trim()) f.genre = genre.trim();
    const yf = parseInt(yearFrom);
    if (!isNaN(yf) && yf > 0) f.year_from = yf;
    const yt = parseInt(yearTo);
    if (!isNaN(yt) && yt > 0) f.year_to = yt;
    if (format) f.format = format;
    const bmin = parseInt(bitrateMin);
    if (!isNaN(bmin) && bmin > 0) f.bitrate_min = bmin;
    const bmax = parseInt(bitrateMax);
    if (!isNaN(bmax) && bmax > 0) f.bitrate_max = bmax;
    const pmin = parseFloat(bpmMin);
    if (!isNaN(pmin) && pmin > 0) f.bpm_min = pmin;
    const pmax = parseFloat(bpmMax);
    if (!isNaN(pmax) && pmax > 0) f.bpm_max = pmax;
    if (sortTracks) f.sort_tracks = sortTracks;
    if (sortAlbums) f.sort_albums = sortAlbums;
    const types: ("tracks" | "albums" | "artists" | "audiobooks" | "podcasts")[] =
      [];
    if (typesTracks) types.push("tracks");
    if (typesAlbums) types.push("albums");
    if (typesArtists) types.push("artists");
    if (typesAudiobooks) types.push("audiobooks");
    if (typesPodcasts) types.push("podcasts");
    if (!isDefaultMusicTypes(types)) f.types = types;
    return f;
  }

  function syncFromFilters(f: SearchFilters) {
    genre = f.genre ?? "";
    yearFrom = f.year_from ? String(f.year_from) : "";
    yearTo = f.year_to ? String(f.year_to) : "";
    format = f.format ?? "";
    bitrateMin = f.bitrate_min ? String(f.bitrate_min) : "";
    bitrateMax = f.bitrate_max ? String(f.bitrate_max) : "";
    bpmMin = f.bpm_min ? String(f.bpm_min) : "";
    bpmMax = f.bpm_max ? String(f.bpm_max) : "";
    sortTracks = f.sort_tracks ?? "";
    sortAlbums = f.sort_albums ?? "";
    if (!f.types || f.types.length === 0) {
      typesTracks = true;
      typesAlbums = true;
      typesArtists = true;
      typesAudiobooks = false;
      typesPodcasts = false;
      return;
    }
    typesTracks = f.types.includes("tracks");
    typesAlbums = f.types.includes("albums");
    typesArtists = f.types.includes("artists");
    typesAudiobooks = f.types.includes("audiobooks");
    typesPodcasts = f.types.includes("podcasts");
  }

  function hasActiveFilters(f: SearchFilters): boolean {
    const hasTypeOverride = !!f.types && !isDefaultMusicTypes(f.types);
    return !!(
      f.genre ||
      f.year_from ||
      f.year_to ||
      f.format ||
      f.bitrate_min ||
      f.bitrate_max ||
      f.bpm_min ||
      f.bpm_max ||
      f.sort_tracks ||
      f.sort_albums ||
      hasTypeOverride
    );
  }

  async function doSearch(q: string, filters: SearchFilters) {
    if (!q.trim()) return;
    loading = true;
    try {
      const res = await libApi.search(q, filters);
      searchResults.set(res);
    } finally {
      loading = false;
    }
  }

  function handleQueryInput() {
    if (searchDebounce) clearTimeout(searchDebounce);
    searchDebounce = setTimeout(() => {
      searchQuery.set(localQuery);
      doSearch(localQuery, buildFilters());
    }, 300);
  }

  function applyFilters() {
    const f = buildFilters();
    searchFilters.set(f);
    if (localQuery.trim()) doSearch(localQuery, f);
  }

  function clearFilters() {
    genre =
      yearFrom =
      yearTo =
      format =
      bitrateMin =
      bitrateMax =
      bpmMin =
      bpmMax =
      sortTracks =
      sortAlbums =
        "";
    typesTracks = typesAlbums = typesArtists = true;
    typesAudiobooks = typesPodcasts = false;
    searchFilters.set({});
    if (localQuery.trim()) doSearch(localQuery, {});
  }

  function applySavedFilter(f: SearchFilters) {
    syncFromFilters(f);
    searchFilters.set(f);
    if (localQuery.trim()) doSearch(localQuery, f);
  }

  function handleSave() {
    if (!saveName.trim()) return;
    saveFilter(saveName.trim(), buildFilters());
    saveName = "";
    showSaveInput = false;
  }

  onMount(() => {
    syncFromFilters($searchFilters);
    // Sync local query with store (e.g. navigated from TopBar quick search)
    localQuery = $searchQuery;
    if (localQuery.trim()) doSearch(localQuery, $searchFilters);

    // Focus the input on desktop only — on mobile this would pop the keyboard up immediately
    if (!("ontouchstart" in window)) {
      searchInputEl?.focus();
    }
  });

  $: hasResults =
    $searchResults.tracks.length > 0 ||
    $searchResults.albums.length > 0 ||
    $searchResults.artists.length > 0 ||
    $searchResults.audiobooks.length > 0 ||
    $searchResults.podcasts.length > 0;

  $: activeFilters = hasActiveFilters($searchFilters);
  $: activeFilterCount = [
    $searchFilters.genre,
    $searchFilters.year_from,
    $searchFilters.year_to,
    $searchFilters.format,
    $searchFilters.bitrate_min,
    $searchFilters.bitrate_max,
    $searchFilters.bpm_min,
    $searchFilters.bpm_max,
    $searchFilters.sort_tracks,
    $searchFilters.sort_albums,
    $searchFilters.types && !isDefaultMusicTypes($searchFilters.types)
      ? true
      : null,
  ].filter(Boolean).length;
</script>

<div class="search-page">
  <!-- Hero search input -->
  <div class="hero">
    <div class="hero-search">
      <svg
        class="hero-icon"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        viewBox="0 0 24 24"
        aria-hidden="true"
      >
        <circle cx="11" cy="11" r="8" />
        <path d="m21 21-4.35-4.35" />
      </svg>
      <input
        bind:this={searchInputEl}
        class="hero-input"
        type="search"
        placeholder="Search tracks, albums, artists…"
        bind:value={localQuery}
        on:input={handleQueryInput}
        aria-label="Search your library"
      />
      {#if localQuery}
        <button
          class="hero-clear"
          on:click={() => {
            localQuery = "";
            searchQuery.set("");
            searchResults.set({
              tracks: [],
              albums: [],
              artists: [],
              audiobooks: [],
              podcasts: [],
            });
            searchInputEl?.focus();
          }}
          aria-label="Clear"
        >
          <svg
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            viewBox="0 0 24 24"
            ><line x1="18" y1="6" x2="6" y2="18" /><line
              x1="6"
              y1="6"
              x2="18"
              y2="18"
            /></svg
          >
        </button>
      {/if}
    </div>
  </div>

  <!-- Filters -->
  <details class="filter-section" open>
    <summary class="filter-summary">
      <svg
        width="13"
        height="13"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        viewBox="0 0 24 24"
        aria-hidden="true"
      >
        <line x1="4" y1="6" x2="20" y2="6" />
        <line x1="8" y1="12" x2="16" y2="12" />
        <line x1="11" y1="18" x2="13" y2="18" />
      </svg>
      Filters
      {#if activeFilterCount > 0}
        <span class="filter-count">{activeFilterCount}</span>
      {/if}
      <svg
        class="chevron"
        width="12"
        height="12"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        viewBox="0 0 24 24"><path d="m6 9 6 6 6-6" /></svg
      >
    </summary>

    <div class="filter-body">
      <div class="filter-grid">
        <!-- Result types -->
        <fieldset class="filter-group filter-types">
          <legend class="filter-label">Show</legend>
          <label class="check-label"
            ><input type="checkbox" bind:checked={typesTracks} /> Tracks</label
          >
          <label class="check-label"
            ><input type="checkbox" bind:checked={typesAlbums} /> Albums</label
          >
          <label class="check-label"
            ><input type="checkbox" bind:checked={typesArtists} /> Artists</label
          >
          <label class="check-label"
            ><input type="checkbox" bind:checked={typesAudiobooks} />
            Audiobooks</label
          >
          <label class="check-label"
            ><input type="checkbox" bind:checked={typesPodcasts} />
            Podcasts</label
          >
        </fieldset>

        <!-- Genre -->
        <div class="filter-group">
          <label class="filter-label" for="f-genre">Genre</label>
          <input
            id="f-genre"
            class="filter-input"
            type="text"
            placeholder="e.g. jazz"
            bind:value={genre}
          />
        </div>

        <!-- Year range -->
        <div class="filter-group">
          <span class="filter-label">Year</span>
          <div class="range-row">
            <input
              class="filter-input narrow"
              type="number"
              placeholder="from"
              min="1900"
              max="2100"
              bind:value={yearFrom}
            />
            <span class="range-sep">–</span>
            <input
              class="filter-input narrow"
              type="number"
              placeholder="to"
              min="1900"
              max="2100"
              bind:value={yearTo}
            />
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
            <input
              class="filter-input narrow"
              type="number"
              placeholder="min"
              min="0"
              bind:value={bitrateMin}
            />
            <span class="range-sep">–</span>
            <input
              class="filter-input narrow"
              type="number"
              placeholder="max"
              min="0"
              bind:value={bitrateMax}
            />
          </div>
        </div>

        <!-- BPM -->
        <div class="filter-group">
          <span class="filter-label">BPM</span>
          <div class="range-row">
            <input
              class="filter-input narrow"
              type="number"
              placeholder="min"
              min="0"
              max="400"
              step="1"
              bind:value={bpmMin}
            />
            <span class="range-sep">–</span>
            <input
              class="filter-input narrow"
              type="number"
              placeholder="max"
              min="0"
              max="400"
              step="1"
              bind:value={bpmMax}
            />
          </div>
        </div>

        <!-- Sort tracks -->
        <div class="filter-group">
          <label class="filter-label" for="f-sort-tracks">Sort tracks</label>
          <select
            id="f-sort-tracks"
            class="filter-select"
            bind:value={sortTracks}
          >
            <option value="">Relevance</option>
            <option value="title">Title</option>
            <option value="year">Year</option>
            <option value="bitrate">Bitrate</option>
            <option value="duration">Duration</option>
            <option value="bpm">BPM</option>
          </select>
        </div>

        <!-- Sort albums -->
        <div class="filter-group">
          <label class="filter-label" for="f-sort-albums">Sort albums</label>
          <select
            id="f-sort-albums"
            class="filter-select"
            bind:value={sortAlbums}
          >
            <option value="">Relevance</option>
            <option value="title">Title</option>
            <option value="year">Year</option>
          </select>
        </div>
      </div>

      <div class="filter-actions">
        <button class="btn-primary" on:click={applyFilters}
          >Apply filters</button
        >
        {#if activeFilters}
          <button class="btn-secondary" on:click={clearFilters}>Clear</button>
        {/if}

        {#if showSaveInput}
          <div class="save-row">
            <input
              class="filter-input save-input"
              type="text"
              placeholder="Preset name"
              bind:value={saveName}
            />
            <button class="btn-primary" on:click={handleSave}>Save</button>
            <button
              class="btn-secondary"
              on:click={() => (showSaveInput = false)}>Cancel</button
            >
          </div>
        {:else}
          <button class="btn-ghost" on:click={() => (showSaveInput = true)}
            >Save preset…</button
          >
        {/if}
      </div>

      <!-- Saved presets -->
      {#if $savedFilters.length > 0}
        <div class="presets">
          <span class="presets-label">Presets:</span>
          {#each $savedFilters as sf (sf.name)}
            <button
              class="preset-chip"
              on:click={() => applySavedFilter(sf.filters)}
            >
              {sf.name}
            </button>
            <button
              class="preset-del"
              title="Delete preset"
              on:click={() => deleteSavedFilter(sf.name)}
              aria-label="Delete {sf.name}">×</button
            >
          {/each}
        </div>
      {/if}
    </div>
  </details>

  <!-- Results -->
  <div class="results">
    {#if loading}
      <p class="muted">Searching…</p>
    {:else if localQuery.trim()}
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

        {#if $searchResults.audiobooks.length}
          <section>
            <h2 class="section-title">
              Audiobooks
              <span class="count">{$searchResults.audiobooks.length}</span>
            </h2>
            <div class="media-list">
              {#each $searchResults.audiobooks as audiobook (audiobook.id)}
                <a class="media-item" href="/audiobooks/{audiobook.id}">
                  <span class="media-title">{audiobook.title}</span>
                  {#if audiobook.author_name}
                    <span class="media-meta">{audiobook.author_name}</span>
                  {/if}
                </a>
              {/each}
            </div>
          </section>
        {/if}

        {#if $searchResults.podcasts.length}
          <section>
            <h2 class="section-title">
              Podcasts
              <span class="count">{$searchResults.podcasts.length}</span>
            </h2>
            <div class="media-list">
              {#each $searchResults.podcasts as podcast (podcast.id)}
                <a class="media-item" href="/podcasts/{podcast.id}">
                  <span class="media-title">{podcast.title}</span>
                  {#if podcast.author}
                    <span class="media-meta">{podcast.author}</span>
                  {/if}
                </a>
              {/each}
            </div>
          </section>
        {/if}

        {#if $searchResults.tracks.length && !$searchResults.artists.length}
          <section>
            <h2 class="section-title">
              Tracks
              <span class="count">{$searchResults.tracks.length}</span>
            </h2>
            <TrackList tracks={$searchResults.tracks} />
          </section>
        {/if}
      {:else}
        <p class="muted">
          No results for "<span class="query">{localQuery}</span>"
          {#if activeFilters}— try removing some filters{/if}
        </p>
      {/if}
    {:else}
      <p class="muted hint">Type to search your library</p>
    {/if}
  </div>
</div>

<svelte:head>
  <title
    >{$searchQuery ? `"${$searchQuery}" – Search – Orb` : "Search – Orb"}</title
  >
</svelte:head>

<style>
  .search-page {
    display: flex;
    flex-direction: column;
    gap: 0;
  }

  /* ── Hero search ────────────────────────────────────────── */
  .hero {
    padding: 8px 0 20px;
  }

  .hero-search {
    display: flex;
    align-items: center;
    gap: 12px;
    background: var(--surface);
    border: 1px solid var(--border-2);
    border-radius: 12px;
    padding: 0 16px;
    height: 52px;
    transition:
      border-color 0.15s,
      box-shadow 0.15s;
  }
  .hero-search:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-dim);
  }

  .hero-icon {
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .hero-input {
    flex: 1;
    background: none;
    border: none;
    outline: none;
    font-size: 1rem;
    color: var(--text);
    font-family: "Syne", sans-serif;
  }
  .hero-input::placeholder {
    color: var(--text-muted);
  }

  .hero-clear {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 4px;
    border-radius: 4px;
    display: flex;
    align-items: center;
    transition: color 0.12s;
  }
  .hero-clear:hover {
    color: var(--text);
  }

  /* ── Filters (details/summary) ──────────────────────────── */
  .filter-section {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    margin-bottom: 24px;
    overflow: hidden;
  }

  .filter-summary {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 12px 16px;
    font-size: 0.8125rem;
    font-weight: 600;
    color: var(--text-2);
    cursor: pointer;
    list-style: none;
    user-select: none;
    transition: background 0.1s;
  }
  .filter-summary::-webkit-details-marker {
    display: none;
  }
  .filter-summary:hover {
    background: var(--surface-2);
  }

  .filter-count {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: var(--accent);
    color: #fff;
    font-size: 0.625rem;
    font-weight: 700;
    border-radius: 10px;
    min-width: 16px;
    height: 16px;
    padding: 0 4px;
    line-height: 1;
  }

  .chevron {
    margin-left: auto;
    transition: transform 0.2s;
    color: var(--text-muted);
  }
  details[open] .chevron {
    transform: rotate(180deg);
  }

  .filter-body {
    border-top: 1px solid var(--border);
    padding: 16px;
  }

  .filter-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 12px 16px;
    margin-bottom: 16px;
  }

  .filter-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
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
    border-radius: 6px;
    padding: 7px 10px;
    font-size: 0.8125rem;
    color: var(--text);
    width: 100%;
    box-sizing: border-box;
    transition: border-color 0.15s;
  }
  .filter-input:focus {
    outline: none;
    border-color: var(--accent);
  }

  .filter-select {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 7px 10px;
    font-size: 0.8125rem;
    color: var(--text);
    width: 100%;
    transition: border-color 0.15s;
  }
  .filter-select:focus {
    outline: none;
    border-color: var(--accent);
  }

  .narrow {
    width: 70px;
    flex: 0 0 70px;
  }

  .range-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .range-sep {
    color: var(--text-muted);
    font-size: 0.875rem;
    flex-shrink: 0;
  }

  .check-label {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.8125rem;
    color: var(--text);
    cursor: pointer;
    padding: 2px 0;
  }

  .filter-actions {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 12px;
  }

  .save-row {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }
  .save-input {
    max-width: 180px;
  }

  /* Presets bar */
  .presets {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: wrap;
    padding-top: 8px;
    border-top: 1px solid var(--border);
  }
  .presets-label {
    font-size: 0.6875rem;
    color: var(--text-muted);
    margin-right: 2px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    font-weight: 600;
  }
  .preset-chip {
    font-size: 0.75rem;
    padding: 3px 10px;
    border-radius: 12px;
    border: 1px solid var(--border);
    background: var(--accent-dim);
    color: var(--accent);
    cursor: pointer;
    transition: background 0.12s;
  }
  .preset-chip:hover {
    background: var(--accent-glow);
  }
  .preset-del {
    font-size: 0.75rem;
    padding: 1px 4px;
    border: none;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    margin-left: -2px;
    line-height: 1;
  }
  .preset-del:hover {
    color: var(--text);
  }

  .btn-primary {
    padding: 7px 16px;
    border-radius: 7px;
    border: none;
    background: var(--accent);
    color: #fff;
    font-size: 0.8125rem;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 0.12s;
  }
  .btn-primary:hover {
    opacity: 0.85;
  }

  .btn-secondary {
    padding: 7px 14px;
    border-radius: 7px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text-muted);
    font-size: 0.8125rem;
    cursor: pointer;
    transition: color 0.12s;
  }
  .btn-secondary:hover {
    color: var(--text);
  }

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
  .btn-ghost:hover {
    color: var(--text);
  }

  /* ── Results ────────────────────────────────────────────── */
  .results {
    display: block;
  }

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

  .media-list {
    display: grid;
    gap: 8px;
  }

  .media-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 10px 12px;
    border: 1px solid var(--border-2);
    border-radius: 8px;
    background: var(--surface);
    text-decoration: none;
  }
  .media-item:hover {
    border-color: var(--accent);
  }

  .media-title {
    color: var(--text);
    font-size: 0.875rem;
    font-weight: 600;
  }

  .media-meta {
    color: var(--text-muted);
    font-size: 0.75rem;
  }

  .muted {
    color: var(--text-muted);
    font-size: 0.875rem;
  }
  .hint {
    text-align: center;
    padding: 48px 0;
  }

  .query {
    color: var(--text);
  }

  /* ── Mobile ─────────────────────────────────────────────── */
  @media (max-width: 640px) {
    .hero {
      padding: 4px 0 16px;
    }
    .hero-search {
      height: 46px;
      padding: 0 12px;
      gap: 10px;
    }
    .hero-input {
      font-size: 0.9375rem;
    }

    .filter-grid {
      grid-template-columns: 1fr 1fr;
      gap: 10px 12px;
    }
    .filter-types {
      grid-column: 1 / -1;
      display: flex;
      flex-direction: row;
      gap: 16px;
    }

    .album-grid {
      grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
      gap: 12px;
    }
  }

  @media (max-width: 400px) {
    .filter-grid {
      grid-template-columns: 1fr;
    }
    .filter-types {
      flex-direction: row;
      flex-wrap: wrap;
    }
  }
</style>
