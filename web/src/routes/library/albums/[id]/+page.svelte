<script lang="ts">
  import { page } from "$app/stores";
  import { goto } from "$app/navigation";
  import { library as libApi } from "$lib/api/library";
  import { admin as adminApi } from "$lib/api/admin";
  import { recommend } from "$lib/api/recommend";
  import { authStore } from "$lib/stores/auth";
  import TrackList from "$lib/components/library/TrackList.svelte";
  import type { Album, Track, Genre } from "$lib/types";
  import {
    playTrack,
    shuffle,
    startRadio,
    currentTrack,
    playbackState,
    togglePlayPause,
  } from "$lib/stores/player";
  import { downloadAlbum, downloads } from "$lib/stores/offline/downloads";
  import { getApiBase } from "$lib/api/base";
  import Spinner from "$lib/components/ui/Spinner.svelte";

  interface SimilarAlbum {
    id: string;
    title: string;
    cover_art_key?: string;
    artist_name?: string;
  }

  let album: Album | null = null;
  let tracks: Track[] = [];
  let genres: Genre[] = [];
  let variants: Album[] = [];
  let artistName: string | null = null;
  let artistId: string | null = null;
  let similarAlbums: SimilarAlbum[] = [];
  let loading = true;
  let isRestoring = false;

  $: isAlbumActive = album?.id === $currentTrack?.album_id;
  $: isPlayingGlobal = $playbackState === "playing";
  $: isPausedThisAlbum = isAlbumActive && $playbackState === "paused";

  export const snapshot = {
    capture: () => ({
      album,
      tracks,
      genres,
      variants,
      artistName,
      artistId,
      similarAlbums,
    }),
    restore: (value) => {
      album = value.album;
      tracks = value.tracks;
      genres = value.genres;
      variants = value.variants;
      artistName = value.artistName;
      artistId = value.artistId;
      similarAlbums = value.similarAlbums ?? [];
      isRestoring = true;
      loading = false;
    },
  };

  $: isAdmin = $authStore.user?.is_admin === true;

  async function loadAlbum(id: string) {
    if (isRestoring && album?.id === id) {
      loading = false;
      isRestoring = false;
      return;
    }
    loading = true;
    album = null;
    tracks = [];
    genres = [];
    variants = [];
    artistName = null;
    artistId = null;
    similarAlbums = [];
    try {
      const res = await libApi.album(id);
      album = res.album;
      tracks = res.tracks;
      genres = res.genres ?? [];
      variants = res.variants ?? [];
      if (res.artist) {
        artistName = res.artist.name;
        artistId = res.artist.id;
      }
      // Fetch similar albums in the background using the first track as seed
      if (res.tracks.length > 0) {
        recommend
          .similar(res.tracks[0].id, 30, id)
          .then((simTracks) => {
            const seen = new Set<string>();
            const result: SimilarAlbum[] = [];
            for (const t of simTracks) {
              if (!t.album_id || seen.has(t.album_id)) continue;
              seen.add(t.album_id);
              result.push({
                id: t.album_id,
                title: t.album_name ?? "Unknown Album",
                cover_art_key: t.cover_art_key,
                artist_name: t.artist_name,
              });
            }
            similarAlbums = result.slice(0, 12);
          })
          .catch(() => {});
      }
    } finally {
      loading = false;
      isRestoring = false;
    }
  }

  $: if ($page.params.id) loadAlbum($page.params.id);

  function playAll() {
    if (tracks.length > 0) playTrack(tracks[0], tracks);
  }
  function shuffleAll() {
    if (tracks.length === 0) return;
    shuffle.set(true);
    playTrack(tracks[Math.floor(Math.random() * tracks.length)], tracks);
  }

  let radioLoading = false;
  async function startAlbumRadio() {
    if (tracks.length === 0 || radioLoading) return;
    radioLoading = true;
    try {
      await startRadio(tracks[0].id);
    } finally {
      radioLoading = false;
    }
  }

  $: discCount = new Set(tracks.map((t) => t.disc_number ?? 1)).size;

  let downloading = false;
  $: dlDoneCount = tracks.filter(
    (t) => $downloads.get(t.id)?.status === "done",
  ).length;
  $: allDownloaded = tracks.length > 0 && dlDoneCount === tracks.length;
  $: dlActiveCount = tracks.filter(
    (t) => $downloads.get(t.id)?.status === "downloading",
  ).length;

  async function downloadAll() {
    if (downloading || tracks.length === 0) return;
    downloading = true;
    try {
      await downloadAlbum(tracks);
    } finally {
      downloading = false;
    }
  }

  // ── Inline editing ────────────────────────────────────────────────────────
  type EditField = "title" | "release_year" | "label";
  let editingField: EditField | null = null;
  let editValue = "";
  let saving = false;
  let saveError = "";

  function startEdit(field: EditField) {
    if (!isAdmin || !album) return;
    editingField = field;
    saveError = "";
    switch (field) {
      case "title":
        editValue = album.title;
        break;
      case "release_year":
        editValue =
          album.release_year != null ? String(album.release_year) : "";
        break;
      case "label":
        editValue = (album as any).label ?? "";
        break;
    }
  }

  function handleEditTriggerKey(e: KeyboardEvent, field: EditField) {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      startEdit(field);
    }
  }

  function cancelEdit() {
    editingField = null;
    editValue = "";
    saveError = "";
  }

  async function commitEdit() {
    const albumId = $page.params.id;
    if (!album || !editingField || saving || !albumId) return;
    saving = true;
    saveError = "";
    try {
      const body: {
        title: string;
        release_year: number | null;
        label: string | null;
      } = {
        title: album.title,
        release_year: album.release_year ?? null,
        label: ((album as any).label ?? null) as string | null,
      };
      switch (editingField) {
        case "title":
          if (!editValue.trim()) {
            saveError = "Title cannot be empty";
            saving = false;
            return;
          }
          body.title = editValue.trim();
          break;
        case "release_year": {
          const trimmedYear = String(editValue ?? "").trim();
          body.release_year = trimmedYear ? parseInt(trimmedYear, 10) : null;
          break;
        }
        case "label":
          body.label = editValue.trim() || null;
          break;
      }
      await adminApi.updateAlbumMeta(albumId, body);
      album = {
        ...album,
        title: body.title,
        release_year: body.release_year ?? undefined,
        label: body.label ?? undefined,
      };
      editingField = null;
    } catch (e) {
      saveError = e instanceof Error ? e.message : "Save failed";
    } finally {
      saving = false;
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Enter") {
      e.preventDefault();
      commitEdit();
    } else if (e.key === "Escape") cancelEdit();
  }

  function focusOnMount(node: HTMLElement) {
    setTimeout(() => {
      node.focus();
      if (node instanceof HTMLInputElement) node.select();
    }, 0);
  }

  // ── Re-fetch cover / refresh ──────────────────────────────────────────────
  let refreshing = false;
  let refreshMsg = "";

  async function handleRefetchCover() {
    const albumId = $page.params.id;
    if (refreshing || !album || !albumId) return;
    refreshing = true;
    refreshMsg = "";
    try {
      await adminApi.refetchAlbumCover(albumId);
      refreshMsg = "Cover refreshed";
      // reload album to show new cover
      const res = await libApi.album(albumId);
      album = res.album;
    } catch (e) {
      refreshMsg = e instanceof Error ? e.message : "Cover fetch failed";
    } finally {
      refreshing = false;
      setTimeout(() => {
        refreshMsg = "";
      }, 3000);
    }
  }

  let scanning = false;
  let scanMsg = "";

  async function handleRescan() {
    const albumId = $page.params.id;
    if (scanning || !albumId) return;
    scanning = true;
    scanMsg = "";
    try {
      await adminApi.reingestAlbum(albumId);
      scanMsg = "Reingest started";
    } catch (e) {
      scanMsg = e instanceof Error ? e.message : "Scan failed";
    } finally {
      scanning = false;
      setTimeout(() => {
        scanMsg = "";
      }, 3000);
    }
  }
</script>

{#if loading}
  <div class="hero-skeleton">
    <div class="sk-cover"></div>
    <div class="sk-info">
      <div class="sk-line sk-type"></div>
      <div class="sk-line sk-title"></div>
      <div class="sk-line sk-artist"></div>
      <div class="sk-line sk-meta"></div>
    </div>
  </div>
{:else if album}
  <!-- ── Hero ─────────────────────────────────────────────────────── -->
  <div class="hero">
    {#if album.cover_art_key}
      <div class="hero-bg" aria-hidden="true">
        <img src="{getApiBase()}/covers/{album.id}" alt="" class="hero-bg-img" />
      </div>
    {/if}
    <div class="hero-body">
      <div class="cover-wrap">
        {#if album.cover_art_key}
          <img src="{getApiBase()}/covers/{album.id}" alt={album.title} class="cover" />
        {:else}
          <div class="cover cover-fallback">♪</div>
        {/if}
      </div>

      <div class="hero-info">
        <p class="type-badge">{album.album_type ?? "Album"}</p>

        <h1 class="hero-title" class:editable={isAdmin}>
          {#if editingField === "title"}
            <input class="inline-input title-input" bind:value={editValue} on:keydown={onKeydown} on:blur={commitEdit} disabled={saving} use:focusOnMount />
          {:else if isAdmin}
            <button type="button" class="editable-trigger" on:click={() => startEdit("title")} aria-label="Edit album title">{album.title}<span class="edit-hint">✎</span></button>
          {:else}
            {album.title}
          {/if}
        </h1>

        {#if artistName}
          {#if artistId}
            <a href="/artists/{artistId}" class="hero-artist">{artistName}</a>
          {:else}
            <p class="hero-artist">{artistName}</p>
          {/if}
        {/if}

        <div class="hero-meta-row">
          {#if editingField === "release_year"}
            <input class="inline-input year-input" type="number" bind:value={editValue} on:keydown={onKeydown} on:blur={commitEdit} disabled={saving} placeholder="Year" use:focusOnMount />
          {:else if isAdmin}
            <button type="button" class="editable-trigger meta-val" on:click={() => startEdit("release_year")} aria-label="Edit release year">{album.release_year ?? "—"}<span class="edit-hint">✎</span></button>
          {:else}
            <span class="meta-val">{album.release_year ?? "—"}</span>
          {/if}
          {#if discCount > 1}<span class="meta-sep">·</span><span class="meta-val">{discCount} discs</span>{/if}
          <span class="meta-sep">·</span>
          {#if editingField === "label"}
            <input class="inline-input label-input" bind:value={editValue} on:keydown={onKeydown} on:blur={commitEdit} disabled={saving} placeholder="Label" use:focusOnMount />
          {:else if isAdmin}
            <button type="button" class="editable-trigger meta-val" on:click={() => startEdit("label")} aria-label="Edit label">{(album as any).label ?? "—"}<span class="edit-hint">✎</span></button>
          {:else}
            <span class="meta-val">{(album as any).label ?? "—"}</span>
          {/if}
          <span class="meta-sep">·</span>
          <span class="meta-val">{tracks.length} track{tracks.length !== 1 ? "s" : ""}</span>
        </div>

        {#if saveError}<p class="save-error">{saveError}</p>{/if}

        {#if genres.length > 0}
          <div class="genre-pills">
            {#each genres as genre}
              <a href="/genres/{genre.id}" class="genre-pill">{genre.name}</a>
            {/each}
          </div>
        {/if}

        <div class="hero-actions">
          <button class="btn-play" on:click={isPlayingGlobal && isAlbumActive || isPausedThisAlbum ? togglePlayPause : playAll} disabled={tracks.length === 0}>
            {#if isPlayingGlobal && isAlbumActive}
              <svg width="15" height="15" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="4" width="4" height="16" rx="1" /><rect x="14" y="4" width="4" height="16" rx="1" /></svg> Pause
            {:else if isPausedThisAlbum}
              <svg width="15" height="15" viewBox="0 0 24 24" fill="currentColor"><polygon points="5,3 19,12 5,21" /></svg> Resume
            {:else}
              <svg width="15" height="15" viewBox="0 0 24 24" fill="currentColor"><polygon points="5,3 19,12 5,21" /></svg> Play
            {/if}
          </button>
          <button class="btn-icon" on:click={shuffleAll} disabled={tracks.length === 0} title="Shuffle">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 3 21 3 21 8" /><line x1="4" y1="20" x2="21" y2="3" /><polyline points="21 16 21 21 16 21" /><line x1="15" y1="15" x2="21" y2="21" /><line x1="4" y1="4" x2="9" y2="9" /></svg>
          </button>
          <button class="btn-icon btn-icon--accent" on:click={startAlbumRadio} disabled={tracks.length === 0 || radioLoading} title="Start Radio">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="2" /><path d="M16.24 7.76a6 6 0 0 1 0 8.49m-8.48-.01a6 6 0 0 1 0-8.49m11.31-2.82a10 10 0 0 1 0 14.14m-14.14 0a10 10 0 0 1 0-14.14" /></svg>
          </button>
          <button class="btn-icon" class:btn-icon--done={allDownloaded} on:click={downloadAll} disabled={tracks.length === 0 || allDownloaded || downloading} title={allDownloaded ? "Downloaded" : "Download all"}>
            {#if downloading || dlActiveCount > 0}
              <svg class="spin-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><circle cx="12" cy="12" r="9" stroke-dasharray="44 13" /></svg>
            {:else}
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" y1="15" x2="12" y2="3" /></svg>
            {/if}
          </button>
          {#if isAdmin}
            <button class="btn-admin" on:click={handleRefetchCover} disabled={refreshing} title="Refresh cover art">
              {#if refreshing}<svg class="spin-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><circle cx="12" cy="12" r="9" stroke-dasharray="44 13" /></svg>{:else}<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" /><circle cx="8.5" cy="8.5" r="1.5" /><polyline points="21 15 16 10 5 21" /></svg>{/if}
              Cover
            </button>
            <button class="btn-admin" on:click={handleRescan} disabled={scanning} title="Force rescan">
              {#if scanning}<svg class="spin-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><circle cx="12" cy="12" r="9" stroke-dasharray="44 13" /></svg>{:else}<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M23 4v6h-6" /><path d="M1 20v-6h6" /><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" /></svg>{/if}
              Rescan
            </button>
          {/if}
        </div>

        {#if refreshMsg || scanMsg}
          <p class="action-msg" class:error-msg={refreshMsg.includes("fail") || scanMsg.includes("fail")}>{refreshMsg || scanMsg}</p>
        {/if}
      </div>
    </div>
  </div>

  {#if variants.length > 1}
    <div class="variant-picker">
      <span class="variant-label">Versions</span>
      {#each variants as v}
        <a href="/library/albums/{v.id}" class="variant-pill" class:active={v.id === album.id}>
          <span>{v.edition ?? "Standard"}</span>
          <span class="variant-count">{v.track_count ?? 0} tracks</span>
        </a>
      {/each}
    </div>
  {/if}

  <TrackList {tracks} />

  {#if similarAlbums.length > 0}
    <section class="similar-section">
      <h2 class="section-label">Similar Albums</h2>
      <div class="carousel">
        {#each similarAlbums as sa (sa.id)}
          <button class="carousel-card" on:click={() => goto(`/library/albums/${sa.id}`)} aria-label="Open {sa.title}">
            <div class="carousel-cover-wrap">
              {#if sa.cover_art_key}
                <img src="{getApiBase()}/covers/{sa.id}" alt={sa.title} class="carousel-cover" loading="lazy" />
              {:else}
                <div class="carousel-cover carousel-placeholder">♪</div>
              {/if}
            </div>
            <span class="carousel-name" title={sa.title}>{sa.title}</span>
            {#if sa.artist_name}<span class="carousel-artist">{sa.artist_name}</span>{/if}
          </button>
        {/each}
      </div>
    </section>
  {/if}
{/if}

<svelte:head>
  <title>{album ? `${album.title} – Orb` : "Album – Orb"}</title>
</svelte:head>

<style>
  /* ── Hero skeleton ── */
  .hero-skeleton {
    display: flex;
    gap: 24px;
    align-items: flex-end;
    padding: 32px 0 28px;
    margin-bottom: 8px;
  }
  .sk-cover {
    width: 200px;
    height: 200px;
    border-radius: 8px;
    background: var(--bg-elevated);
    flex-shrink: 0;
    animation: sk-pulse 1.4s ease-in-out infinite;
  }
  .sk-info {
    display: flex;
    flex-direction: column;
    gap: 10px;
    flex: 1;
  }
  .sk-line {
    border-radius: 4px;
    background: var(--bg-elevated);
    animation: sk-pulse 1.4s ease-in-out infinite;
  }
  .sk-type  { height: 10px; width: 60px; }
  .sk-title { height: 28px; width: min(340px, 80%); }
  .sk-artist{ height: 14px; width: min(180px, 50%); }
  .sk-meta  { height: 12px; width: min(240px, 65%); }
  @keyframes sk-pulse {
    0%, 100% { opacity: 0.5; }
    50%       { opacity: 1;   }
  }

  /* ── Hero ── */
  .hero {
    position: relative;
    border-radius: 12px;
    overflow: hidden;
    margin-bottom: 28px;
  }
  .hero-bg {
    position: absolute;
    inset: 0;
    z-index: 0;
    overflow: hidden;
  }
  .hero-bg-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    filter: blur(40px) saturate(1.4) brightness(0.55);
    transform: scale(1.1);
  }
  .hero-body {
    position: relative;
    z-index: 1;
    display: flex;
    gap: 28px;
    align-items: flex-end;
    padding: 32px 28px 28px;
    background: linear-gradient(
      to bottom,
      transparent 0%,
      color-mix(in srgb, var(--bg) 30%, transparent) 60%,
      color-mix(in srgb, var(--bg) 70%, transparent) 100%
    );
  }

  /* ── Cover ── */
  .cover-wrap {
    flex-shrink: 0;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.45);
    border-radius: 8px;
    overflow: hidden;
  }
  .cover {
    width: 200px;
    height: 200px;
    object-fit: cover;
    display: block;
  }
  .cover-fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 5rem;
    color: var(--text-muted);
    background: var(--bg-hover);
    user-select: none;
  }

  /* ── Hero info ── */
  .hero-info {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
  }
  .type-badge {
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--text-muted);
    margin: 0;
  }
  .hero-title {
    font-size: clamp(1.5rem, 3vw, 2.4rem);
    font-weight: 800;
    margin: 0;
    line-height: 1.1;
    color: var(--text);
    cursor: default;
  }
  .hero-title.editable { cursor: pointer; }
  .hero-title.editable:hover .edit-hint { opacity: 1; }
  .hero-artist {
    color: var(--text-muted);
    font-size: 0.9rem;
    font-weight: 600;
    text-decoration: none;
    margin: 0;
  }
  a.hero-artist:hover {
    text-decoration: underline;
    color: var(--text);
  }
  .hero-meta-row {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }
  .meta-val {
    color: var(--text-muted);
    font-size: 0.8rem;
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    cursor: default;
  }
  button.meta-val {
    cursor: pointer;
  }
  button.meta-val:hover .edit-hint { opacity: 1; }
  .meta-sep {
    color: var(--text-muted);
    font-size: 0.8rem;
    opacity: 0.5;
  }
  .save-error {
    font-size: 0.8rem;
    color: #ef4444;
    margin: 0;
  }

  /* ── Inline edit ── */
  .edit-hint {
    opacity: 0;
    font-size: 0.75em;
    color: var(--accent);
    margin-left: 6px;
    transition: opacity 0.15s;
    pointer-events: none;
  }
  .editable-trigger {
    background: none;
    border: none;
    padding: 0;
    color: inherit;
    font: inherit;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
  }
  .editable-trigger:hover .edit-hint { opacity: 1; }
  .inline-input {
    background: color-mix(in srgb, var(--accent) 8%, var(--bg));
    border: 1.5px solid var(--accent);
    border-radius: 4px;
    color: inherit;
    font: inherit;
    padding: 2px 6px;
    outline: none;
    box-sizing: border-box;
  }
  .inline-input:disabled { opacity: 0.6; }
  .title-input {
    font-size: clamp(1.5rem, 3vw, 2.4rem);
    font-weight: 800;
    width: 100%;
  }
  .year-input  { width: 80px; }
  .label-input { width: 160px; }

  /* ── Hero actions ── */
  .hero-actions {
    display: flex;
    gap: 8px;
    margin-top: 4px;
    align-items: center;
    flex-wrap: wrap;
  }
  .btn-play {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    background: var(--accent);
    border: none;
    border-radius: 20px;
    padding: 8px 20px;
    color: #fff;
    font-size: 0.875rem;
    font-weight: 700;
    cursor: pointer;
    transition: background 0.15s;
  }
  .btn-play:hover:not(:disabled) { background: var(--accent-hover, color-mix(in srgb, var(--accent) 80%, #000)); }
  .btn-play:disabled { opacity: 0.6; cursor: not-allowed; }

  .btn-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    background: color-mix(in srgb, var(--bg-elevated) 80%, transparent);
    border: 1px solid var(--border);
    color: var(--text-muted);
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s, background 0.15s;
  }
  .btn-icon:hover:not(:disabled) {
    color: var(--text);
    border-color: var(--text-muted);
    background: var(--bg-elevated);
  }
  .btn-icon:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-icon--accent {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 40%, transparent);
  }
  .btn-icon--accent:hover:not(:disabled) {
    color: var(--accent);
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, var(--bg-elevated));
  }
  .btn-icon--done {
    color: #22c55e;
    border-color: color-mix(in srgb, #22c55e 40%, transparent);
  }

  .btn-admin {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 20px;
    color: var(--text-muted);
    font-size: 0.78rem;
    padding: 5px 12px;
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s;
  }
  .btn-admin:hover:not(:disabled) { color: var(--text); border-color: var(--text-muted); }
  .btn-admin:disabled { opacity: 0.5; cursor: not-allowed; }

  @keyframes spin-anim { to { transform: rotate(360deg); } }
  .spin-icon {
    display: inline-block;
    vertical-align: middle;
    animation: spin-anim 0.8s linear infinite;
  }

  .action-msg {
    font-size: 0.8rem;
    color: var(--accent);
    margin: 2px 0 0;
  }
  .action-msg.error-msg { color: #ef4444; }

  /* ── Genre pills ── */
  .genre-pills {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 2px;
  }
  .genre-pill {
    display: inline-block;
    padding: 3px 10px;
    border-radius: 20px;
    border: 1px solid var(--border);
    color: var(--text-muted);
    font-size: 0.7rem;
    font-weight: 500;
    text-decoration: none;
    transition: color 0.15s, border-color 0.15s;
  }
  .genre-pill:hover { color: var(--text); border-color: var(--accent); }

  /* ── Variant picker ── */
  .variant-picker {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 20px;
  }
  .variant-label {
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
    margin-right: 4px;
  }
  .variant-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 5px 12px;
    border-radius: 20px;
    border: 1px solid var(--border);
    font-size: 0.8rem;
    text-decoration: none;
    color: var(--text-muted);
    transition: color 0.15s, border-color 0.15s, background 0.15s;
  }
  .variant-pill:hover { color: var(--text); border-color: var(--accent); }
  .variant-pill.active {
    color: var(--accent);
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 12%, transparent);
  }
  .variant-count {
    font-size: 0.7rem;
    color: var(--text-muted);
    opacity: 0.7;
  }
  .variant-pill.active .variant-count { opacity: 1; }

  /* ── Similar Albums ── */
  .similar-section { margin-top: 40px; }
  .section-label {
    font-size: 1rem;
    font-weight: 700;
    margin: 0 0 16px;
    color: var(--text);
  }
  .carousel {
    display: flex;
    gap: 14px;
    overflow-x: auto;
    padding-bottom: 8px;
    scrollbar-width: thin;
  }
  .carousel-card {
    flex-shrink: 0;
    width: 120px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    cursor: pointer;
    background: none;
    border: none;
    padding: 0;
    text-align: left;
  }
  .carousel-card:hover .carousel-cover { transform: scale(1.03); }
  .carousel-cover-wrap {
    width: 120px;
    height: 120px;
    border-radius: 6px;
    overflow: hidden;
    background: var(--bg-elevated);
  }
  .carousel-cover {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.15s;
  }
  .carousel-placeholder {
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 2.5rem;
    color: var(--text-muted);
  }
  .carousel-name {
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .carousel-artist {
    font-size: 0.72rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* ── Responsive ── */
  @media (max-width: 640px) {
    .hero-body {
      flex-direction: column;
      align-items: center;
      text-align: center;
      padding: 20px 16px 20px;
    }
    .cover { width: min(180px, 55vw); height: min(180px, 55vw); }
    .hero-info { align-items: center; }
    .hero-meta-row { justify-content: center; }
    .genre-pills { justify-content: center; }
    .hero-actions { justify-content: center; }
    .hero-skeleton { flex-direction: column; align-items: center; }
    .sk-cover { width: min(180px, 55vw); height: min(180px, 55vw); }
    .sk-info { align-items: center; }
  }
</style>
