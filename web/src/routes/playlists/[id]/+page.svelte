<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/stores";
  import { playlists as playlistApi } from "$lib/api/playlists";
  import { getPlaylistCoverGrid } from "$lib/api/playlists";
  import TrackList from "$lib/components/library/TrackList.svelte";
  import type { Playlist, Track } from "$lib/types";
  import { playTrack, shuffle } from "$lib/stores/player";
  import { downloadPlaylist, downloads } from "$lib/stores/offline/downloads";
  import Spinner from "$lib/components/ui/Spinner.svelte";
  import { exportM3U, exportXSPF } from "$lib/utils/playlistExport";
  import { getApiBase } from "$lib/api/base";
  import { authStore } from "$lib/stores/auth";
  import {
    parseM3U,
    parseXSPF,
    matchTracks,
    detectFormat,
  } from "$lib/utils/playlistImport";
  import { addToast } from "$lib/stores/ui/toast";

  let playlist: Playlist | null = null;
  let tracks: Track[] = [];

  let coverGrid: string[] = [];
  let loading = true;

  // ── Import state ──────────────────────────────────────────────────────────────
  let importFileInput: HTMLInputElement;
  let importPhase: "idle" | "matching" | "preview" | "importing" = "idle";
  let importResults: import("$lib/utils/playlistImport").MatchResult[] = [];
  let importProgress = 0;

  onMount(async () => {
    const id = String($page.params.id);
    try {
      const res = await playlistApi.get(id);
      playlist = res.playlist;
      tracks = res.tracks;
      try {
        coverGrid = await getPlaylistCoverGrid(id);
      } catch (e) {
        coverGrid = [];
      }
    } finally {
      loading = false;
    }
  });

  function playAll() {
    if ((tracks?.length ?? 0) > 0) playTrack(tracks[0], tracks);
  }

  function shuffleAll() {
    if ((tracks?.length ?? 0) === 0) return;
    shuffle.set(true);
    const idx = Math.floor(Math.random() * tracks.length);
    playTrack(tracks[idx], tracks);
  }

  async function togglePublic() {
    if (!playlist) return;
    const newVal = !playlist.is_public;
    playlist = { ...playlist, is_public: newVal };
    await playlistApi.update(String($page.params.id), { is_public: newVal });
  }

  let downloading = false;
  $: dlDoneCount = tracks.filter(
    (t) => $downloads.get(t.id)?.status === "done",
  ).length;
  $: allDownloaded = tracks.length > 0 && dlDoneCount === tracks.length;
  $: dlActiveCount = tracks.filter(
    (t) => $downloads.get(t.id)?.status === "downloading",
  ).length;

  async function downloadAll() {
    if (downloading || (tracks?.length ?? 0) === 0) return;
    downloading = true;
    try {
      await downloadPlaylist(tracks);
    } finally {
      downloading = false;
    }
  }

  // ── Export ────────────────────────────────────────────────────────────────────
  function doExportM3U() {
    if (!playlist || tracks.length === 0) return;
    exportM3U(playlist.name, tracks, getApiBase(), $authStore.token ?? "");
  }

  function doExportXSPF() {
    if (!playlist || tracks.length === 0) return;
    exportXSPF(playlist.name, tracks, getApiBase(), $authStore.token ?? "");
  }

  // ── Import ────────────────────────────────────────────────────────────────────
  async function handleImportFile(e: Event) {
    const file = (e.target as HTMLInputElement).files?.[0];
    if (!file) return;
    (e.target as HTMLInputElement).value = "";

    const text = await file.text();
    const fmt = detectFormat(file.name, text);
    if (!fmt) {
      addToast("Unsupported file format. Use .m3u or .xspf", "error");
      return;
    }

    const parsed = fmt === "m3u" ? parseM3U(text) : parseXSPF(text);
    if (parsed.length === 0) {
      addToast("No tracks found in file.", "error");
      return;
    }

    importPhase = "matching";
    importProgress = 0;

    // Match in batches, updating progress
    const batchSize = 5;
    importResults = new Array(parsed.length);
    for (let i = 0; i < parsed.length; i += batchSize) {
      const batch = parsed.slice(i, i + batchSize);
      const batchResults = await matchTracks(batch, batchSize);
      for (let j = 0; j < batchResults.length; j++) {
        importResults[i + j] = batchResults[j];
      }
      importProgress = Math.min(
        100,
        Math.round(((i + batchSize) / parsed.length) * 100),
      );
    }
    importPhase = "preview";
  }

  async function confirmImport() {
    if (!playlist) return;
    const matched = importResults.filter((r) => r.matched !== null);
    if (matched.length === 0) {
      addToast("No tracks matched.", "error");
      return;
    }

    importPhase = "importing";
    const id = String($page.params.id);
    let added = 0;
    for (const r of matched) {
      if (!r.matched) continue;
      try {
        await playlistApi.addTrack(id, r.matched.id);
        added++;
      } catch {
        /* skip duplicates / errors */
      }
    }
    // Refresh track list
    const res = await playlistApi.get(id);
    tracks = res.tracks;
    addToast(
      `Added ${added} track${added !== 1 ? "s" : ""} to playlist.`,
      "info",
    );
    importPhase = "idle";
    importResults = [];
  }

  function cancelImport() {
    importPhase = "idle";
    importResults = [];
  }

  $: matchedCount = importResults.filter((r) => r.matched !== null).length;
</script>

{#if loading}
  <p class="muted"><Spinner /></p>
{:else if playlist}
  <div class="header">
    <div class="cover-placeholder cover-grid">
      {#if coverGrid.length > 0}
        <div class="grid">
          {#each Array(4) as _, i}
            {#if coverGrid[i]}
              <img
                src={coverGrid[i]}
                alt="cover"
                class="grid-img"
                on:error={(e) => {
                  (e.currentTarget as HTMLImageElement).style.display = "none";
                }}
              />
            {:else}
              <span class="grid-fallback">♪</span>
            {/if}
          {/each}
        </div>
      {:else}
        <span class="grid-fallback">♪</span>
      {/if}
    </div>
    <div class="meta">
      <p class="type">Playlist</p>
      <h1 class="title">{playlist.name}</h1>
      {#if playlist.description}
        <p class="desc">{playlist.description}</p>
      {/if}
      <div class="actions">
        <button
          class="btn-play"
          on:click={playAll}
          disabled={(tracks?.length ?? 0) === 0}>▶ Play</button
        >
        <button
          class="btn-shuffle"
          on:click={shuffleAll}
          disabled={(tracks?.length ?? 0) === 0}
          title="Shuffle"
        >
          <svg
            width="15"
            height="15"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <polyline points="16 3 21 3 21 8" /><line
              x1="4"
              y1="20"
              x2="21"
              y2="3"
            />
            <polyline points="21 16 21 21 16 21" /><line
              x1="15"
              y1="15"
              x2="21"
              y2="21"
            />
            <line x1="4" y1="4" x2="9" y2="9" />
          </svg>
          Shuffle
        </button>
        <button
          class="btn-visibility"
          on:click={togglePublic}
          title={playlist.is_public ? "Make private" : "Make public"}
        >
          {#if playlist.is_public}
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <circle cx="12" cy="12" r="10" /><line
                x1="2"
                y1="12"
                x2="22"
                y2="12"
              />
              <path
                d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"
              />
            </svg>
            Public
          {:else}
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <rect x="3" y="11" width="18" height="11" rx="2" ry="2" /><path
                d="M7 11V7a5 5 0 0 1 10 0v4"
              />
            </svg>
            Private
          {/if}
        </button>
        <button
          class="btn-download"
          on:click={downloadAll}
          disabled={(tracks?.length ?? 0) === 0 || allDownloaded || downloading}
          title="Download all tracks for offline playback"
        >
          <svg
            width="15"
            height="15"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline
              points="7 10 12 15 17 10"
            /><line x1="12" y1="15" x2="12" y2="3" />
          </svg>
          {#if allDownloaded}Downloaded{:else if downloading || dlActiveCount > 0}{dlDoneCount}/{tracks.length}{:else}Download{/if}
        </button>
      </div>

      <!-- Export / Import row -->
      <div class="actions actions-secondary">
        <button
          class="btn-secondary"
          on:click={doExportM3U}
          disabled={tracks.length === 0}
          title="Export as M3U playlist file"
        >
          <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline
              points="7 10 12 15 17 10"
            /><line x1="12" y1="15" x2="12" y2="3" />
          </svg>
          M3U
        </button>
        <button
          class="btn-secondary"
          on:click={doExportXSPF}
          disabled={tracks.length === 0}
          title="Export as XSPF playlist file"
        >
          <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline
              points="7 10 12 15 17 10"
            /><line x1="12" y1="15" x2="12" y2="3" />
          </svg>
          XSPF
        </button>
        <button
          class="btn-secondary"
          on:click={() => importFileInput.click()}
          title="Import tracks from M3U or XSPF file"
        >
          <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline
              points="17 8 12 3 7 8"
            /><line x1="12" y1="3" x2="12" y2="15" />
          </svg>
          Import
        </button>
        <input
          bind:this={importFileInput}
          type="file"
          accept=".m3u,.m3u8,.xspf"
          class="hidden-input"
          on:change={handleImportFile}
        />
      </div>
    </div>
  </div>

  <!-- Import progress / preview panel -->
  {#if importPhase === "matching"}
    <div class="import-panel">
      <p class="import-status">Matching tracks… {importProgress}%</p>
      <div class="import-progress-bar">
        <div class="import-progress-fill" style="width:{importProgress}%"></div>
      </div>
    </div>
  {:else if importPhase === "preview"}
    <div class="import-panel">
      <p class="import-status">
        Matched <strong>{matchedCount}</strong> of
        <strong>{importResults.length}</strong> tracks from file.
      </p>
      <div class="import-preview">
        {#each importResults as r}
          <div class="import-row" class:unmatched={!r.matched}>
            <span class="import-status-icon">{r.matched ? "✓" : "✗"}</span>
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
          class="btn-play"
          on:click={confirmImport}
          disabled={matchedCount === 0}
          >Add {matchedCount} track{matchedCount !== 1 ? "s" : ""}</button
        >
        <button class="btn-secondary" on:click={cancelImport}>Cancel</button>
      </div>
    </div>
  {:else if importPhase === "importing"}
    <div class="import-panel">
      <p class="import-status"><Spinner /> Adding tracks…</p>
    </div>
  {/if}

  <TrackList {tracks} showCover={true} />
{/if}

<svelte:head>
  <title>{playlist ? `${playlist.name} – Orb` : "Playlist – Orb"}</title>
</svelte:head>

<style>
  .cover-grid {
    position: relative;
    width: 180px;
    height: 180px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg-hover);
    border-radius: 8px;
    overflow: hidden;
  }
  .cover-grid .grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    grid-template-rows: 1fr 1fr;
    width: 100%;
    height: 100%;
    gap: 0;
  }
  .cover-grid .grid-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    border-radius: 0;
    display: block;
  }
  .cover-grid .grid-fallback {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 2.5rem;
    color: var(--text-muted);
    background: var(--bg-hover);
  }
  .header {
    display: flex;
    gap: 24px;
    align-items: flex-end;
    margin-bottom: 32px;
  }
  .cover-placeholder {
    width: 180px;
    height: 180px;
    background: var(--bg-hover);
    border-radius: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 3rem;
    color: var(--text-muted);
    flex-shrink: 0;
  }
  .meta {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .type {
    font-size: 0.75rem;
    text-transform: uppercase;
    color: var(--text-muted);
  }
  .title {
    font-size: 2rem;
    font-weight: 700;
    margin: 0;
  }
  .desc {
    color: var(--text-muted);
    font-size: 0.875rem;
  }
  .actions {
    display: flex;
    gap: 8px;
    margin-top: 4px;
    align-items: center;
    flex-wrap: wrap;
  }
  .actions-secondary {
    margin-top: 0;
  }
  .btn-play {
    background: var(--accent);
    border: none;
    border-radius: 20px;
    padding: 8px 20px;
    color: #fff;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-play:hover {
    background: var(--accent-hover);
  }
  .btn-play:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  .btn-shuffle,
  .btn-visibility,
  .btn-download,
  .btn-secondary {
    display: flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 7px 16px;
    color: var(--text-muted);
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-shuffle:hover,
  .btn-visibility:hover,
  .btn-download:hover,
  .btn-secondary:hover {
    color: var(--text);
    border-color: var(--text);
  }
  .btn-shuffle:disabled,
  .btn-download:disabled,
  .btn-secondary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .hidden-input {
    display: none;
  }

  /* ── Import panel ────────────────────────────────────────── */
  .import-panel {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 16px;
    margin-bottom: 24px;
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
    max-height: 240px;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: 6px;
    margin: 8px 0 12px;
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
  .import-status-icon {
    flex-shrink: 0;
    width: 14px;
    color: var(--accent);
    font-size: 0.75rem;
  }
  .import-row.unmatched .import-status-icon {
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

  .muted {
    color: var(--text-muted);
  }
</style>
