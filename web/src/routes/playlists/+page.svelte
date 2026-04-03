<script lang="ts">
  import { onMount } from "svelte";
  import { playlists as playlistApi } from "$lib/api/playlists";
  import { getPlaylistCoverGrid } from "$lib/api/playlists";
  import { smartPlaylists as smartPlaylistApi } from "$lib/api/smartPlaylists";
  import PlaylistCard from "$lib/components/playlist/PlaylistCard.svelte";
  import SmartPlaylistCard from "$lib/components/playlist/SmartPlaylistCard.svelte";
  import type { Playlist, SmartPlaylist } from "$lib/types";
  import { goto } from "$app/navigation";
  import Spinner from "$lib/components/ui/Spinner.svelte";
  import { addToast } from "$lib/stores/ui/toast";
  import {
    parseM3U,
    parseXSPF,
    matchTracks,
    detectFormat,
  } from "$lib/utils/playlistImport";
  import {
    startSpotifyLogin,
    handleSpotifyReturn,
    fetchUserPlaylists,
    fetchPlaylistTracks,
    matchSpotifyTracks,
    isSpotifyAuthorized,
    clearSpotifySession,
    isSpotifyConfigured,
  } from "$lib/utils/spotifyImport";

  let items: Playlist[] = [];
  let smartItems: SmartPlaylist[] = [];
  let coverGrids: Record<string, string[]> = {};
  let loading = true;
  let creating = false;
  let newName = "";
  let showSmartForm = false;
  let smartName = "";
  let isRestoring = false;

  // Whether the server has SPOTIFY_CLIENT_ID configured
  let spotifyEnabled = false;

  export const snapshot = {
    capture: () => ({ items, smartItems, coverGrids }),
    restore: (value) => {
      items = value.items;
      smartItems = value.smartItems;
      coverGrids = value.coverGrids;
      isRestoring = true;
      loading = false;
    },
  };

  $: systemSmartPlaylists = smartItems.filter((p) => p.system);
  $: userSmartPlaylists = smartItems.filter((p) => !p.system);

  // ── M3U/XSPF import state ─────────────────────────────────────────────────────
  let fileImportInput: HTMLInputElement;
  let fileImportPhase: "idle" | "matching" | "preview" | "creating" = "idle";
  let fileImportResults: import("$lib/utils/playlistImport").MatchResult[] = [];
  let fileImportProgress = 0;
  let fileImportName = "";

  // ── Spotify import state ──────────────────────────────────────────────────────
  type SpotifyPhase = "idle" | "picking" | "matching" | "preview" | "creating";
  let spotifyPhase: SpotifyPhase = "idle";
  let spotifyPlaylists: import("$lib/utils/spotifyImport").SpotifyPlaylistSummary[] =
    [];
  let spotifySelectedId = "";
  let spotifyResults: import("$lib/utils/spotifyImport").SpotifyMatchResult[] =
    [];
  let spotifyMatchProgress = 0;

  onMount(async () => {
    // Check server Spotify config (non-blocking)
    isSpotifyConfigured().then((v) => {
      spotifyEnabled = v;
    });

    // Handle Spotify OAuth return (#spotify_token=... in fragment)
    try {
      const returned = handleSpotifyReturn();
      if (returned) {
        spotifyPhase = "picking";
        spotifyPlaylists = await fetchUserPlaylists();
      }
    } catch (err: unknown) {
      addToast(
        err instanceof Error ? err.message : "Spotify auth failed",
        "error",
      );
    }

    if (isRestoring && (items.length > 0 || smartItems.length > 0)) {
      loading = false;
      return;
    }

    try {
      const [pls, spls] = await Promise.all([
        playlistApi.list(),
        smartPlaylistApi.list(),
      ]);
      items = pls;
      smartItems = spls;

      await Promise.all(
        items.map(async (pl) => {
          try {
            coverGrids[pl.id] = await getPlaylistCoverGrid(pl.id);
          } catch {
            coverGrids[pl.id] = [];
          }
        }),
      );
    } finally {
      loading = false;
    }
  });

  async function createPlaylist(e: Event) {
    e.preventDefault();
    if (!newName.trim()) return;
    creating = true;
    try {
      const pl = await playlistApi.create(newName.trim());
      items = [...items, pl];
      coverGrids[pl.id] = [];
      newName = "";
    } finally {
      creating = false;
    }
  }

  async function createSmartPlaylist(e: Event) {
    e.preventDefault();
    if (!smartName.trim()) return;
    creating = true;
    try {
      const pl = await smartPlaylistApi.create({ name: smartName.trim() });
      if (pl) goto(`/smart-playlists/${pl.id}`);
    } finally {
      creating = false;
    }
  }

  // ── M3U / XSPF import ─────────────────────────────────────────────────────────
  async function handleFileImport(e: Event) {
    const file = (e.target as HTMLInputElement).files?.[0];
    if (!file) return;
    (e.target as HTMLInputElement).value = "";

    const text = await file.text();
    const fmt = detectFormat(file.name, text);
    if (!fmt) {
      addToast("Unsupported file. Use .m3u or .xspf", "error");
      return;
    }

    fileImportName = file.name.replace(/\.[^.]+$/, "");
    const parsed = fmt === "m3u" ? parseM3U(text) : parseXSPF(text);
    if (parsed.length === 0) {
      addToast("No tracks found in file.", "error");
      return;
    }

    fileImportPhase = "matching";
    fileImportProgress = 0;
    fileImportResults = [];

    const batchSize = 5;
    const results: import("$lib/utils/playlistImport").MatchResult[] =
      new Array(parsed.length);
    for (let i = 0; i < parsed.length; i += batchSize) {
      const batchRes = await matchTracks(
        parsed.slice(i, i + batchSize),
        batchSize,
      );
      for (let j = 0; j < batchRes.length; j++) results[i + j] = batchRes[j];
      fileImportProgress = Math.min(
        100,
        Math.round(((i + batchSize) / parsed.length) * 100),
      );
    }
    fileImportResults = results;
    fileImportPhase = "preview";
  }

  $: fileMatchedCount = fileImportResults.filter(
    (r) => r.matched !== null,
  ).length;

  async function confirmFileImport() {
    const matched = fileImportResults.filter((r) => r.matched !== null);
    if (matched.length === 0) return;
    fileImportPhase = "creating";
    const pl = await playlistApi.create(fileImportName || "Imported Playlist");
    for (const r of matched) {
      if (!r.matched) continue;
      try {
        await playlistApi.addTrack(pl.id, r.matched.id);
      } catch {
        /* skip */
      }
    }
    items = [...items, pl];
    coverGrids[pl.id] = [];
    addToast(
      `Created "${pl.name}" with ${matched.length} track${matched.length !== 1 ? "s" : ""}.`,
      "info",
    );
    fileImportPhase = "idle";
    fileImportResults = [];
    goto(`/playlists/${pl.id}`);
  }

  // ── Spotify import ────────────────────────────────────────────────────────────
  async function connectSpotify() {
    if (isSpotifyAuthorized()) {
      spotifyPhase = "picking";
      spotifyPlaylists = await fetchUserPlaylists();
    } else {
      startSpotifyLogin(); // full-page redirect to /auth/spotify
    }
  }

  async function selectSpotifyPlaylist() {
    if (!spotifySelectedId) return;
    spotifyPhase = "matching";
    spotifyMatchProgress = 0;
    const tracks = await fetchPlaylistTracks(spotifySelectedId);
    const batchSize = 5;
    const results: import("$lib/utils/spotifyImport").SpotifyMatchResult[] =
      new Array(tracks.length);
    for (let i = 0; i < tracks.length; i += batchSize) {
      const batchRes = await matchSpotifyTracks(
        tracks.slice(i, i + batchSize),
        batchSize,
      );
      for (let j = 0; j < batchRes.length; j++) results[i + j] = batchRes[j];
      spotifyMatchProgress = Math.min(
        100,
        Math.round(((i + batchSize) / tracks.length) * 100),
      );
    }
    spotifyResults = results;
    spotifyPhase = "preview";
  }

  $: spotifyMatchedCount = spotifyResults.filter(
    (r) => r.matched !== null,
  ).length;

  async function confirmSpotifyImport() {
    const matched = spotifyResults.filter((r) => r.matched !== null);
    if (matched.length === 0) return;
    const name =
      spotifyPlaylists.find((p) => p.id === spotifySelectedId)?.name ??
      "Spotify Import";
    spotifyPhase = "creating";
    const pl = await playlistApi.create(name);
    for (const r of matched) {
      if (!r.matched) continue;
      try {
        await playlistApi.addTrack(pl.id, r.matched.id);
      } catch {
        /* skip */
      }
    }
    items = [...items, pl];
    coverGrids[pl.id] = [];
    addToast(
      `Created "${pl.name}" with ${matched.length} track${matched.length !== 1 ? "s" : ""}.`,
      "info",
    );
    clearSpotifySession();
    spotifyPhase = "idle";
    spotifyResults = [];
    goto(`/playlists/${pl.id}`);
  }

  function cancelFileImport() {
    fileImportPhase = "idle";
    fileImportResults = [];
  }
  function cancelSpotify() {
    spotifyPhase = "idle";
    spotifyResults = [];
    clearSpotifySession();
  }
</script>

<div class="playlists-page">
  <div class="header">
    <h2 class="title">Playlists</h2>
    <div class="header-actions">
      <button
        class="btn-secondary"
        on:click={() => fileImportInput.click()}
        title="Import M3U or XSPF playlist"
      >
        Import
      </button>
      {#if spotifyEnabled}
        <button
          class="btn-secondary"
          on:click={connectSpotify}
          title="Import from Spotify"
        >
          Spotify
        </button>
      {/if}
      <button
        class="btn-secondary"
        on:click={() => (showSmartForm = !showSmartForm)}
      >
        {showSmartForm ? "Cancel" : "+ Smart"}
      </button>
    </div>
  </div>

  <input
    bind:this={fileImportInput}
    type="file"
    accept=".m3u,.m3u8,.xspf"
    class="hidden-input"
    on:change={handleFileImport}
  />

  <!-- M3U/XSPF import panel -->
  {#if fileImportPhase === "matching"}
    <div class="import-panel">
      <p class="import-status">Matching tracks… {fileImportProgress}%</p>
      <div class="import-progress-bar">
        <div
          class="import-progress-fill"
          style="width:{fileImportProgress}%"
        ></div>
      </div>
    </div>
  {:else if fileImportPhase === "preview"}
    <div class="import-panel">
      <p class="import-status">
        Matched <strong>{fileMatchedCount}</strong> of
        <strong>{fileImportResults.length}</strong> tracks.
      </p>
      <div class="import-preview">
        {#each fileImportResults as r}
          <div class="import-row" class:unmatched={!r.matched}>
            <span class="import-icon">{r.matched ? "✓" : "✗"}</span>
            <span class="import-source"
              >{r.parsed.artist ? `${r.parsed.artist} – ` : ""}{r.parsed
                .title}</span
            >
            {#if r.matched}
              <span class="import-match"
                >→ {r.matched.artist_name
                  ? `${r.matched.artist_name} – `
                  : ""}{r.matched.title}</span
              >
            {:else}
              <span class="import-nomatch">not found</span>
            {/if}
          </div>
        {/each}
      </div>
      <div class="import-actions">
        <button
          class="btn-create"
          on:click={confirmFileImport}
          disabled={fileMatchedCount === 0}
        >
          Create playlist ({fileMatchedCount} track{fileMatchedCount !== 1
            ? "s"
            : ""})
        </button>
        <button class="btn-secondary" on:click={cancelFileImport}>Cancel</button
        >
      </div>
    </div>
  {:else if fileImportPhase === "creating"}
    <div class="import-panel">
      <p class="import-status"><Spinner /> Creating playlist…</p>
    </div>
  {/if}

  <!-- Spotify import panel -->
  {#if spotifyPhase === "picking"}
    <div class="import-panel">
      <p class="import-status">Choose a Spotify playlist to import</p>
      <select class="spotify-select" bind:value={spotifySelectedId}>
        <option value="">— select —</option>
        {#each spotifyPlaylists as pl}
          <option value={pl.id}>{pl.name} ({pl.trackCount} tracks)</option>
        {/each}
      </select>
      <div class="import-actions">
        <button
          class="btn-create"
          on:click={selectSpotifyPlaylist}
          disabled={!spotifySelectedId}>Import</button
        >
        <button class="btn-secondary" on:click={cancelSpotify}>Cancel</button>
      </div>
    </div>
  {:else if spotifyPhase === "matching"}
    <div class="import-panel">
      <p class="import-status">
        Matching Spotify tracks… {spotifyMatchProgress}%
      </p>
      <div class="import-progress-bar">
        <div
          class="import-progress-fill"
          style="width:{spotifyMatchProgress}%"
        ></div>
      </div>
    </div>
  {:else if spotifyPhase === "preview"}
    <div class="import-panel">
      <p class="import-status">
        Matched <strong>{spotifyMatchedCount}</strong> of
        <strong>{spotifyResults.length}</strong> Spotify tracks.
      </p>
      <div class="import-preview">
        {#each spotifyResults as r}
          <div class="import-row" class:unmatched={!r.matched}>
            <span class="import-icon">{r.matched ? "✓" : "✗"}</span>
            <span class="import-source"
              >{r.spotify.artist ? `${r.spotify.artist} – ` : ""}{r.spotify
                .title}</span
            >
            {#if r.matched}
              <span class="import-match"
                >→ {r.matched.artist_name
                  ? `${r.matched.artist_name} – `
                  : ""}{r.matched.title}</span
              >
            {:else}
              <span class="import-nomatch">not in library</span>
            {/if}
          </div>
        {/each}
      </div>
      <div class="import-actions">
        <button
          class="btn-create"
          on:click={confirmSpotifyImport}
          disabled={spotifyMatchedCount === 0}
        >
          Create playlist ({spotifyMatchedCount} track{spotifyMatchedCount !== 1
            ? "s"
            : ""})
        </button>
        <button class="btn-secondary" on:click={cancelSpotify}>Cancel</button>
      </div>
    </div>
  {:else if spotifyPhase === "creating"}
    <div class="import-panel">
      <p class="import-status"><Spinner /> Creating playlist…</p>
    </div>
  {/if}

  {#if showSmartForm}
    <form class="create-form" on:submit={createSmartPlaylist}>
      <input
        type="text"
        placeholder="New smart playlist name…"
        bind:value={smartName}
        class="create-input"
      />
      <button
        type="submit"
        class="btn-create"
        disabled={creating || !smartName.trim()}>Create</button
      >
    </form>
  {:else}
    <form class="create-form" on:submit={createPlaylist}>
      <input
        type="text"
        placeholder="New playlist name…"
        bind:value={newName}
        class="create-input"
      />
      <button
        type="submit"
        class="btn-create"
        disabled={creating || !newName.trim()}>Create</button
      >
    </form>
  {/if}

  {#if loading}
    <p class="muted"><Spinner /></p>
  {:else}
    {#if systemSmartPlaylists.length > 0}
      <div class="section-label">Auto-Generated</div>
      <div class="list">
        {#each systemSmartPlaylists as pl (pl.id)}
          <SmartPlaylistCard playlist={pl} />
        {/each}
      </div>
    {/if}

    {#if userSmartPlaylists.length > 0}
      <div class="section-label" class:mt-24={systemSmartPlaylists.length > 0}>
        Smart Playlists
      </div>
      <div class="list">
        {#each userSmartPlaylists as pl (pl.id)}
          <SmartPlaylistCard playlist={pl} />
        {/each}
      </div>
    {/if}

    <div class="section-label" class:mt-24={smartItems.length > 0}>
      My Playlists
    </div>
    {#if items.length === 0}
      <p class="muted">No manual playlists yet</p>
    {:else}
      <div class="list">
        {#each items as pl (pl.id)}
          <PlaylistCard playlist={pl} coverGrid={coverGrids[pl.id]} />
        {/each}
      </div>
    {/if}
  {/if}
</div>

<svelte:head><title>Playlists – Orb</title></svelte:head>

<style>
  .playlists-page {
    max-width: 800px;
  }
  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 20px;
  }
  .title {
    font-size: 1.25rem;
    font-weight: 600;
    margin: 0;
  }
  .header-actions {
    display: flex;
    gap: 8px;
  }

  .hidden-input {
    display: none;
  }

  .create-form {
    display: flex;
    gap: 8px;
    margin-bottom: 24px;
  }
  .create-input {
    flex: 1;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    color: var(--text);
    font-size: 0.875rem;
    outline: none;
  }
  .create-input:focus {
    border-color: var(--accent);
  }
  .btn-create {
    background: var(--accent);
    border: none;
    border-radius: 6px;
    padding: 8px 16px;
    color: #fff;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    white-space: nowrap;
  }
  .btn-create:hover {
    background: var(--accent-hover);
  }
  .btn-create:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .btn-secondary {
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 12px;
    color: var(--text-muted);
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    white-space: nowrap;
  }
  .btn-secondary:hover {
    color: var(--text);
    border-color: var(--text-muted);
  }

  .section-label {
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
    margin-bottom: 8px;
  }
  .mt-24 {
    margin-top: 24px;
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .muted {
    color: var(--text-muted);
    font-size: 0.875rem;
    padding: 0 12px;
  }

  /* ── Import / Spotify panels ─────────────────────────────── */
  .import-panel {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 16px;
    margin-bottom: 20px;
  }
  .import-status {
    margin: 0 0 8px;
    font-size: 0.875rem;
    color: var(--text-muted);
  }
  .import-progress-bar {
    height: 4px;
    background: var(--bg-hover);
    border-radius: 2px;
    overflow: hidden;
  }
  .import-progress-fill {
    height: 100%;
    background: var(--accent);
    transition: width 0.2s;
  }
  .import-preview {
    max-height: 220px;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: 6px;
    margin: 10px 0 12px;
  }
  .import-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 5px 10px;
    font-size: 0.78rem;
    border-bottom: 1px solid var(--border);
  }
  .import-row:last-child {
    border-bottom: none;
  }
  .import-row.unmatched {
    opacity: 0.5;
  }
  .import-icon {
    flex-shrink: 0;
    width: 14px;
    color: var(--accent);
    font-size: 0.75rem;
  }
  .import-row.unmatched .import-icon {
    color: var(--text-muted);
  }
  .import-source {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
  }
  .import-match {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-muted);
  }
  .import-nomatch {
    color: var(--text-muted);
    font-style: italic;
  }
  .import-actions {
    display: flex;
    gap: 8px;
    margin-top: 4px;
  }
  .spotify-select {
    width: 100%;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    color: var(--text);
    font-size: 0.875rem;
    outline: none;
    margin-bottom: 10px;
  }
  .spotify-select:focus {
    border-color: var(--accent);
  }
</style>
