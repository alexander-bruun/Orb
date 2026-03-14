<script lang="ts">
  import { page } from '$app/stores';
  import { library as libApi } from '$lib/api/library';
  import TrackList from '$lib/components/library/TrackList.svelte';
  import type { Album, Track, Genre } from '$lib/types';
  import { playTrack, shuffle, startRadio } from '$lib/stores/player';
  import { downloadAlbum, downloads } from '$lib/stores/offline/downloads';

  import { getApiBase } from '$lib/api/base';

  let album: Album | null = null;
  let tracks: Track[] = [];
  let genres: Genre[] = [];
  let variants: Album[] = [];
  let artistName: string | null = null;
  let artistId: string | null = null;
  let loading = true;

  async function loadAlbum(id: string) {
    loading = true;
    album = null;
    tracks = [];
    genres = [];
    variants = [];
    artistName = null;
    artistId = null;
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
    } finally {
      loading = false;
    }
  }

  $: if ($page.params.id) loadAlbum($page.params.id);

  function playAll() {
    if (tracks.length > 0) playTrack(tracks[0], tracks);
  }

  function shuffleAll() {
    if (tracks.length === 0) return;
    shuffle.set(true);
    const idx = Math.floor(Math.random() * tracks.length);
    playTrack(tracks[idx], tracks);
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
  $: dlDoneCount = tracks.filter(t => $downloads.get(t.id)?.status === 'done').length;
  $: allDownloaded = tracks.length > 0 && dlDoneCount === tracks.length;
  $: dlActiveCount = tracks.filter(t => $downloads.get(t.id)?.status === 'downloading').length;

  async function downloadAll() {
    if (downloading || tracks.length === 0) return;
    downloading = true;
    try {
      await downloadAlbum(tracks);
    } finally {
      downloading = false;
    }
  }
</script>

{#if loading}
  <p class="muted">Loading…</p>
{:else if album}
  <div class="header">
    {#if album.cover_art_key}
      <img src="{getApiBase()}/covers/{album.id}" alt={album.title} class="cover" />
    {:else}
      <div class="cover album-fallback">♪</div>
    {/if}
    <div class="meta">
      <p class="type">{album.album_type ?? 'Album'}</p>
      <h1 class="title">{album.title}</h1>
      {#if artistName}
        {#if artistId}
          <a href="/artists/{artistId}" class="artist">{artistName}</a>
        {:else}
          <p class="artist">{artistName}</p>
        {/if}
      {/if}
      <div class="meta-row">
        {#if album.release_year}
          <span class="year">{album.release_year}</span>
        {/if}
        {#if discCount > 1}
          <span class="disc-count">{discCount} discs</span>
        {/if}
        {#if album.label}
          <span class="label-info">{album.label}</span>
        {/if}
      </div>
      {#if genres.length > 0}
        <div class="genre-pills">
          {#each genres as genre}
            <a href="/genres/{genre.id}" class="genre-pill">{genre.name}</a>
          {/each}
        </div>
      {/if}
      <div class="actions">
        <button class="btn-play" on:click={playAll} disabled={tracks.length === 0}>▶ Play</button>
        <button class="btn-shuffle" on:click={shuffleAll} disabled={tracks.length === 0} title="Shuffle">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/>
            <polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/>
            <line x1="4" y1="4" x2="9" y2="9"/>
          </svg>
          Shuffle
        </button>
        <button class="btn-radio" on:click={startAlbumRadio} disabled={tracks.length === 0 || radioLoading} title="Start radio based on this album">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="2"/><path d="M16.24 7.76a6 6 0 0 1 0 8.49m-8.48-.01a6 6 0 0 1 0-8.49m11.31-2.82a10 10 0 0 1 0 14.14m-14.14 0a10 10 0 0 1 0-14.14"/>
          </svg>
          {radioLoading ? 'Loading…' : 'Start Radio'}
        </button>
        <button class="btn-download" on:click={downloadAll} disabled={tracks.length === 0 || allDownloaded || downloading} title="Download all tracks for offline playback">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>
          </svg>
          {#if allDownloaded}Downloaded{:else if downloading || dlActiveCount > 0}{dlDoneCount}/{tracks.length}{:else}Download{/if}
        </button>
      </div>
    </div>
  </div>
  {#if variants.length > 1}
    <div class="variant-picker">
      <span class="variant-label">Versions</span>
      {#each variants as v}
        <a
          href="/library/albums/{v.id}"
          class="variant-pill"
          class:active={v.id === album.id}
        >
          <span class="variant-edition">{v.edition ?? 'Standard'}</span>
          <span class="variant-count">{v.track_count ?? 0} tracks</span>
        </a>
      {/each}
    </div>
  {/if}
  <TrackList {tracks} />
{/if}

<svelte:head>
  <title>{album ? `${album.title} – Orb` : 'Album – Orb'}</title>
</svelte:head>

<style>
  .header { display: flex; gap: 24px; align-items: flex-end; margin-bottom: 32px; }
  .cover {
    width: 180px;
    height: 180px;
    object-fit: cover;
    border-radius: 8px;
    flex-shrink: 0;
  }
  .album-fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 4.5rem;
    color: var(--text-muted);
    background: var(--bg-hover);
    user-select: none;
  }
  .meta { display: flex; flex-direction: column; gap: 4px; }
  .type { font-size: 0.75rem; text-transform: uppercase; color: var(--text-muted); }
  .title { font-size: 2rem; font-weight: 700; margin: 0; }
  .artist { color: var(--text-muted); font-size: 0.9rem; font-weight: 600; text-decoration: none; }
  a.artist:hover { text-decoration: underline; color: var(--text); }
  .meta-row { display: flex; gap: 10px; align-items: center; }
  .year { color: var(--text-muted); font-size: 0.875rem; }
  .disc-count { color: var(--text-muted); font-size: 0.875rem; }
  .disc-count::before { content: '·'; margin-right: 10px; }
  .label-info { color: var(--text-muted); font-size: 0.875rem; }
  .label-info::before { content: '·'; margin-right: 10px; }
  .genre-pills { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; }
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
  .actions { display: flex; gap: 8px; margin-top: 8px; align-items: center; }
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
  .btn-play:hover { background: var(--accent-hover); }
  .btn-play:disabled { opacity: 0.6; cursor: not-allowed; }
  .btn-shuffle {
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
  .btn-shuffle:hover { color: var(--text); border-color: var(--text); }
  .btn-shuffle:disabled { opacity: 0.6; cursor: not-allowed; }
  .btn-radio {
    display: flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid color-mix(in srgb, var(--accent) 40%, transparent);
    border-radius: 20px;
    padding: 7px 16px;
    color: var(--accent);
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-radio:hover { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, transparent); }
  .btn-radio:disabled { opacity: 0.6; cursor: not-allowed; }
  .btn-download {
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
  .btn-download:hover { color: var(--text); border-color: var(--text); }
  .btn-download:disabled { opacity: 0.6; cursor: not-allowed; }
  .muted { color: var(--text-muted); }

  /* Variant / edition picker */
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
    background: rgba(var(--accent-rgb, 99 102 241) / 0.12);
  }
  .variant-count {
    font-size: 0.7rem;
    color: var(--text-muted);
    opacity: 0.7;
  }
  .variant-pill.active .variant-count { opacity: 1; }

  /* ── Mobile ─────────────────────────────────────────────── */
  @media (max-width: 640px) {
    .header {
      flex-direction: column;
      align-items: center;
      text-align: center;
      gap: 16px;
      margin-top: var(--page-padding);
      margin-bottom: 20px;
    }
    .cover {
      width: min(200px, 60vw);
      height: min(200px, 60vw);
    }
    .meta {
      width: 100%;
      align-items: center;
    }
    .title { font-size: 1.5rem; }
    .actions { justify-content: center; flex-wrap: wrap; }
    .meta-row { justify-content: center; flex-wrap: wrap; }
    .genre-pills { justify-content: center; }
  }
</style>
