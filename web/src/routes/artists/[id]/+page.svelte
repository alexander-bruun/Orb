<script lang="ts">
  import { page } from "$app/stores";
  import { goto } from "$app/navigation";
  import { library as libApi } from "$lib/api/library";
  import { recommend } from "$lib/api/recommend";
  import AlbumGrid from "$lib/components/library/AlbumGrid.svelte";
  import type {
    Artist,
    Album,
    Genre,
    RelatedArtist,
    ArtistEvent,
  } from "$lib/types";
  import { playTrack, shuffle, startRadio } from "$lib/stores/player";

  import { getApiBase } from "$lib/api/base";
  interface SimilarArtist {
    id: string;
    name: string;
    hasImage: boolean;
  }

  let artist: Artist | null = null;
  let albums: Album[] = [];
  let genres: Genre[] = [];
  let relatedArtists: RelatedArtist[] = [];
  let appearsOn: Album[] = [];
  let appearsOnGrouped: Map<string, Album[]> = new Map();
  let appearsOnKeys: string[] = [];
  let similarArtists: SimilarArtist[] = [];
  let events: ArtistEvent[] = [];
  let eventsVisible = 5;

  // Discography timeline
  let timelineYears: string[] = [];
  let albumsByYear: Map<string, Album[]> = new Map();

  // Bio
  let bio = "";
  let bioUrl = "";
  let bioExpanded = false;

  let loading = true;
  let shuffling = false;
  let radioLoading = false;
  let isRestoring = false;

  export const snapshot = {
    capture: () => ({
      artist,
      albums,
      genres,
      relatedArtists,
      appearsOn,
      appearsOnGrouped,
      appearsOnKeys,
      timelineYears,
      albumsByYear,
      bio,
      bioUrl,
      similarArtists,
      events,
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
      events = value.events ?? [];
      isRestoring = true;
      loading = false;
    },
  };

  async function loadArtist(id: string) {
    if (isRestoring && artist?.id === id) {
      loading = false;
      isRestoring = false;
      return;
    }
    loading = true;
    artist = null;
    albums = [];
    genres = [];
    relatedArtists = [];
    appearsOn = [];
    appearsOnGrouped = new Map();
    appearsOnKeys = [];
    timelineYears = [];
    albumsByYear = new Map();
    bio = "";
    bioUrl = "";
    similarArtists = [];
    events = [];
    eventsVisible = 5;

    try {
      const res = await libApi.artist(id);
      artist = res.artist;
      albums = res.albums;
      genres = res.genres ?? [];
      relatedArtists = res.related_artists ?? [];
      appearsOn = res.appears_on ?? [];

      // Build discography timeline (newest first)
      const sorted = [...albums].sort(
        (a, b) => (b.release_year ?? 0) - (a.release_year ?? 0),
      );
      albumsByYear = new Map();
      for (const album of sorted) {
        const key = album.release_year ? String(album.release_year) : "Unknown";
        if (!albumsByYear.has(key)) albumsByYear.set(key, []);
        albumsByYear.get(key)!.push(album);
      }
      timelineYears = Array.from(albumsByYear.keys());

      // Group appears-on albums by first letter
      appearsOnGrouped = new Map();
      for (const album of appearsOn) {
        const key = album.title?.[0]?.toUpperCase() ?? "#";
        if (!appearsOnGrouped.has(key)) appearsOnGrouped.set(key, []);
        appearsOnGrouped.get(key)?.push(album);
      }
      appearsOnKeys = Array.from(appearsOnGrouped.keys()).sort();
    } finally {
      loading = false;
      isRestoring = false;
    }

    // Fetch concert events in the background (non-blocking)
    libApi
      .artistEvents(id)
      .then((ev) => {
        events = ev ?? [];
      })
      .catch(() => {});

    // Fetch bio in the background (non-blocking)
    libApi
      .artistBio(id)
      .then((bioRes) => {
        bio = bioRes.bio ?? "";
        bioUrl = bioRes.bio_url ?? "";
      })
      .catch(() => {});

    // Fetch similar artists via recommendation engine (non-blocking)
    recommend
      .radioByArtist(id, 60)
      .then((simTracks) => {
        const seen = new Set<string>();
        const result: SimilarArtist[] = [];
        for (const t of simTracks) {
          const aid = t.artist_id;
          if (!aid || aid === id || seen.has(aid)) continue;
          seen.add(aid);
          result.push({
            id: aid,
            name: t.artist_name ?? "Unknown Artist",
            hasImage: false,
          });
        }
        similarArtists = result.slice(0, 12);
      })
      .catch(() => {});
  }

  $: if ($page.params.id) loadArtist($page.params.id);

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
    if (!artist.begin_date) return "";
    let s = artist.begin_date.substring(0, 4);
    if (artist.end_date) {
      s += " – " + artist.end_date.substring(0, 4);
    } else {
      s += " – present";
    }
    return s;
  }
</script>

{#if loading}
  <div class="hero-skeleton">
    <div class="sk-photo"></div>
    <div class="sk-info">
      <div class="sk-line sk-name"></div>
      <div class="sk-line sk-meta"></div>
      <div class="sk-line sk-tags"></div>
    </div>
  </div>
{:else if artist}
  <div class="hero">
    {#if artist.image_key}
      <div class="hero-bg" aria-hidden="true">
        <img src="{getApiBase()}/covers/artist/{artist.id}" alt="" class="hero-bg-img" />
      </div>
    {/if}
    <div class="hero-body">
      <div class="photo-wrap">
        {#if artist.image_key}
          <img src="{getApiBase()}/covers/artist/{artist.id}" alt={artist.name} class="artist-photo" />
        {:else}
          <div class="artist-photo photo-fallback">{artist.name[0]?.toUpperCase() ?? "?"}</div>
        {/if}
      </div>
      <div class="hero-info">
        <h1 class="hero-title">{artist.name}</h1>
        {#if artist.artist_type || artist.country || artist.begin_date}
          <div class="hero-meta-row">
            {#if artist.artist_type}<span class="meta-chip">{artist.artist_type}</span>{/if}
            {#if artist.country}<span class="meta-chip">{artist.country}</span>{/if}
            {#if artist.begin_date}<span class="meta-chip">{formatDates(artist)}</span>{/if}
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
        <div class="hero-actions">
          <button class="btn-play" on:click={shuffleAll} disabled={albums.length === 0 || shuffling}>
            {#if shuffling}
              <svg class="spin-icon" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><circle cx="12" cy="12" r="9" stroke-dasharray="44 13" /></svg> Loading…
            {:else}
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 3 21 3 21 8" /><line x1="4" y1="20" x2="21" y2="3" /><polyline points="21 16 21 21 16 21" /><line x1="15" y1="15" x2="21" y2="21" /><line x1="4" y1="4" x2="9" y2="9" /></svg> Shuffle
            {/if}
          </button>
          <button class="btn-icon btn-icon--accent" on:click={startArtistRadio} disabled={radioLoading} title="Start Radio">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="2" /><path d="M16.24 7.76a6 6 0 0 1 0 8.49m-8.48-.01a6 6 0 0 1 0-8.49m11.31-2.82a10 10 0 0 1 0 14.14m-14.14 0a10 10 0 0 1 0-14.14" /></svg>
          </button>
        </div>
      </div>
    </div>
  </div>
  {#if bio}
    <div class="bio-section">
      <p class="bio-text" class:bio-collapsed={!bioExpanded}>{bio}</p>
      <div class="bio-footer">
        <button
          class="bio-toggle"
          on:click={() => (bioExpanded = !bioExpanded)}
        >
          {bioExpanded ? "Show less" : "Read more"}
        </button>
        {#if bioUrl}
          <a
            href={bioUrl}
            target="_blank"
            rel="noopener noreferrer"
            class="bio-source">Wikipedia ↗</a
          >
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
          on:keydown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              goto(`/artists/${sa.id}`);
            }
          }}
        >
          <div class="sim-photo-wrap">
            <img
              src="{getApiBase()}/covers/artist/{sa.id}"
              alt={sa.name}
              class="sim-photo"
              on:error={(e) => {
                (e.currentTarget as HTMLImageElement).style.display = "none";
                (
                  e.currentTarget.nextElementSibling as HTMLElement
                ).style.display = "flex";
              }}
            />
            <div class="sim-photo-fallback" style="display:none">
              {sa.name[0]?.toUpperCase() ?? "?"}
            </div>
          </div>
          <span class="sim-name" title={sa.name}>{sa.name}</span>
        </div>
      {/each}
    </div>
  {/if}

  {#if events.length > 0}
    <h2 class="section" style="margin-top: 32px;">Upcoming Shows</h2>
    <div class="events-list">
      {#each events.slice(0, eventsVisible) as event (event.id)}
        {@const date = new Date(event.datetime)}
        {@const ticketOffer = event.offers?.find(
          (o) => o.type === "Tickets" && o.status === "available",
        )}
        <div class="event-row">
          <div class="event-date">
            <span class="event-month"
              >{date
                .toLocaleString("default", { month: "short" })
                .toUpperCase()}</span
            >
            <span class="event-day">{date.getDate()}</span>
            <span class="event-year">{date.getFullYear()}</span>
          </div>
          <div class="event-info">
            <span class="event-venue">{event.venue.name}</span>
            <span class="event-location">
              {[event.venue.city, event.venue.region, event.venue.country]
                .filter(Boolean)
                .join(", ")}
            </span>
            {#if event.lineup.length > 1}
              <span class="event-lineup"
                >with {event.lineup
                  .filter((n) => n !== artist?.name)
                  .join(", ")}</span
              >
            {/if}
          </div>
          <div class="event-actions">
            {#if ticketOffer}
              <a
                href={ticketOffer.url}
                target="_blank"
                rel="noopener noreferrer"
                class="btn-tickets">Tickets</a
              >
            {/if}
            <a
              href={event.url}
              target="_blank"
              rel="noopener noreferrer"
              class="btn-event-more">Details ↗</a
            >
          </div>
        </div>
      {/each}
    </div>
    {#if eventsVisible < events.length}
      <button class="btn-show-more" on:click={() => (eventsVisible += 3)}>
        Show more ({events.length - eventsVisible} remaining)
      </button>
    {/if}
  {/if}
{/if}

<svelte:head>
  <title>{artist ? `${artist.name} – Orb` : "Artist – Orb"}</title>
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
  .sk-photo {
    width: 160px;
    height: 160px;
    border-radius: 50%;
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
  .sk-name { height: 32px; width: min(280px, 70%); }
  .sk-meta { height: 12px; width: min(200px, 55%); }
  .sk-tags { height: 22px; width: min(240px, 60%); border-radius: 20px; }
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
    filter: blur(40px) saturate(1.3) brightness(0.5);
    transform: scale(1.1);
  }
  .hero-body {
    position: relative;
    z-index: 1;
    display: flex;
    gap: 28px;
    align-items: flex-end;
    padding: 40px 28px 28px;
    background: linear-gradient(
      to bottom,
      transparent 0%,
      color-mix(in srgb, var(--bg) 30%, transparent) 60%,
      color-mix(in srgb, var(--bg) 70%, transparent) 100%
    );
  }

  /* ── Photo ── */
  .photo-wrap { flex-shrink: 0; }
  .artist-photo {
    width: 160px;
    height: 160px;
    border-radius: 50%;
    object-fit: cover;
    display: block;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.45);
  }
  .photo-fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 4rem;
    font-weight: 700;
    color: var(--text-muted);
    background: var(--bg-hover);
    user-select: none;
  }

  /* ── Hero info ── */
  .hero-info {
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-width: 0;
  }
  .hero-title {
    font-size: clamp(1.75rem, 4vw, 2.8rem);
    font-weight: 800;
    margin: 0;
    line-height: 1.1;
    color: var(--text);
  }
  .hero-meta-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .meta-chip {
    font-size: 0.75rem;
    color: var(--text-muted);
    background: color-mix(in srgb, var(--bg-elevated) 80%, transparent);
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 3px 10px;
  }
  .disambiguation {
    font-size: 0.85rem;
    color: var(--text-muted);
    font-style: italic;
    margin: 0;
  }
  .genre-pills {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
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

  /* ── Hero actions ── */
  .hero-actions {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
    margin-top: 4px;
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
  .btn-icon:hover:not(:disabled) { color: var(--text); border-color: var(--text-muted); background: var(--bg-elevated); }
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
  @keyframes spin-anim { to { transform: rotate(360deg); } }
  .spin-icon { display: inline-block; vertical-align: middle; animation: spin-anim 0.8s linear infinite; }

  .section {
    font-size: 1rem;
    font-weight: 600;
    color: var(--text-muted);
    margin-bottom: 16px;
  }
  .related-list {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .related-artist {
    display: flex;
    flex-direction: column;
    padding: 10px 16px;
    border-radius: 8px;
    border: 1px solid var(--border);
    text-decoration: none;
    transition: border-color 0.15s;
  }
  .related-artist:hover {
    border-color: var(--accent);
  }
  .related-name {
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--text);
  }
  .related-type {
    font-size: 0.7rem;
    color: var(--text-muted);
  }

  /* ── Bio ─────────────────────────────────────────────────── */
  .bio-section {
    margin-bottom: 32px;
  }
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
  .bio-footer {
    display: flex;
    align-items: center;
    gap: 16px;
  }
  .bio-toggle {
    background: none;
    border: none;
    padding: 0;
    color: var(--accent);
    font-size: 0.8rem;
    cursor: pointer;
  }
  .bio-toggle:hover {
    text-decoration: underline;
  }
  .bio-source {
    font-size: 0.8rem;
    color: var(--text-muted);
    text-decoration: none;
  }
  .bio-source:hover {
    color: var(--text);
  }

  /* ── Similar Artists ────────────────────────────────────── */
  .similar-carousel {
    display: flex;
    gap: 16px;
    overflow-x: auto;
    padding-bottom: 8px;
    scrollbar-width: thin;
  }
  .sim-card {
    flex-shrink: 0;
    width: 100px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    cursor: pointer;
  }
  .sim-card:hover .sim-photo {
    transform: scale(1.05);
  }
  .sim-photo-wrap {
    position: relative;
    width: 90px;
    height: 90px;
    border-radius: 50%;
    overflow: hidden;
    background: var(--bg-elevated);
    flex-shrink: 0;
  }
  .sim-photo {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.15s;
  }
  .sim-photo-fallback {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 2rem;
    font-weight: 700;
    color: var(--text-muted);
    background: var(--bg-hover);
  }
  .sim-name {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text);
    text-align: center;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    width: 100%;
  }

  /* ── Concert Events ─────────────────────────────────────── */
  .events-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-bottom: 8px;
  }
  .event-row {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 12px 16px;
    border: 1px solid var(--border);
    border-radius: 8px;
    transition: border-color 0.15s;
  }
  .event-row:hover {
    border-color: var(--accent);
  }
  .event-date {
    display: flex;
    flex-direction: column;
    align-items: center;
    min-width: 44px;
    flex-shrink: 0;
  }
  .event-month {
    font-size: 0.65rem;
    font-weight: 700;
    color: var(--accent);
    letter-spacing: 0.05em;
  }
  .event-day {
    font-size: 1.4rem;
    font-weight: 700;
    color: var(--text);
    line-height: 1;
  }
  .event-year {
    font-size: 0.65rem;
    color: var(--text-muted);
  }
  .event-info {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .event-venue {
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .event-location {
    font-size: 0.75rem;
    color: var(--text-muted);
  }
  .event-lineup {
    font-size: 0.75rem;
    color: var(--text-muted);
    font-style: italic;
  }
  .event-actions {
    display: flex;
    gap: 8px;
    flex-shrink: 0;
    align-items: center;
  }
  .btn-tickets {
    display: inline-block;
    padding: 6px 14px;
    border-radius: 20px;
    background: var(--accent);
    color: #fff;
    font-size: 0.75rem;
    font-weight: 600;
    text-decoration: none;
    transition: opacity 0.15s;
  }
  .btn-tickets:hover {
    opacity: 0.85;
  }
  .btn-event-more {
    font-size: 0.75rem;
    color: var(--text-muted);
    text-decoration: none;
  }
  .btn-event-more:hover {
    color: var(--text);
  }
  .btn-show-more {
    margin-top: 8px;
    background: none;
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 6px 16px;
    font-size: 0.8rem;
    color: var(--text-muted);
    cursor: pointer;
  }
  .btn-show-more:hover {
    color: var(--text);
    border-color: var(--text);
  }

  /* ── Mobile ─────────────────────────────────────────────── */
  @media (max-width: 640px) {
    .hero-body {
      flex-direction: column;
      align-items: center;
      text-align: center;
      padding: 24px 16px 20px;
    }
    .artist-photo {
      width: min(140px, 50vw);
      height: min(140px, 50vw);
    }
    .hero-info { align-items: center; }
    .hero-meta-row { justify-content: center; }
    .genre-pills { justify-content: center; }
    .hero-actions { justify-content: center; }
    .hero-skeleton { flex-direction: column; align-items: center; }
    .sk-photo { width: min(140px, 50vw); height: min(140px, 50vw); }
    .sk-info { align-items: center; }
    .related-list { justify-content: center; }
  }
</style>
