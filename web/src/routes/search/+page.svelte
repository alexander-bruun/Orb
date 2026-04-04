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
  import type { Artist, SearchFilters } from "$lib/types";
  import { goto } from "$app/navigation";
  import { getApiBase } from "$lib/api/base";

  // ── Recent artists (persisted to localStorage) ───────────────
  const RECENT_KEY = "orb_recent_artists";
  const RECENT_MAX = 8;

  type RecentArtist = { id: string; name: string; image_key?: string };

  function loadRecent(): RecentArtist[] {
    try {
      return JSON.parse(localStorage.getItem(RECENT_KEY) ?? "[]");
    } catch {
      return [];
    }
  }

  function pushRecent(artist: Artist) {
    const list = loadRecent().filter((a) => a.id !== artist.id);
    list.unshift({ id: artist.id, name: artist.name, image_key: artist.image_key });
    if (list.length > RECENT_MAX) list.length = RECENT_MAX;
    localStorage.setItem(RECENT_KEY, JSON.stringify(list));
    recentArtists = list;
  }

  function removeRecent(id: string) {
    recentArtists = recentArtists.filter((a) => a.id !== id);
    localStorage.setItem(RECENT_KEY, JSON.stringify(recentArtists));
  }

  function clearRecent() {
    recentArtists = [];
    localStorage.removeItem(RECENT_KEY);
  }

  let recentArtists: RecentArtist[] = [];

  function handleArtistSelect(artist: Artist) {
    pushRecent(artist);
    goto(`/artists/${artist.id}`);
  }

  let loading = false;
  let saveName = "";
  let showSaveInput = false;
  let searchDebounce: ReturnType<typeof setTimeout> | null = null;
  let localQuery = "";
  let searchInputEl: HTMLInputElement;

  // Filter sheet state
  let filterSheetOpen = false;
  let filterSheetEl: HTMLDivElement;

  // Selected result type pill
  let selectedType: "all" | "tracks" | "albums" | "artists" | "audiobooks" | "podcasts" = "all";

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
    const types: ("tracks" | "albums" | "artists" | "audiobooks" | "podcasts")[] = [];
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
    filterSheetOpen = false;
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
    filterSheetOpen = false;
  }

  function handleSave() {
    if (!saveName.trim()) return;
    saveFilter(saveName.trim(), buildFilters());
    saveName = "";
    showSaveInput = false;
  }

  function closeFilterSheet(e: MouseEvent) {
    if (filterSheetEl && !filterSheetEl.contains(e.target as Node)) {
      filterSheetOpen = false;
    }
  }

  onMount(() => {
    recentArtists = loadRecent();
    syncFromFilters($searchFilters);
    localQuery = $searchQuery;
    if (localQuery.trim()) doSearch(localQuery, $searchFilters);

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
    $searchFilters.types && !isDefaultMusicTypes($searchFilters.types) ? true : null,
  ].filter(Boolean).length;

  // Visible result counts per type
  $: countTracks = $searchResults.tracks.length;
  $: countAlbums = $searchResults.albums.length;
  $: countArtists = $searchResults.artists.length;
  $: countAudiobooks = $searchResults.audiobooks.length;
  $: countPodcasts = $searchResults.podcasts.length;

  // Which types have results
  $: typesWithResults = [
    countTracks > 0 && "tracks",
    countAlbums > 0 && "albums",
    countArtists > 0 && "artists",
    countAudiobooks > 0 && "audiobooks",
    countPodcasts > 0 && "podcasts",
  ].filter(Boolean) as string[];

  // Reset to "all" when results change
  $: if (hasResults && !typesWithResults.includes(selectedType)) {
    selectedType = "all";
  }

  // Filter display by selected pill
  $: showTracks = (selectedType === "all" || selectedType === "tracks") && countTracks > 0;
  $: showAlbums = (selectedType === "all" || selectedType === "albums") && countAlbums > 0;
  $: showArtists = (selectedType === "all" || selectedType === "artists") && countArtists > 0;
  $: showAudiobooks = (selectedType === "all" || selectedType === "audiobooks") && countAudiobooks > 0;
  $: showPodcasts = (selectedType === "all" || selectedType === "podcasts") && countPodcasts > 0;
</script>

<svelte:window on:click={closeFilterSheet} />

<div class="search-page">
  <!-- ── Search bar ─────────────────────────────────────────────── -->
  <div class="search-bar-wrap">
    <div class="search-bar" class:focused={false}>
      <svg class="bar-icon" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true">
        <circle cx="11" cy="11" r="8" /><path d="m21 21-4.35-4.35" />
      </svg>
      <input
        bind:this={searchInputEl}
        class="bar-input"
        type="search"
        placeholder="Search your library…"
        bind:value={localQuery}
        on:input={handleQueryInput}
        aria-label="Search your library"
        autocomplete="off"
        autocorrect="off"
        spellcheck="false"
      />
      {#if localQuery}
        <button
          class="bar-clear"
          on:click={() => {
            localQuery = "";
            searchQuery.set("");
            searchResults.set({ tracks: [], albums: [], artists: [], audiobooks: [], podcasts: [] });
            selectedType = "all";
            searchInputEl?.focus();
          }}
          aria-label="Clear search"
        >
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
            <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>
      {/if}
    </div>
  </div>

  <!-- ── Toolbar: type pills + filter button ───────────────────── -->
  <div class="toolbar">
    <div class="pills-scroll">
      <div class="pills">
        <button
          class="pill"
          class:pill-active={selectedType === "all"}
          on:click={() => (selectedType = "all")}
        >All</button>

        {#if !hasResults || countTracks > 0}
          <button
            class="pill"
            class:pill-active={selectedType === "tracks"}
            on:click={() => (selectedType = selectedType === "tracks" ? "all" : "tracks")}
          >
            Tracks{#if countTracks > 0}<span class="pill-count">{countTracks}</span>{/if}
          </button>
        {/if}

        {#if !hasResults || countAlbums > 0}
          <button
            class="pill"
            class:pill-active={selectedType === "albums"}
            on:click={() => (selectedType = selectedType === "albums" ? "all" : "albums")}
          >
            Albums{#if countAlbums > 0}<span class="pill-count">{countAlbums}</span>{/if}
          </button>
        {/if}

        {#if !hasResults || countArtists > 0}
          <button
            class="pill"
            class:pill-active={selectedType === "artists"}
            on:click={() => (selectedType = selectedType === "artists" ? "all" : "artists")}
          >
            Artists{#if countArtists > 0}<span class="pill-count">{countArtists}</span>{/if}
          </button>
        {/if}

        {#if !hasResults || countAudiobooks > 0}
          <button
            class="pill"
            class:pill-active={selectedType === "audiobooks"}
            on:click={() => (selectedType = selectedType === "audiobooks" ? "all" : "audiobooks")}
          >
            Audiobooks{#if countAudiobooks > 0}<span class="pill-count">{countAudiobooks}</span>{/if}
          </button>
        {/if}

        {#if !hasResults || countPodcasts > 0}
          <button
            class="pill"
            class:pill-active={selectedType === "podcasts"}
            on:click={() => (selectedType = selectedType === "podcasts" ? "all" : "podcasts")}
          >
            Podcasts{#if countPodcasts > 0}<span class="pill-count">{countPodcasts}</span>{/if}
          </button>
        {/if}
      </div>
    </div>

    <!-- Filter button (stops event propagation so window click doesn't close it immediately) -->
    <button
      class="filter-btn"
      class:filter-btn-active={filterSheetOpen || activeFilterCount > 0}
      on:click|stopPropagation={() => (filterSheetOpen = !filterSheetOpen)}
      aria-label="Open filters"
      aria-expanded={filterSheetOpen}
    >
      <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true">
        <line x1="4" y1="6" x2="20" y2="6" /><line x1="8" y1="12" x2="16" y2="12" /><line x1="11" y1="18" x2="13" y2="18" />
      </svg>
      {#if activeFilterCount > 0}
        <span class="filter-badge">{activeFilterCount}</span>
      {:else}
        <span class="filter-label-text">Filters</span>
      {/if}
    </button>
  </div>

  <!-- ── Filter sheet ───────────────────────────────────────────── -->
  {#if filterSheetOpen}
    <div class="sheet-backdrop" aria-hidden="true"></div>
    <div
      class="filter-sheet"
      bind:this={filterSheetEl}
      on:click|stopPropagation
      on:keydown|stopPropagation
      role="dialog"
      tabindex="-1"
      aria-label="Search filters"
    >
      <div class="sheet-header">
        <span class="sheet-title">Filters</span>
        <button class="sheet-close" on:click={() => (filterSheetOpen = false)} aria-label="Close filters">
          <svg width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>
      </div>

      <div class="sheet-body">
        <!-- Type toggles -->
        <div class="sheet-group">
          <span class="sheet-group-label">Show</span>
          <div class="toggle-chips">
            <label class="toggle-chip" class:chip-on={typesTracks}>
              <input type="checkbox" bind:checked={typesTracks} /><span>Tracks</span>
            </label>
            <label class="toggle-chip" class:chip-on={typesAlbums}>
              <input type="checkbox" bind:checked={typesAlbums} /><span>Albums</span>
            </label>
            <label class="toggle-chip" class:chip-on={typesArtists}>
              <input type="checkbox" bind:checked={typesArtists} /><span>Artists</span>
            </label>
            <label class="toggle-chip" class:chip-on={typesAudiobooks}>
              <input type="checkbox" bind:checked={typesAudiobooks} /><span>Audiobooks</span>
            </label>
            <label class="toggle-chip" class:chip-on={typesPodcasts}>
              <input type="checkbox" bind:checked={typesPodcasts} /><span>Podcasts</span>
            </label>
          </div>
        </div>

        <div class="sheet-row2">
          <!-- Genre -->
          <div class="sheet-group">
            <label class="sheet-group-label" for="f-genre">Genre</label>
            <input id="f-genre" class="sheet-input" type="text" placeholder="e.g. jazz" bind:value={genre} />
          </div>

          <!-- Format -->
          <div class="sheet-group">
            <label class="sheet-group-label" for="f-format">Format</label>
            <select id="f-format" class="sheet-select" bind:value={format}>
              <option value="">Any</option>
              <option value="flac">FLAC</option>
              <option value="mp3">MP3</option>
              <option value="wav">WAV</option>
              <option value="aac">AAC</option>
              <option value="ogg">OGG</option>
            </select>
          </div>
        </div>

        <div class="sheet-row2">
          <!-- Year range -->
          <div class="sheet-group">
            <span class="sheet-group-label">Year</span>
            <div class="range-pair">
              <input class="sheet-input" type="number" placeholder="from" min="1900" max="2100" bind:value={yearFrom} />
              <span class="range-dash">–</span>
              <input class="sheet-input" type="number" placeholder="to" min="1900" max="2100" bind:value={yearTo} />
            </div>
          </div>

          <!-- Bitrate -->
          <div class="sheet-group">
            <span class="sheet-group-label">Bitrate (kbps)</span>
            <div class="range-pair">
              <input class="sheet-input" type="number" placeholder="min" min="0" bind:value={bitrateMin} />
              <span class="range-dash">–</span>
              <input class="sheet-input" type="number" placeholder="max" min="0" bind:value={bitrateMax} />
            </div>
          </div>
        </div>

        <div class="sheet-row2">
          <!-- BPM -->
          <div class="sheet-group">
            <span class="sheet-group-label">BPM</span>
            <div class="range-pair">
              <input class="sheet-input" type="number" placeholder="min" min="0" max="400" step="1" bind:value={bpmMin} />
              <span class="range-dash">–</span>
              <input class="sheet-input" type="number" placeholder="max" min="0" max="400" step="1" bind:value={bpmMax} />
            </div>
          </div>

          <!-- Sort -->
          <div class="sheet-group">
            <label class="sheet-group-label" for="f-sort-tracks">Sort tracks</label>
            <select id="f-sort-tracks" class="sheet-select" bind:value={sortTracks}>
              <option value="">Relevance</option>
              <option value="title">Title</option>
              <option value="year">Year</option>
              <option value="bitrate">Bitrate</option>
              <option value="duration">Duration</option>
              <option value="bpm">BPM</option>
            </select>
          </div>
        </div>

        <div class="sheet-row2">
          <div class="sheet-group">
            <label class="sheet-group-label" for="f-sort-albums">Sort albums</label>
            <select id="f-sort-albums" class="sheet-select" bind:value={sortAlbums}>
              <option value="">Relevance</option>
              <option value="title">Title</option>
              <option value="year">Year</option>
            </select>
          </div>
        </div>

        <!-- Saved presets -->
        {#if $savedFilters.length > 0}
          <div class="sheet-group">
            <span class="sheet-group-label">Presets</span>
            <div class="presets">
              {#each $savedFilters as sf (sf.name)}
                <button class="preset-chip" on:click={() => applySavedFilter(sf.filters)}>{sf.name}</button>
                <button class="preset-del" title="Delete preset" on:click={() => deleteSavedFilter(sf.name)} aria-label="Delete {sf.name}">
                  <svg width="10" height="10" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
                    <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
                  </svg>
                </button>
              {/each}
            </div>
          </div>
        {/if}

        <!-- Save preset input -->
        {#if showSaveInput}
          <div class="save-row">
            <input class="sheet-input" type="text" placeholder="Preset name" bind:value={saveName} />
            <button class="btn-accent" on:click={handleSave}>Save</button>
            <button class="btn-ghost" on:click={() => (showSaveInput = false)}>Cancel</button>
          </div>
        {/if}
      </div>

      <div class="sheet-footer">
        <button class="btn-ghost" on:click={() => (showSaveInput = !showSaveInput)}>
          {showSaveInput ? "Cancel preset" : "Save as preset…"}
        </button>
        <div class="sheet-footer-right">
          {#if activeFilters}
            <button class="btn-outline" on:click={clearFilters}>Clear</button>
          {/if}
          <button class="btn-accent" on:click={applyFilters}>Apply</button>
        </div>
      </div>
    </div>
  {/if}

  <!-- ── Results ────────────────────────────────────────────────── -->
  <div class="results">
    {#if loading}
      <!-- Skeleton loader -->
      <div class="skeleton-wrap" aria-label="Searching…" aria-busy="true">
        <div class="sk-section-label"></div>
        <div class="sk-row"></div>
        <div class="sk-row sk-row--short"></div>
        <div class="sk-row"></div>
        <div class="sk-section-label" style="margin-top:28px"></div>
        <div class="sk-grid">
          {#each Array(4) as _}
            <div class="sk-card"></div>
          {/each}
        </div>
      </div>

    {:else if !localQuery.trim()}
      <!-- Empty / idle state -->
      {#if recentArtists.length > 0}
        <div class="recent-section">
          <div class="recent-header">
            <span class="section-label">Recent</span>
            <button class="btn-ghost recent-clear" on:click={clearRecent}>Clear</button>
          </div>
          <div class="recent-list">
            {#each recentArtists as artist (artist.id)}
              <div class="recent-row">
                <button
                  class="recent-artist"
                  on:click={() => goto(`/artists/${artist.id}`)}
                >
                  {#if artist.image_key}
                    <img
                      class="recent-avatar"
                      src="{getApiBase()}/covers/artist/{artist.id}"
                      alt={artist.name}
                      loading="lazy"
                    />
                  {:else}
                    <div class="recent-monogram">{artist.name.slice(0, 1).toUpperCase()}</div>
                  {/if}
                  <span class="recent-name">{artist.name}</span>
                  <svg class="recent-chevron" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true">
                    <polyline points="9 18 15 12 9 6" />
                  </svg>
                </button>
                <button
                  class="recent-remove"
                  on:click={() => removeRecent(artist.id)}
                  aria-label="Remove {artist.name} from recent"
                >
                  <svg width="12" height="12" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
                    <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
                  </svg>
                </button>
              </div>
            {/each}
          </div>
        </div>
      {:else}
        <div class="idle-state">
          <div class="idle-icon" aria-hidden="true">
            <svg width="48" height="48" fill="none" stroke="currentColor" stroke-width="1.25" viewBox="0 0 24 24" opacity="0.25">
              <circle cx="11" cy="11" r="8" /><path d="m21 21-4.35-4.35" />
            </svg>
          </div>
          <p class="idle-title">Search your library</p>
          <p class="idle-sub">Tracks, albums, artists, audiobooks, and podcasts</p>
        </div>
      {/if}

    {:else if hasResults}
      {#if showArtists}
        <section class="result-section">
          <h2 class="section-label">
            Artists
            <span class="section-count">{countArtists}</span>
          </h2>
          <ArtistList artists={$searchResults.artists} onSelect={handleArtistSelect} />
        </section>
      {/if}

      {#if showAlbums}
        <section class="result-section">
          <h2 class="section-label">
            Albums
            <span class="section-count">{countAlbums}</span>
          </h2>
          <div class="album-grid">
            {#each $searchResults.albums as album (album.id)}
              <AlbumCard {album} />
            {/each}
          </div>
        </section>
      {/if}

      {#if showAudiobooks}
        <section class="result-section">
          <h2 class="section-label">
            Audiobooks
            <span class="section-count">{countAudiobooks}</span>
          </h2>
          <div class="media-list">
            {#each $searchResults.audiobooks as audiobook (audiobook.id)}
              <a class="media-item" href="/audiobooks/{audiobook.id}">
                <span class="media-title">{audiobook.title}</span>
                {#if audiobook.author_name}<span class="media-meta">{audiobook.author_name}</span>{/if}
              </a>
            {/each}
          </div>
        </section>
      {/if}

      {#if showPodcasts}
        <section class="result-section">
          <h2 class="section-label">
            Podcasts
            <span class="section-count">{countPodcasts}</span>
          </h2>
          <div class="media-list">
            {#each $searchResults.podcasts as podcast (podcast.id)}
              <a class="media-item" href="/podcasts/{podcast.id}">
                <span class="media-title">{podcast.title}</span>
                {#if podcast.author}<span class="media-meta">{podcast.author}</span>{/if}
              </a>
            {/each}
          </div>
        </section>
      {/if}

      {#if showTracks}
        <section class="result-section">
          <h2 class="section-label">
            Tracks
            <span class="section-count">{countTracks}</span>
          </h2>
          <TrackList tracks={$searchResults.tracks} />
        </section>
      {/if}

    {:else}
      <!-- No results -->
      <div class="no-results">
        <p class="no-results-title">No results for "<span class="query-text">{localQuery}</span>"</p>
        {#if activeFilters}
          <p class="no-results-sub">Try removing some filters</p>
          <button class="btn-outline" style="margin-top:12px" on:click={clearFilters}>Clear filters</button>
        {:else}
          <p class="no-results-sub">Check your spelling or try a different search</p>
        {/if}
      </div>
    {/if}
  </div>
</div>

<svelte:head>
  <title>{$searchQuery ? `"${$searchQuery}" – Search – Orb` : "Search – Orb"}</title>
</svelte:head>

<style>
  /* ── Page shell ─────────────────────────────────────────────── */
  .search-page {
    display: flex;
    flex-direction: column;
    gap: 0;
    position: relative;
  }

  /* ── Search bar ─────────────────────────────────────────────── */
  .search-bar-wrap {
    padding: 6px 0 14px;
    position: sticky;
    top: 0;
    z-index: 10;
    background: var(--bg);
  }

  .search-bar {
    display: flex;
    align-items: center;
    gap: 12px;
    background: var(--surface);
    border: 1.5px solid var(--border-2);
    border-radius: 100px;
    padding: 0 18px;
    height: 52px;
    transition:
      border-color 0.15s,
      box-shadow 0.15s,
      background 0.15s;
  }
  .search-bar:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-dim);
    background: var(--surface-2);
  }

  .bar-icon {
    color: var(--text-2);
    flex-shrink: 0;
    transition: color 0.15s;
  }
  .search-bar:focus-within .bar-icon {
    color: var(--accent);
  }

  .bar-input {
    flex: 1;
    background: none;
    border: none;
    outline: none;
    font-size: 1rem;
    color: var(--text);
    font-family: "Syne", sans-serif;
    min-width: 0;
  }
  .bar-input::placeholder {
    color: var(--text-2);
  }
  /* Remove browser default search field X */
  .bar-input::-webkit-search-cancel-button { display: none; }

  .bar-clear {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: 50%;
    color: var(--text-2);
    flex-shrink: 0;
    transition: background 0.12s, color 0.12s;
  }
  .bar-clear:hover {
    background: var(--surface-2);
    color: var(--text);
  }

  /* ── Toolbar ────────────────────────────────────────────────── */
  .toolbar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 20px;
    min-width: 0;
  }

  .pills-scroll {
    flex: 1;
    min-width: 0;
    overflow-x: auto;
    scrollbar-width: none;
    -webkit-overflow-scrolling: touch;
  }
  .pills-scroll::-webkit-scrollbar { display: none; }

  .pills {
    display: flex;
    gap: 6px;
    width: max-content;
  }

  .pill {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 6px 14px;
    border-radius: 100px;
    font-size: 0.8125rem;
    font-weight: 500;
    white-space: nowrap;
    border: 1.5px solid var(--border-2);
    background: var(--surface);
    color: var(--text-2);
    transition:
      background 0.12s,
      border-color 0.12s,
      color 0.12s;
    cursor: pointer;
  }
  .pill:hover {
    border-color: var(--accent);
    color: var(--text);
  }
  .pill-active {
    background: var(--accent-dim);
    border-color: var(--accent);
    color: var(--accent);
  }

  .pill-count {
    font-size: 0.6875rem;
    font-weight: 700;
    opacity: 0.75;
  }

  /* Filter button */
  .filter-btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 6px 12px;
    border-radius: 100px;
    border: 1.5px solid var(--border-2);
    background: var(--surface);
    color: var(--text-2);
    font-size: 0.8125rem;
    font-weight: 500;
    white-space: nowrap;
    flex-shrink: 0;
    transition:
      background 0.12s,
      border-color 0.12s,
      color 0.12s;
    cursor: pointer;
  }
  .filter-btn:hover {
    border-color: var(--accent);
    color: var(--text);
  }
  .filter-btn-active {
    background: var(--accent-dim);
    border-color: var(--accent);
    color: var(--accent);
  }

  .filter-label-text {
    font-size: 0.8125rem;
  }

  .filter-badge {
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

  /* ── Filter sheet ───────────────────────────────────────────── */
  .sheet-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.45);
    z-index: 40;
    backdrop-filter: blur(2px);
    animation: fade-in 0.18s ease;
  }

  .filter-sheet {
    position: fixed;
    left: 50%;
    top: 50%;
    transform: translate(-50%, -50%);
    width: min(520px, calc(100vw - 32px));
    max-height: min(640px, 90dvh);
    background: var(--surface);
    border: 1px solid var(--border-2);
    border-radius: 16px;
    display: flex;
    flex-direction: column;
    z-index: 50;
    overflow: hidden;
    box-shadow: 0 24px 64px rgba(0, 0, 0, 0.55);
    animation: sheet-in 0.2s cubic-bezier(0.34, 1.56, 0.64, 1);
  }

  @keyframes fade-in {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  @keyframes sheet-in {
    from { opacity: 0; transform: translate(-50%, -48%) scale(0.96); }
    to { opacity: 1; transform: translate(-50%, -50%) scale(1); }
  }

  .sheet-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 20px 14px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }

  .sheet-title {
    font-size: 0.9375rem;
    font-weight: 700;
    color: var(--text);
  }

  .sheet-close {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: 50%;
    color: var(--text-2);
    transition: background 0.12s, color 0.12s;
  }
  .sheet-close:hover {
    background: var(--surface-2);
    color: var(--text);
  }

  .sheet-body {
    overflow-y: auto;
    padding: 16px 20px;
    display: flex;
    flex-direction: column;
    gap: 16px;
    flex: 1;
  }

  .sheet-group {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .sheet-group-label {
    font-size: 0.6875rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--text-2);
  }

  .sheet-row2 {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }

  .sheet-input {
    background: var(--bg);
    border: 1px solid var(--border-2);
    border-radius: 8px;
    padding: 8px 11px;
    font-size: 0.875rem;
    color: var(--text);
    width: 100%;
    box-sizing: border-box;
    transition: border-color 0.15s;
    font-family: "Syne", sans-serif;
  }
  .sheet-input:focus {
    outline: none;
    border-color: var(--accent);
  }
  .sheet-input::placeholder {
    color: var(--text-2);
  }

  .sheet-select {
    background: var(--bg);
    border: 1px solid var(--border-2);
    border-radius: 8px;
    padding: 8px 11px;
    font-size: 0.875rem;
    color: var(--text);
    width: 100%;
    transition: border-color 0.15s;
    font-family: "Syne", sans-serif;
  }
  .sheet-select:focus {
    outline: none;
    border-color: var(--accent);
  }

  .range-pair {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .range-pair .sheet-input {
    flex: 1;
    min-width: 0;
  }
  .range-dash {
    color: var(--text-2);
    font-size: 0.875rem;
    flex-shrink: 0;
  }

  /* Toggle chips (type checkboxes) */
  .toggle-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .toggle-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 5px 12px;
    border-radius: 100px;
    border: 1.5px solid var(--border-2);
    background: var(--bg);
    color: var(--text-2);
    font-size: 0.8125rem;
    font-weight: 500;
    cursor: pointer;
    transition:
      background 0.12s,
      border-color 0.12s,
      color 0.12s;
  }
  .toggle-chip input {
    display: none;
  }
  .toggle-chip:hover {
    border-color: var(--accent);
    color: var(--text);
  }
  .chip-on {
    background: var(--accent-dim);
    border-color: var(--accent);
    color: var(--accent);
  }

  /* Presets */
  .presets {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }
  .preset-chip {
    font-size: 0.75rem;
    font-weight: 500;
    padding: 4px 12px;
    border-radius: 100px;
    border: 1px solid var(--border-2);
    background: var(--accent-dim);
    color: var(--accent);
    cursor: pointer;
    transition: background 0.12s;
  }
  .preset-chip:hover { background: var(--accent-glow); }

  .preset-del {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    color: var(--text-2);
    margin-left: -4px;
    transition: background 0.12s, color 0.12s;
  }
  .preset-del:hover {
    background: var(--surface-2);
    color: var(--text);
  }

  .save-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .save-row .sheet-input { flex: 1; }

  .sheet-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 20px;
    border-top: 1px solid var(--border);
    flex-shrink: 0;
    gap: 8px;
  }
  .sheet-footer-right {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  /* Buttons */
  .btn-accent {
    padding: 8px 18px;
    border-radius: 8px;
    border: none;
    background: var(--accent);
    color: #fff;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    font-family: "Syne", sans-serif;
    transition: opacity 0.12s;
  }
  .btn-accent:hover { opacity: 0.85; }

  .btn-outline {
    padding: 8px 16px;
    border-radius: 8px;
    border: 1.5px solid var(--border-2);
    background: var(--surface);
    color: var(--text-2);
    font-size: 0.875rem;
    font-weight: 500;
    cursor: pointer;
    font-family: "Syne", sans-serif;
    transition: border-color 0.12s, color 0.12s;
  }
  .btn-outline:hover {
    border-color: var(--accent);
    color: var(--text);
  }

  .btn-ghost {
    padding: 6px 8px;
    border: none;
    background: none;
    color: var(--text-2);
    font-size: 0.8125rem;
    cursor: pointer;
    font-family: "Syne", sans-serif;
    transition: color 0.12s;
  }
  .btn-ghost:hover { color: var(--text); }

  /* ── Results ────────────────────────────────────────────────── */
  .results { display: flex; flex-direction: column; gap: 0; }

  .result-section { margin-bottom: 36px; }

  .section-label {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.6875rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-2);
    margin-bottom: 14px;
  }

  .section-count {
    font-size: 0.6875rem;
    font-weight: 700;
    color: var(--accent);
    background: var(--accent-dim);
    border: 1px solid var(--accent-glow);
    border-radius: 4px;
    padding: 1px 6px;
    letter-spacing: 0;
  }

  .album-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 16px;
  }

  .media-list { display: grid; gap: 8px; }
  .media-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 11px 14px;
    border: 1px solid var(--border-2);
    border-radius: 10px;
    background: var(--surface);
    text-decoration: none;
    transition: border-color 0.12s, background 0.12s;
  }
  .media-item:hover {
    border-color: var(--accent);
    background: var(--surface-2);
  }
  .media-title {
    color: var(--text);
    font-size: 0.875rem;
    font-weight: 600;
  }
  .media-meta {
    color: var(--text-2);
    font-size: 0.75rem;
  }

  /* ── Idle / empty states ────────────────────────────────────── */
  .idle-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 64px 24px;
    text-align: center;
  }
  .idle-icon { margin-bottom: 4px; }
  .idle-title {
    font-size: 1rem;
    font-weight: 600;
    color: var(--text-2);
  }
  .idle-sub {
    font-size: 0.8125rem;
    color: var(--text-2);
    opacity: 0.6;
  }

  .no-results {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 6px;
    padding: 56px 24px;
    text-align: center;
  }
  .no-results-title {
    font-size: 0.9375rem;
    font-weight: 600;
    color: var(--text-2);
  }
  .query-text { color: var(--text); }
  .no-results-sub {
    font-size: 0.8125rem;
    color: var(--text-2);
    opacity: 0.6;
  }

  /* ── Skeleton loader ─────────────────────────────────────────── */
  @keyframes shimmer {
    0% { background-position: -400px 0; }
    100% { background-position: 400px 0; }
  }

  .skeleton-wrap {
    padding: 4px 0;
  }

  .sk-section-label,
  .sk-row,
  .sk-card {
    background: linear-gradient(
      90deg,
      var(--surface) 0%,
      var(--surface-2) 40%,
      var(--surface) 80%
    );
    background-size: 800px 100%;
    animation: shimmer 1.4s infinite linear;
    border-radius: 6px;
  }

  .sk-section-label {
    height: 10px;
    width: 80px;
    margin-bottom: 12px;
    border-radius: 4px;
  }
  .sk-row {
    height: 42px;
    margin-bottom: 6px;
    border-radius: 8px;
  }
  .sk-row--short { width: 80%; }
  .sk-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 12px;
  }
  .sk-card {
    aspect-ratio: 0.8;
    border-radius: 8px;
  }

  /* ── Recent artists ─────────────────────────────────────────── */
  .recent-section {
    padding: 4px 0 8px;
  }

  .recent-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 10px;
  }

  .recent-clear {
    font-size: 0.75rem;
    padding: 2px 6px;
    opacity: 0.7;
  }
  .recent-clear:hover { opacity: 1; }

  .recent-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .recent-row {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .recent-artist {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 7px 10px;
    border-radius: 8px;
    background: none;
    border: none;
    color: var(--text);
    text-align: left;
    cursor: pointer;
    min-width: 0;
    transition: background 0.1s;
  }
  .recent-artist:hover { background: var(--surface-2); }

  .recent-avatar {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    object-fit: cover;
    flex-shrink: 0;
  }

  .recent-monogram {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: var(--accent-dim);
    border: 1px solid var(--accent-glow);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.875rem;
    font-weight: 700;
    color: var(--accent);
    flex-shrink: 0;
  }

  .recent-name {
    flex: 1;
    font-size: 0.875rem;
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .recent-chevron {
    color: var(--text-2);
    opacity: 0.4;
    flex-shrink: 0;
  }
  .recent-artist:hover .recent-chevron { opacity: 0.8; }

  .recent-remove {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: 50%;
    color: var(--text-2);
    opacity: 0;
    flex-shrink: 0;
    transition: background 0.1s, opacity 0.15s, color 0.1s;
  }
  .recent-row:hover .recent-remove { opacity: 1; }
  .recent-remove:hover {
    background: var(--surface-2);
    color: var(--text);
  }

  /* ── Mobile ─────────────────────────────────────────────────── */
  @media (max-width: 640px) {
    .search-bar {
      height: 46px;
      padding: 0 14px;
      gap: 10px;
    }
    .bar-input { font-size: 0.9375rem; }

    /* Bottom sheet on mobile */
    .filter-sheet {
      left: 0;
      right: 0;
      bottom: 0;
      top: auto;
      transform: none;
      width: 100%;
      max-height: 85dvh;
      border-radius: 20px 20px 0 0;
      animation: slide-up 0.25s cubic-bezier(0.34, 1.2, 0.64, 1);
    }

    @keyframes slide-up {
      from { transform: translateY(60px); opacity: 0; }
      to { transform: translateY(0); opacity: 1; }
    }

    .album-grid {
      grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
      gap: 12px;
    }

    .sheet-row2 {
      grid-template-columns: 1fr;
    }

    .idle-state { padding: 48px 16px; }
    .no-results { padding: 48px 16px; }
  }
</style>
