<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { library as libApi } from '$lib/api/library';
  import { recommend } from '$lib/api/recommend';
  import AlbumGrid from '$lib/components/library/AlbumGrid.svelte';
  import type { Artist, Album, Genre, RelatedArtist } from '$lib/types';
  import { playTrack, shuffle, startRadio } from '$lib/stores/player';

  import { getApiBase } from '$lib/api/base';

  interface SimilarArtist { id: string; name: string; hasImage: boolean; }

  let artist: Artist | null = null;
  let albums: Album[] = [];
  let genres: Genre[] = [];
  let relatedArtists: RelatedArtist[] = [];
  let appearsOn: Album[] = [];
  let appearsOnGrouped: Map<string, Album[]> = new Map();
  let appearsOnKeys: string[] = [];
  let similarArtists: SimilarArtist[] = [];

  // Discography timeline
  let timelineYears: string[] = [];
  let albumsByYear: Map<string, Album[]> = new Map();

  // Bio
  let bio = '';
  let bioUrl = '';
  let bioExpanded = false;

  let loading = true;
  let shuffling = false;
  let radioLoading = false;
  let isRestoring = false;

  export const snapshot = {
    capture: () => ({
      artist, albums, genres, relatedArtists, appearsOn, appearsOnGrouped, appearsOnKeys,
      timelineYears, albumsByYear, bio, bioUrl, similarArtists
    }),
    restore: (value) => {
      artist = value.artist;
      albums = value.albums;
      genres = value.genres;
      relatedArtists = value.relatedArtists;
      appearsOn = value.appearsOn;
      appearsOnGrouped = value.appearsOnGrouped;
      appearsOnKeys = value.appearsOnKeys;
      timelineYears = value.timelineYears;
      albumsByYear = value.albumsByYear;
      bio = value.bio;
      bioUrl = value.bioUrl;
      similarArtists = value.similarArtists ?? [];
      isRestoring = true;
      loading = false;
    }
  };

  onMount(async () => {
    const id = $page.params.id ?? '';

    if (isRestoring && artist?.id === id) {
      isRestoring = false;
      return;
    }

    try {
      const res = await libApi.artist(id);
      artist = res.artist;
      albums = res.albums;
      genres = res.genres ?? [];
      relatedArtists = res.related_artists ?? [];
      appearsOn = res.appears_on ?? [];

      // Build discography timeline (newest first)
      const sorted = [...albums].sort((a, b) => (b.release_year ?? 0) - (a.release_year ?? 0));
      albumsByYear = new Map();
      for (const album of sorted) {
        const key = album.release_year ? String(album.release_year) : 'Unknown';
        if (!albumsByYear.has(key)) albumsByYear.set(key, []);
        albumsByYear.get(key)!.push(album);
      }
      timelineYears = Array.from(albumsByYear.keys());

      // Group appears-on albums by first letter
      appearsOnGrouped = new Map();
      for (const album of appearsOn) {
        const key = album.title?.[0]?.toUpperCase() ?? '#';
        if (!appearsOnGrouped.has(key)) appearsOnGrouped.set(key, []);
        appearsOnGrouped.get(key)?.push(album);
      }
      appearsOnKeys = Array.from(appearsOnGrouped.keys()).sort();
    } finally {
      loading = false;
    }

    // Fetch bio in the background (non-blocking)
    try {
      const bioRes = await libApi.artistBio(id);
      bio = bioRes.bio ?? '';
      bioUrl = bioRes.bio_url ?? '';
    } catch {
      // bio stays empty
    }

    // Fetch similar artists via recommendation engine (non-blocking)
    try {
      const simTracks = await recommend.radioByArtist(id, 60);
      const seen = new Set<string>();
      const result: SimilarArtist[] = [];
      for (const t of simTracks) {
        if (!t.artist_id || t.artist_id === id || seen.has(t.artist_id)) continue;
        seen.add(t.artist_id);
        result.push({ id: t.artist_id, name: t.artist_name ?? 'Unknown Artist', hasImage: false });
      }
      similarArtists = result.slice(0, 12);
    } catch {
      // similarArtists stays empty
    }
  });

  async function shuffleAll() {
    if (albums.length === 0 || shuffling) return;
    shuffling = true;
    try {
      const results = await Promise.all(albums.map((a) => libApi.album(a.id)));
      const tracks = results.flatMap((r) => r.tracks);
      if (tracks.length === 0) return;
      shuffle.set(true);
      const idx = Math.floor(Math.random() * tracks.length);
      await playTrack(tracks[idx], tracks);
    } finally {
      shuffling = false;
    }
  }

  async function startArtistRadio() {
    if (radioLoading) return;
    radioLoading = true;
    try {
      await startRadio();
    } finally {
      radioLoading = false;
    }
  }

  function formatDates(artist: Artist): string {
    if (!artist.begin_date) return '';
    let s = artist.begin_date.substring(0, 4);
    if (artist.end_date) {
      s += ' – ' + artist.end_date.substring(0, 4);
    } else {
      s += ' – present';
    }
    return s;
  }
</script>

{#if loading}
  <p class="muted">Loading…</p>
{:else if artist}
  <div class="header">
    {#if artist.image_key}
      <img src="{getApiBase()}/covers/artist/{artist.id}" alt={artist.name} class="artist-photo" />
    {/if}
    <div class="header-text">
      <h1 class="title">{artist.name}</h1>
      {#if artist.artist_type || artist.country || artist.begin_date}
        <div class="artist-meta">
          {#if artist.artist_type}
            <span class="meta-item">{artist.artist_type}</span>
          {/if}
          {#if artist.country}
            <span class="meta-item">{artist.country}</span>
          {/if}
          {#if artist.begin_date}
            <span class="meta-item">{formatDates(artist)}</span>
          {/if}
        </div>
      {/if}
      {#if artist.disambiguation}
        <p class="disambiguation">{artist.disambiguation}</p>
      {/if}
      {#if genres.length > 0}
        <div class="genre-pills">
          {#each genres as genre}
            <a href="/genres/{genre.id}" class="genre-pill">{genre.name}</a>
          {/each}
        </div>
      {/if}
    </div>
    <div class="header-actions">
      <button class="btn-shuffle" on:click={shuffleAll} disabled={albums.length === 0 || shuffling} title="Shuffle all songs">
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/>
          <polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/>
          <line x1="4" y1="4" x2="9" y2="9"/>
        </svg>
        {shuffling ? 'Loading…' : 'Shuffle All'}
      </button>
      <button class="btn-radio" on:click={startArtistRadio} disabled={radioLoading} title="Start radio based on your listening history">
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="2"/><path d="M16.24 7.76a6 6 0 0 1 0 8.49m-8.48-.01a6 6 0 0 1 0-8.49m11.31-2.82a10 10 0 0 1 0 14.14m-14.14 0a10 10 0 0 1 0-14.14"/>
        </svg>
        {radioLoading ? 'Loading…' : 'Start Radio'}
      </button>
    </div>
  </div>
  {#if bio}
    <div class="bio-section">
      <p class="bio-text" class:bio-collapsed={!bioExpanded}>{bio}</p>
      <div class="bio-footer">
        <button class="bio-toggle" on:click={() => (bioExpanded = !bioExpanded)}>
          {bioExpanded ? 'Show less' : 'Read more'}
        </button>
        {#if bioUrl}
          <a href={bioUrl} target="_blank" rel="noopener noreferrer" class="bio-source">Wikipedia ↗</a>
        {/if}
      </div>
    </div>
  {/if}

  {#if albums.length > 0}
    <h2 class="section">Discography</h2>
    <AlbumGrid grouped={albumsByYear} keys={timelineYears} />
  {/if}

  {#if appearsOn.length > 0}
    <h2 class="section" style="margin-top: 32px;">Appears On</h2>
    <AlbumGrid grouped={appearsOnGrouped} keys={appearsOnKeys} />
  {/if}

  {#if relatedArtists.length > 0}
    <h2 class="section" style="margin-top: 32px;">Related Artists</h2>
    <div class="related-list">
      {#each relatedArtists as rel}
        <a href="/artists/{rel.related_id}" class="related-artist">
          <span class="related-name">{rel.artist_name}</span>
          <span class="related-type">{rel.rel_type}</span>
        </a>
      {/each}
    </div>
  {/if}

  {#if similarArtists.length > 0}
    <h2 class="section" style="margin-top: 32px;">Similar Artists</h2>
    <div class="similar-carousel">
      {#each similarArtists as sa (sa.id)}
        <div
          class="sim-card"
          role="button"
          tabindex="0"
          aria-label="Open {sa.name}"
          on:click={() => goto(`/artists/${sa.id}`)}
          on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); goto(`/artists/${sa.id}`); } }}
        >
          <div class="sim-photo-wrap">
            <img
              src="{getApiBase()}/covers/artist/{sa.id}"
              alt={sa.name}
              class="sim-photo"
              on:error={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none'; (e.currentTarget.nextElementSibling as HTMLElement).style.display = 'flex'; }}
            />
            <div class="sim-photo-fallback" style="display:none">{sa.name[0]?.toUpperCase() ?? '?'}</div>
          </div>
          <span class="sim-name" title={sa.name}>{sa.name}</span>
        </div>
      {/each}
    </div>
  {/if}
{/if}

<svelte:head>
  <title>{artist ? `${artist.name} – Orb` : 'Artist – Orb'}</title>
</svelte:head>

<style>
  .header { display: flex; align-items: flex-start; gap: 20px; margin-bottom: 32px; }
  .artist-photo {
    width: 160px;
    height: 160px;
    border-radius: 50%;
    object-fit: cover;
    flex-shrink: 0;
    box-shadow: 0 4px 24px rgba(0,0,0,0.25);
  }
  .header-text { flex: 1; }
  .title { font-size: 2.5rem; font-weight: 700; margin: 0; }
  .artist-meta { display: flex; gap: 12px; align-items: center; margin-top: 6px; }
  .meta-item { font-size: 0.8rem; color: var(--text-muted); }
  .meta-item + .meta-item::before { content: '·'; margin-right: 12px; }
  .disambiguation { font-size: 0.85rem; color: var(--text-muted); font-style: italic; margin: 4px 0 0; }
  .genre-pills { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }
  .genre-pill {
    display: inline-block;
    padding: 4px 12px;
    border-radius: 20px;
    border: 1px solid var(--border);
    color: var(--text-muted);
    font-size: 0.75rem;
    font-weight: 500;
    text-decoration: none;
    transition: color 0.15s, border-color 0.15s;
  }
  .genre-pill:hover { color: var(--text); border-color: var(--accent); }
  .btn-shuffle {
    display: flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 8px 18px;
    color: var(--text-muted);
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    flex-shrink: 0;
    margin-top: 8px;
  }
  .btn-shuffle:hover { color: var(--text); border-color: var(--text); }
  .btn-shuffle:disabled { opacity: 0.6; cursor: not-allowed; }
  .header-actions { display: flex; flex-direction: column; gap: 8px; flex-shrink: 0; }
  .btn-radio {
    display: flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 8px 18px;
    color: var(--accent);
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    border-color: color-mix(in srgb, var(--accent) 40%, transparent);
  }
  .btn-radio:hover { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, transparent); }
  .btn-radio:disabled { opacity: 0.6; cursor: not-allowed; }
  .section { font-size: 1rem; font-weight: 600; color: var(--text-muted); margin-bottom: 16px; }
  .muted { color: var(--text-muted); }
  .related-list { display: flex; flex-wrap: wrap; gap: 8px; }
  .related-artist {
    display: flex;
    flex-direction: column;
    padding: 10px 16px;
    border-radius: 8px;
    border: 1px solid var(--border);
    text-decoration: none;
    transition: border-color 0.15s;
  }
  .related-artist:hover { border-color: var(--accent); }
  .related-name { font-size: 0.875rem; font-weight: 600; color: var(--text); }
  .related-type { font-size: 0.7rem; color: var(--text-muted); }

  /* ── Bio ─────────────────────────────────────────────────── */
  .bio-section { margin-bottom: 32px; }
  .bio-text {
    font-size: 0.875rem;
    line-height: 1.7;
    color: var(--text-muted);
    white-space: pre-wrap;
    margin: 0 0 8px;
  }
  .bio-collapsed {
    display: -webkit-box;
    -webkit-line-clamp: 4;
    line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .bio-footer { display: flex; align-items: center; gap: 16px; }
  .bio-toggle {
    background: none;
    border: none;
    padding: 0;
    color: var(--accent);
    font-size: 0.8rem;
    cursor: pointer;
  }
  .bio-toggle:hover { text-decoration: underline; }
  .bio-source {
    font-size: 0.8rem;
    color: var(--text-muted);
    text-decoration: none;
  }
  .bio-source:hover { color: var(--text); }

  /* ── Similar Artists ────────────────────────────────────── */
  .similar-carousel { display: flex; gap: 16px; overflow-x: auto; padding-bottom: 8px; scrollbar-width: thin; }
  .sim-card { flex-shrink: 0; width: 100px; display: flex; flex-direction: column; align-items: center; gap: 8px; cursor: pointer; }
  .sim-card:hover .sim-photo { transform: scale(1.05); }
  .sim-photo-wrap { position: relative; width: 90px; height: 90px; border-radius: 50%; overflow: hidden; background: var(--bg-elevated); flex-shrink: 0; }
  .sim-photo { width: 100%; height: 100%; object-fit: cover; display: block; transition: transform 0.15s; }
  .sim-photo-fallback { width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; font-size: 2rem; font-weight: 700; color: var(--text-muted); background: var(--bg-hover); }
  .sim-name { font-size: 0.75rem; font-weight: 600; color: var(--text); text-align: center; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; width: 100%; }

  /* ── Mobile ─────────────────────────────────────────────── */
  @media (max-width: 640px) {
    .header {
      flex-direction: column;
      align-items: center;
      text-align: center;
      gap: 16px;
      margin-bottom: 20px;
    }
    .artist-photo {
      width: min(140px, 50vw);
      height: min(140px, 50vw);
    }
    .header-text { width: 100%; }
    .title { font-size: 1.75rem; }
    .artist-meta { justify-content: center; flex-wrap: wrap; }
    .genre-pills { justify-content: center; }
    .header-actions { flex-direction: row; flex-wrap: wrap; justify-content: center; }
    .related-list { justify-content: center; }
  }
</style>
