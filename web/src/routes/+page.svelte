<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { library as libApi } from "$lib/api/library";
  import { smartPlaylists as spApi } from "$lib/api/smartPlaylists";
  import { audiobooks as abApi } from "$lib/api/audiobooks";
  import { podcasts as pcApi } from "$lib/api/podcasts";
  import type {
    Track,
    Album,
    SmartPlaylist,
    Audiobook,
    Podcast,
  } from "$lib/types";
  import TrackList from "$lib/components/library/TrackList.svelte";
  import AlbumCard from "$lib/components/library/AlbumCard.svelte";
  import Skeleton from "$lib/components/ui/Skeleton.svelte";
  import { playTrack, shuffle as shuffleStore } from "$lib/stores/player";
  import { playAudiobook } from "$lib/stores/player/audiobookPlayer";
  import { downloads } from "$lib/stores/offline/downloads";
  import { isOffline } from "$lib/stores/offline/connectivity";
  import { getApiBase } from "$lib/api/base";
  import Spinner from "$lib/components/ui/Spinner.svelte";

  const PAGE_SIZE = 10;

  type Interval = "today" | "week" | "month" | "all" | "custom";

  let interval: Interval = "all";
  let customFrom = "";
  let customTo = "";

  let recentTracks: Track[] = [];
  let mostTracks: Track[] = [];
  let recentAlbums: Album[] = [];
  let newAlbums: Album[] = [];
  let newAudiobooks: Audiobook[] = [];
  let newPodcasts: Podcast[] = [];
  let podcastsWithNewEpisodes: Podcast[] = [];
  let smartPls: SmartPlaylist[] = [];
  type InProgressBook = Audiobook & {
    position_ms: number;
    progress_updated_at: string;
  };
  let inProgressBooks: InProgressBook[] = [];
  let loading = true;
  let playsLoading = false;
  let isRestoring = false;

  let recentPage = 1;
  let mostPage = 1;

  export const snapshot = {
    capture: () => ({
      recentTracks,
      mostTracks,
      recentAlbums,
      newAlbums,
      newAudiobooks,
      newPodcasts,
      podcastsWithNewEpisodes,
      smartPls,
      inProgressBooks,
      interval,
      customFrom,
      customTo,
      recentPage,
      mostPage,
    }),
    restore: (value) => {
      recentTracks = value.recentTracks;
      mostTracks = value.mostTracks;
      recentAlbums = value.recentAlbums;
      newAlbums = value.newAlbums;
      newAudiobooks = value.newAudiobooks ?? [];
      newPodcasts = value.newPodcasts ?? [];
      podcastsWithNewEpisodes = value.podcastsWithNewEpisodes ?? [];
      smartPls = value.smartPls;
      inProgressBooks = value.inProgressBooks;
      interval = value.interval;
      customFrom = value.customFrom;
      customTo = value.customTo;
      recentPage = value.recentPage;
      mostPage = value.mostPage;
      isRestoring = true;
      loading = false;
    },
  };

  $: recentPages = Math.max(1, Math.ceil(recentTracks.length / PAGE_SIZE));
  $: mostPages = Math.max(1, Math.ceil(mostTracks.length / PAGE_SIZE));
  $: pagedRecent = recentTracks.slice(
    (recentPage - 1) * PAGE_SIZE,
    recentPage * PAGE_SIZE,
  );
  $: pagedMost = mostTracks.slice(
    (mostPage - 1) * PAGE_SIZE,
    mostPage * PAGE_SIZE,
  );

  // ── Offline: derive playable stubs from the downloads store ─────────────────
  $: offlineTracks = [...$downloads.values()]
    .filter((e) => e.status === "done" && !e.isAudiobook)
    .map(
      (e) =>
        ({
          id: e.trackId,
          title: e.title,
          artist_name: e.artistName,
          album_name: e.albumName,
          album_id: e.albumId,
          disc_number: 0,
          duration_ms: 0,
          file_key: "",
          file_size: e.sizeBytes,
          format: "flac" as const,
          sample_rate: 44100,
          channels: 2,
        }) satisfies Track,
    );

  $: offlineBooksList = Array.from(
    Array.from($downloads.values())
      .filter((e) => e.status === "done" && e.isAudiobook === true)
      .reduce((acc, e) => {
        if (e.albumId && !acc.has(e.albumId)) {
          acc.set(e.albumId, {
            id: e.albumId,
            title: e.albumName,
            author_name: e.artistName,
            cover_art_key: "offline",
          } as Audiobook);
        }
        return acc;
      }, new Map<string, Audiobook>())
      .values(),
  );

  function playAllOffline() {
    if (offlineTracks.length > 0) playTrack(offlineTracks[0], offlineTracks);
  }

  function shuffleOffline() {
    if (offlineTracks.length === 0) return;
    shuffleStore.set(true);
    playTrack(
      offlineTracks[Math.floor(Math.random() * offlineTracks.length)],
      offlineTracks,
    );
  }

  let dataFetched = false;

  async function loadHomeData() {
    loading = true;
    try {
      [
        recentTracks,
        mostTracks,
        recentAlbums,
        newAlbums,
        newAudiobooks,
        newPodcasts,
        podcastsWithNewEpisodes,
        smartPls,
        inProgressBooks,
      ] = await Promise.all([
        libApi.recentlyPlayed(100).then((r) => r ?? []),
        libApi.mostPlayed(100).then((r) => r ?? []),
        libApi.recentlyPlayedAlbums().then((r) => r ?? []),
        libApi.recentlyAddedAlbums(20).then((r) => r ?? []),
        abApi
          .recentlyAdded(20)
          .then((r) => r.audiobooks ?? [])
          .catch(() => []),
        pcApi
          .recentlyAdded(20)
          .then((r) => r.podcasts ?? [])
          .catch(() => []),
        pcApi
          .withNewEpisodes(20)
          .then((r) => r.podcasts ?? [])
          .catch(() => []),
        spApi.list().then((r) => r ?? []),
        abApi
          .inProgress(10)
          .then((r) => r.audiobooks ?? [])
          .catch(() => []),
      ]);
      dataFetched = true;
    } catch {
      // ignore — user may not be logged in
    } finally {
      loading = false;
    }
  }

  // ── Online data loading ──────────────────────────────────────────────────────
  const INTERVALS: { key: Interval; label: string }[] = [
    { key: "today", label: "Today" },
    { key: "week", label: "Week" },
    { key: "month", label: "Month" },
    { key: "all", label: "All time" },
    { key: "custom", label: "Custom" },
  ];

  function getDateRange(): { from?: string; to?: string } {
    const now = new Date();
    switch (interval) {
      case "today": {
        const from = new Date(now);
        from.setHours(0, 0, 0, 0);
        return { from: from.toISOString() };
      }
      case "week": {
        const from = new Date(now);
        from.setDate(from.getDate() - 7);
        return { from: from.toISOString() };
      }
      case "month": {
        const from = new Date(now);
        from.setDate(from.getDate() - 30);
        return { from: from.toISOString() };
      }
      case "custom": {
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
        libApi.mostPlayed(100, from, to).then((r) => r ?? []),
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
    if (iv === "custom") {
      if (customFrom) loadPlays();
    } else {
      loadPlays();
    }
  }

  function handleCustomDateChange() {
    loadPlays();
  }

  onMount(() => {
    if ($isOffline) {
      loading = false;
      // Subscribe so we load data as soon as the backend comes back online.
      // store.subscribe fires immediately with the current value; since we're
      // inside the $isOffline branch that value is true, so the guard below
      // won't fire on the first call.
      const unsub = isOffline.subscribe(async (offline) => {
        if (!offline && !dataFetched) {
          unsub();
          await loadHomeData();
        }
      });
      return unsub;
    }

    void (async () => {
      if (isRestoring && (recentTracks.length > 0 || recentAlbums.length > 0)) {
        loading = false;
        isRestoring = false;
        dataFetched = true;
        return;
      }
      await loadHomeData();
    })();
  });

  // ── Greeting ─────────────────────────────────────────────────────────────────
  const hour = new Date().getHours();
  const greeting =
    hour < 12 ? "Good morning" : hour < 17 ? "Good afternoon" : "Good evening";

  // ── Mixed "What's New" shelf ──────────────────────────────────────────────────
  type MediaItem = {
    type: "album" | "audiobook" | "podcast";
    id: string;
    title: string;
    subtitle: string;
    cover_art_key?: string;
    href: string;
  };

  $: mixedNew = ((): MediaItem[] => {
    const albums: MediaItem[] = newAlbums.slice(0, 8).map((a) => ({
      type: "album",
      id: a.id,
      title: a.title,
      subtitle: a.artist_name ?? "",
      cover_art_key: a.cover_art_key,
      href: `/library/albums/${a.id}`,
    }));
    const books: MediaItem[] = newAudiobooks.slice(0, 5).map((b) => ({
      type: "audiobook",
      id: b.id,
      title: b.title,
      subtitle: b.author_name ?? "",
      cover_art_key: b.cover_art_key,
      href: `/audiobooks/${b.id}`,
    }));
    const pods: MediaItem[] = newPodcasts.slice(0, 5).map((p) => ({
      type: "podcast",
      id: p.id,
      title: p.title,
      subtitle: p.author ?? "",
      cover_art_key: p.cover_art_key,
      href: `/podcasts/${p.id}`,
    }));
    return [...albums, ...books, ...pods];
  })();

  function coverSrc(item: MediaItem): string {
    if (!item.cover_art_key) return "";
    const base = getApiBase();
    if (item.type === "audiobook") return `${base}/covers/audiobook/${item.id}`;
    if (item.type === "podcast") return `${base}/covers/podcast/${item.id}`;
    return `${base}/covers/${item.id}`;
  }
</script>

<svelte:head><title>Home – Orb</title></svelte:head>

<!-- ── Offline view ─────────────────────────────────────────────────────────── -->
{#if $isOffline}
  <div class="offline-view">
    {#if offlineBooksList.length > 0}
      <section class="home-section">
        <div class="section-header">
          <h2 class="section-title">Downloaded Audiobooks</h2>
        </div>
        <div class="media-shelf">
          {#each offlineBooksList as book (book.id)}
            <a class="media-card" href="/audiobooks/{book.id}">
              <div class="media-card-cover">
                {#if book.cover_art_key}
                  <img
                    src="{getApiBase()}/covers/audiobook/{book.id}"
                    alt={book.title}
                    loading="lazy"
                  />
                {:else}
                  <div class="media-card-placeholder">
                    <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                      <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
                      <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
                    </svg>
                  </div>
                {/if}
                <span class="type-badge type-audiobook">Book</span>
                <button
                  class="mc-play-btn"
                  aria-label="Play {book.title}"
                  on:click|preventDefault|stopPropagation={() => playAudiobook(book, 0)}
                >
                  <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M4 2.5l10 5.5-10 5.5V2.5z" /></svg>
                </button>
              </div>
              <div class="media-card-info">
                <span class="media-card-title" title={book.title}>{book.title}</span>
                {#if book.author_name}<span class="media-card-sub">{book.author_name}</span>{/if}
              </div>
            </a>
          {/each}
        </div>
      </section>
    {/if}

    <div class="offline-header">
      <div class="offline-title-row">
        <h2 class="page-title">Downloads</h2>
        <span class="offline-badge">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <line x1="1" y1="1" x2="23" y2="23" />
            <path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55" />
            <path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39" />
            <path d="M10.71 5.05A16 16 0 0 1 22.56 9" />
            <path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88" />
            <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
            <line x1="12" y1="20" x2="12.01" y2="20" />
          </svg>
          Offline
        </span>
      </div>
      {#if offlineTracks.length > 0}
        <div class="offline-actions">
          <button class="btn-play" on:click={playAllOffline}>
            <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M4 2.5l10 5.5-10 5.5V2.5z" /></svg>
            Play all
          </button>
          <button class="btn-secondary" on:click={shuffleOffline}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <polyline points="16 3 21 3 21 8" /><line x1="4" y1="20" x2="21" y2="3" />
              <polyline points="21 16 21 21 16 21" /><line x1="15" y1="15" x2="21" y2="21" />
              <line x1="4" y1="4" x2="9" y2="9" />
            </svg>
            Shuffle
          </button>
          <span class="track-count">{offlineTracks.length} track{offlineTracks.length === 1 ? "" : "s"}</span>
        </div>
      {/if}
    </div>

    {#if offlineTracks.length === 0}
      <div class="empty-state">
        <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <line x1="1" y1="1" x2="23" y2="23" />
          <path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55" />
          <path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39" />
          <path d="M10.71 5.05A16 16 0 0 1 22.56 9" />
          <path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88" />
          <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
          <line x1="12" y1="20" x2="12.01" y2="20" />
        </svg>
        <p>You're offline with no downloaded tracks.</p>
        <p class="muted">Download your favorites while connected.</p>
      </div>
    {:else}
      <TrackList tracks={offlineTracks} showCover={true} />
    {/if}
  </div>

<!-- ── Loading skeleton ───────────────────────────────────────────────────── -->
{:else if loading}
  <div class="skeleton-home">
    <div class="skeleton-nav">
      {#each { length: 4 } as _}
        <Skeleton width="100%" height="60px" radius="10px" />
      {/each}
    </div>
    <div class="skeleton-section">
      <Skeleton width="150px" height="1.1rem" radius="4px" />
      <div class="skeleton-shelf">
        {#each { length: 7 } as _}
          <div class="skeleton-card">
            <Skeleton width="140px" height="140px" radius="8px" />
            <Skeleton width="90px" height="0.8rem" />
            <Skeleton width="60px" height="0.72rem" />
          </div>
        {/each}
      </div>
    </div>
    <div class="skeleton-section">
      <Skeleton width="140px" height="1.1rem" radius="4px" />
      <div class="skeleton-shelf">
        {#each { length: 7 } as _}
          <div class="skeleton-card">
            <Skeleton width="140px" height="140px" radius="8px" />
            <Skeleton width="80px" height="0.8rem" />
            <Skeleton width="55px" height="0.72rem" />
          </div>
        {/each}
      </div>
    </div>
  </div>

<!-- ── Online home ────────────────────────────────────────────────────────── -->
{:else}
  <div class="home">

    <!-- Greeting + media-type nav -->
    <div class="home-header">
      <p class="greeting">{greeting}</p>
    </div>

    <nav class="media-nav" aria-label="Browse media types">
      <a href="/library" class="mnc mnc-music">
        <div class="mnc-icon" aria-hidden="true">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M9 18V5l12-2v13" />
            <circle cx="6" cy="18" r="3" />
            <circle cx="18" cy="16" r="3" />
          </svg>
        </div>
        <div class="mnc-text">
          <span class="mnc-label">Music</span>
          <span class="mnc-sub">Albums &amp; tracks</span>
        </div>
      </a>
      <a href="/audiobooks" class="mnc mnc-books">
        <div class="mnc-icon" aria-hidden="true">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
            <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
          </svg>
        </div>
        <div class="mnc-text">
          <span class="mnc-label">Audiobooks</span>
          <span class="mnc-sub">Listen &amp; follow along</span>
        </div>
      </a>
      <a href="/podcasts" class="mnc mnc-podcasts">
        <div class="mnc-icon" aria-hidden="true">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
            <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
            <line x1="12" y1="19" x2="12" y2="23" />
            <line x1="8" y1="23" x2="16" y2="23" />
          </svg>
        </div>
        <div class="mnc-text">
          <span class="mnc-label">Podcasts</span>
          <span class="mnc-sub">Subscribe &amp; discover</span>
        </div>
      </a>
      <a href="/playlists" class="mnc mnc-playlists">
        <div class="mnc-icon" aria-hidden="true">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="8" y1="6" x2="21" y2="6" /><line x1="8" y1="12" x2="21" y2="12" /><line x1="8" y1="18" x2="21" y2="18" />
            <line x1="3" y1="6" x2="3.01" y2="6" /><line x1="3" y1="12" x2="3.01" y2="12" /><line x1="3" y1="18" x2="3.01" y2="18" />
          </svg>
        </div>
        <div class="mnc-text">
          <span class="mnc-label">Playlists</span>
          <span class="mnc-sub">Curated &amp; smart</span>
        </div>
      </a>
    </nav>

    <!-- Continue Listening -->
    {#if inProgressBooks.length > 0}
      <section class="home-section">
        <div class="section-header">
          <h2 class="section-title">Continue Listening</h2>
          <a href="/audiobooks" class="view-all">View all</a>
        </div>
        <div class="cl-shelf">
          {#each inProgressBooks as book (book.id)}
            {@const pct = book.duration_ms > 0 ? Math.min(100, (book.position_ms / book.duration_ms) * 100) : 0}
            <a class="cl-card" href="/audiobooks/{book.id}">
              <div class="cl-cover-wrap">
                {#if book.cover_art_key}
                  <img
                    src="{getApiBase()}/covers/audiobook/{book.id}"
                    alt={book.title}
                    class="cl-cover"
                    loading="lazy"
                  />
                {:else}
                  <div class="cl-cover cl-placeholder">
                    <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                      <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
                      <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
                    </svg>
                  </div>
                {/if}
                <div class="cl-progress-bar">
                  <div class="cl-progress-fill" style="width:{pct}%"></div>
                </div>
                <button
                  class="mc-play-btn"
                  aria-label="Resume {book.title}"
                  on:click|preventDefault|stopPropagation={() => playAudiobook(book, book.position_ms)}
                >
                  <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M4 2.5l10 5.5-10 5.5V2.5z" /></svg>
                </button>
              </div>
              <div class="cl-info">
                <span class="cl-title" title={book.title}>{book.title}</span>
                {#if book.author_name}<span class="cl-author">{book.author_name}</span>{/if}
                {#if book.series}
                  <button
                    type="button"
                    class="cl-series"
                    aria-label="View series: {book.series}"
                    on:click|stopPropagation={() => goto(`/audiobooks/series/${encodeURIComponent(book.series!)}`)}
                    on:keydown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); goto(`/audiobooks/series/${encodeURIComponent(book.series!)}`); } }}
                  >{book.series}{book.series_index != null ? ` #${book.series_index}` : ""}</button>
                {/if}
                <div class="cl-pct-row">
                  <div class="cl-pct-track">
                    <div class="cl-pct-fill" style="width:{pct}%"></div>
                  </div>
                  <span class="cl-pct-label">{Math.round(pct)}%</span>
                </div>
              </div>
            </a>
          {/each}
        </div>
      </section>
    {/if}

    <!-- What's New: mixed albums + audiobooks + podcasts -->
    {#if mixedNew.length > 0}
      <section class="home-section">
        <div class="section-header">
          <h2 class="section-title">What's New</h2>
        </div>
        <div class="media-shelf">
          {#each mixedNew as item (item.type + item.id)}
            <a class="media-card" href={item.href}>
              <div class="media-card-cover">
                {#if item.cover_art_key}
                  <img src={coverSrc(item)} alt={item.title} loading="lazy" />
                {:else}
                  <div class="media-card-placeholder">
                    {#if item.type === "audiobook"}
                      <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                        <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
                        <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
                      </svg>
                    {:else if item.type === "podcast"}
                      <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                        <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
                        <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
                        <line x1="12" y1="19" x2="12" y2="23" />
                        <line x1="8" y1="23" x2="16" y2="23" />
                      </svg>
                    {:else}
                      <span style="font-size:2rem;opacity:.4">♪</span>
                    {/if}
                  </div>
                {/if}
                <span class="type-badge type-{item.type}">
                  {item.type === "album" ? "Music" : item.type === "audiobook" ? "Book" : "Podcast"}
                </span>
              </div>
              <div class="media-card-info">
                <span class="media-card-title" title={item.title}>{item.title}</span>
                {#if item.subtitle}<span class="media-card-sub">{item.subtitle}</span>{/if}
              </div>
            </a>
          {/each}
        </div>
      </section>
    {/if}

    <!-- Podcasts with new episodes -->
    {#if podcastsWithNewEpisodes.length > 0}
      <section class="home-section">
        <div class="section-header">
          <h2 class="section-title">New Episodes</h2>
          <a href="/podcasts" class="view-all">View all</a>
        </div>
        <div class="media-shelf">
          {#each podcastsWithNewEpisodes as pc (pc.id)}
            <a class="media-card" href="/podcasts/{pc.id}">
              <div class="media-card-cover">
                {#if pc.cover_art_key}
                  <img src="{getApiBase()}/covers/podcast/{pc.id}" alt={pc.title} loading="lazy" />
                {:else}
                  <div class="media-card-placeholder">
                    <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                      <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
                      <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
                      <line x1="12" y1="19" x2="12" y2="23" />
                      <line x1="8" y1="23" x2="16" y2="23" />
                    </svg>
                  </div>
                {/if}
                <span class="type-badge type-podcast new-ep-badge">
                  <span class="new-dot"></span>New
                </span>
              </div>
              <div class="media-card-info">
                <span class="media-card-title" title={pc.title}>{pc.title}</span>
                {#if pc.author}<span class="media-card-sub">{pc.author}</span>{/if}
              </div>
            </a>
          {/each}
        </div>
      </section>
    {/if}

    <!-- Listening stats (Recently + Most played) -->
    {#if recentTracks.length > 0 || mostTracks.length > 0}
      <section class="home-section stats-section">
        <div class="stats-header">
          <h2 class="section-title">Your Listening</h2>
          <div class="interval-tabs">
            {#each INTERVALS as iv}
              <button
                class="interval-tab"
                class:active={interval === iv.key}
                on:click={() => selectInterval(iv.key)}
              >{iv.label}</button>
            {/each}
          </div>
        </div>
        {#if interval === "custom"}
          <div class="date-range">
            <input type="date" class="date-input" bind:value={customFrom} on:change={handleCustomDateChange} />
            <span class="date-sep">–</span>
            <input type="date" class="date-input" bind:value={customTo} on:change={handleCustomDateChange} />
          </div>
        {/if}
        <div class="plays-columns">
          <div class="plays-col">
            <div class="col-header">
              <h3 class="col-title">Recently Played</h3>
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
              <p class="muted"><Spinner /></p>
            {:else if pagedRecent.length > 0}
              <TrackList tracks={pagedRecent} showCover showDiscNumbers={false} />
            {:else}
              <p class="muted">No plays in this period.</p>
            {/if}
          </div>
          <div class="plays-col">
            <div class="col-header">
              <h3 class="col-title">Most Played</h3>
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
              <p class="muted"><Spinner /></p>
            {:else if pagedMost.length > 0}
              <TrackList tracks={pagedMost} showCover showDiscNumbers={false} />
            {:else}
              <p class="muted">No plays in this period.</p>
            {/if}
          </div>
        </div>
      </section>
    {/if}

<!-- Recently Played Albums -->
    {#if recentAlbums.length > 0}
      <section class="home-section">
        <div class="section-header">
          <h2 class="section-title">Recently Played</h2>
        </div>
        <div class="album-slider">
          {#each recentAlbums as album (album.id)}
            <div class="slider-item"><AlbumCard {album} /></div>
          {/each}
        </div>
      </section>
    {/if}

    <!-- Smart Playlists -->
    {#if smartPls.length > 0}
      <section class="home-section">
        <div class="section-header">
          <h2 class="section-title">Smart Playlists</h2>
          <a href="/playlists" class="view-all">View all</a>
        </div>
        <div class="sp-grid">
          {#each smartPls.slice(0, 6) as sp (sp.id)}
            <a class="sp-card" href="/smart-playlists/{sp.id}">
              <div class="sp-icon" aria-hidden="true">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <line x1="8" y1="6" x2="21" y2="6" /><line x1="8" y1="12" x2="21" y2="12" /><line x1="8" y1="18" x2="21" y2="18" />
                  <line x1="3" y1="6" x2="3.01" y2="6" /><line x1="3" y1="12" x2="3.01" y2="12" /><line x1="3" y1="18" x2="3.01" y2="18" />
                </svg>
              </div>
              <div class="sp-info">
                <span class="sp-name">{sp.name}</span>
                <span class="sp-meta">{sp.rules.length} rule{sp.rules.length === 1 ? "" : "s"}</span>
              </div>
            </a>
          {/each}
        </div>
      </section>
    {/if}

    {#if mixedNew.length === 0 && recentAlbums.length === 0 && recentTracks.length === 0 && mostTracks.length === 0 && smartPls.length === 0 && inProgressBooks.length === 0 && podcastsWithNewEpisodes.length === 0}
      <p class="muted empty-hint">Nothing here yet. Start listening to build your library!</p>
    {/if}
  </div>
{/if}

<style>
  /* ── Shared utilities ────────────────────────────────────────────────────── */
  .muted { color: var(--text-muted); font-size: 0.875rem; }

  /* ── Offline view ─────────────────────────────────────────────────────────── */
  .offline-view { padding-top: 4px; }

  .offline-header { margin-bottom: 24px; }

  .offline-title-row {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
  }
  .page-title {
    font-size: 1.25rem;
    font-weight: 600;
    margin: 0;
  }
  .offline-badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 0.72rem;
    font-weight: 600;
    color: var(--text-muted);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 3px 10px;
  }
  .offline-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .btn-play {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    background: var(--accent);
    border: none;
    border-radius: 20px;
    padding: 7px 18px;
    color: #fff;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s;
  }
  .btn-play:hover { background: var(--accent-hover); }
  .btn-secondary {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 6px 14px;
    color: var(--text-muted);
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s;
  }
  .btn-secondary:hover { color: var(--text); border-color: var(--text-muted); }
  .track-count { font-size: 0.8rem; color: var(--text-muted); margin-left: 4px; }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 48px 16px;
    color: var(--text-muted);
    text-align: center;
  }
  .empty-state p { margin: 0; font-size: 0.9rem; }
  .empty-state svg { opacity: 0.35; }

  /* ── Loading skeleton ────────────────────────────────────────────────────── */
  .skeleton-home { display: flex; flex-direction: column; gap: 36px; }

  .skeleton-nav {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 10px;
  }
  @media (max-width: 600px) {
    .skeleton-nav { grid-template-columns: repeat(2, 1fr); }
  }

  .skeleton-section { display: flex; flex-direction: column; gap: 14px; }

  .skeleton-shelf {
    display: flex;
    gap: 14px;
    overflow: hidden;
  }
  .skeleton-card {
    display: flex;
    flex-direction: column;
    gap: 7px;
    flex: 0 0 140px;
  }

  /* ── Online home ─────────────────────────────────────────────────────────── */
  .home { display: flex; flex-direction: column; }

  .home-header { margin-bottom: 6px; }

  .greeting {
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--text-muted);
    margin: 0 0 16px;
    letter-spacing: 0.02em;
    text-transform: uppercase;
  }

  /* ── Media type nav ─────────────────────────────────────────────────────── */
  .media-nav {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 10px;
    margin-bottom: 40px;
  }
  @media (max-width: 700px) {
    .media-nav { grid-template-columns: repeat(2, 1fr); }
  }

  .mnc {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 14px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 10px;
    text-decoration: none;
    color: var(--text);
    transition: background 0.15s, border-color 0.15s, transform 0.15s;
    min-width: 0;
  }
  .mnc:hover {
    background: var(--bg-hover);
    border-color: var(--text-muted);
    transform: translateY(-1px);
  }

  .mnc-icon {
    width: 38px;
    height: 38px;
    border-radius: 9px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .mnc-music .mnc-icon  { background: var(--accent-dim); color: var(--accent); }
  .mnc-books .mnc-icon  { background: rgba(232,162,70,.15); color: #e8a246; }
  .mnc-podcasts .mnc-icon { background: rgba(76,175,142,.15); color: #4caf8e; }
  .mnc-playlists .mnc-icon { background: rgba(184,124,212,.15); color: #b87cd4; }

  .mnc:hover .mnc-icon {
    filter: brightness(1.1);
  }

  .mnc-text {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
    overflow: hidden;
  }
  .mnc-label {
    font-size: 0.875rem;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .mnc-sub {
    font-size: 0.7rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* ── Sections ────────────────────────────────────────────────────────────── */
  .home-section { margin-bottom: 40px; }

  .section-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 16px;
  }
  .section-title {
    font-size: 1.05rem;
    font-weight: 600;
    margin: 0;
  }
  .view-all {
    font-size: 0.78rem;
    color: var(--text-muted);
    text-decoration: none;
    letter-spacing: 0.02em;
    transition: color 0.15s;
  }
  .view-all:hover { color: var(--text); }

  /* ── Continue Listening shelf ────────────────────────────────────────────── */
  .cl-shelf {
    display: flex;
    gap: 14px;
    overflow-x: auto;
    padding-bottom: 8px;
    scrollbar-width: thin;
    scrollbar-color: var(--border) transparent;
  }
  .cl-shelf::-webkit-scrollbar { height: 4px; }
  .cl-shelf::-webkit-scrollbar-track { background: transparent; }
  .cl-shelf::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }

  .cl-card {
    flex: 0 0 150px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    text-decoration: none;
    color: inherit;
  }

  .cl-cover-wrap {
    position: relative;
    width: 100%;
    padding-bottom: 150%;
    border-radius: 8px;
    overflow: hidden;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
  }
  .cl-cover {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.25s;
  }
  .cl-card:hover .cl-cover { transform: scale(1.03); }
  .cl-placeholder {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    opacity: 0.3;
  }

  .cl-progress-bar {
    position: absolute;
    bottom: 0;
    left: 0;
    right: 0;
    height: 3px;
    background: rgba(255,255,255,.12);
  }
  .cl-progress-fill {
    height: 100%;
    background: var(--accent);
    border-radius: 0 2px 2px 0;
    transition: width 0.3s;
  }

  .cl-info {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .cl-title {
    font-size: 0.8rem;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--text);
  }
  .cl-author {
    font-size: 0.72rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .cl-series {
    font-size: 0.7rem;
    color: var(--accent);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    cursor: pointer;
    background: none;
    border: none;
    padding: 0;
    text-align: left;
  }
  .cl-series:hover { text-decoration: underline; }
  .cl-pct-row {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-top: 2px;
  }
  .cl-pct-track {
    flex: 1;
    height: 2px;
    background: var(--border);
    border-radius: 1px;
    overflow: hidden;
  }
  .cl-pct-fill {
    height: 100%;
    background: var(--accent);
    border-radius: 1px;
  }
  .cl-pct-label {
    font-size: 0.67rem;
    color: var(--text-muted);
    flex-shrink: 0;
  }

  /* ── Shared play button on cards ─────────────────────────────────────────── */
  .mc-play-btn {
    position: absolute;
    bottom: 8px;
    right: 8px;
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--accent);
    border: none;
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    opacity: 0;
    transform: translateY(4px);
    transition: opacity 0.2s, transform 0.2s;
    box-shadow: 0 2px 8px rgba(0,0,0,.4);
    z-index: 2;
  }
  .cl-card:hover .mc-play-btn,
  .media-card:hover .mc-play-btn {
    opacity: 1;
    transform: translateY(0);
  }
  .mc-play-btn:hover { filter: brightness(1.1); }

  /* ── Unified media shelf ─────────────────────────────────────────────────── */
  .media-shelf {
    display: flex;
    gap: 14px;
    overflow-x: auto;
    padding-bottom: 8px;
    scrollbar-width: thin;
    scrollbar-color: var(--border) transparent;
  }
  .media-shelf::-webkit-scrollbar { height: 4px; }
  .media-shelf::-webkit-scrollbar-track { background: transparent; }
  .media-shelf::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }

  .media-card {
    flex: 0 0 140px;
    width: 140px;
    max-width: 140px;
    display: flex;
    flex-direction: column;
    gap: 7px;
    text-decoration: none;
    color: inherit;
  }

  .media-card-cover {
    position: relative;
    width: 140px;
    height: 140px;
    flex-shrink: 0;
    border-radius: 8px;
    overflow: hidden;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
  }
  .media-card-cover img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.25s;
  }
  .media-card:hover .media-card-cover img { transform: scale(1.03); }

  .media-card-placeholder {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    opacity: 0.3;
  }

  .media-card-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 140px;
    max-width: 140px;
    overflow: hidden;
  }
  .media-card-title {
    font-size: 0.8rem;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--text);
  }
  .media-card-sub {
    font-size: 0.72rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* ── Type badges ─────────────────────────────────────────────────────────── */
  .type-badge {
    position: absolute;
    top: 6px;
    left: 6px;
    font-size: 0.58rem;
    font-weight: 700;
    letter-spacing: 0.07em;
    text-transform: uppercase;
    padding: 2px 6px;
    border-radius: 3px;
    pointer-events: none;
    z-index: 1;
  }
  .type-album    { background: rgba(0,0,0,.5); color: #fff; border: 1px solid rgba(255,255,255,.15); }
  .type-audiobook { background: rgba(232,162,70,.85); color: #fff; }
  .type-podcast  { background: rgba(76,175,142,.85); color: #fff; }

  .new-ep-badge {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .new-dot {
    display: inline-block;
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: #fff;
    flex-shrink: 0;
  }

  /* ── Album slider ────────────────────────────────────────────────────────── */
  .album-slider {
    display: flex;
    gap: 14px;
    overflow-x: auto;
    padding: 8px 12px 2px 12px;
    scrollbar-width: thin;
    scrollbar-color: var(--border) transparent;
  }
  .album-slider::-webkit-scrollbar { height: 4px; }
  .album-slider::-webkit-scrollbar-track { background: transparent; }
  .album-slider::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }

  .slider-item {
    flex: 0 0 154px;
    min-width: 154px;
    max-width: 154px;
    display: flex;
    flex-direction: column;
  }
  .slider-item :global(.album-card) {
    width: 154px;
    max-width: 154px;
    box-sizing: border-box;
  }
  /* ── Smart playlists grid ────────────────────────────────────────────────── */
  .sp-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(175px, 1fr));
    gap: 8px;
  }
  .sp-card {
    display: flex;
    align-items: center;
    gap: 10px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 12px;
    text-decoration: none;
    color: inherit;
    transition: background 0.15s, border-color 0.15s;
  }
  .sp-card:hover { background: var(--bg-hover); border-color: var(--text-muted); }
  .sp-icon {
    width: 34px;
    height: 34px;
    background: rgba(184,124,212,.15);
    border-radius: 6px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    color: #b87cd4;
  }
  .sp-info { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .sp-name {
    font-size: 0.82rem;
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .sp-meta { font-size: 0.7rem; color: var(--text-muted); }

  /* ── Listening stats ─────────────────────────────────────────────────────── */
  .stats-section {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 20px 24px 24px;
    min-width: 0;
    overflow: hidden;
  }

  .stats-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 12px;
    margin-bottom: 16px;
  }

  .interval-tabs { display: flex; flex-wrap: wrap; gap: 4px; }
  .interval-tab {
    background: none;
    border: 1px solid var(--border);
    border-radius: 20px;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 0.75rem;
    padding: 3px 11px;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
  }
  .interval-tab:hover { color: var(--text); border-color: var(--text-muted); }
  .interval-tab.active { background: var(--accent); border-color: var(--accent); color: #fff; }

  .date-range {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 16px;
  }
  .date-input {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text);
    font-size: 0.8rem;
    padding: 3px 8px;
    cursor: pointer;
  }
  .date-input:focus { outline: none; border-color: var(--accent); }
  .date-sep { color: var(--text-muted); font-size: 0.8rem; }

  .plays-columns {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 32px;
    min-width: 0;
  }
  @media (max-width: 900px) {
    .plays-columns { grid-template-columns: 1fr; }
  }

  .plays-col { min-width: 0; }

  .col-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 10px;
  }
  .col-title { font-size: 0.9rem; font-weight: 600; margin: 0; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.04em; }

  .page-label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.78rem;
    color: var(--text-muted);
  }
  .page-select {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text-muted);
    font-size: 0.78rem;
    padding: 2px 6px;
    cursor: pointer;
  }
  .page-select:focus { outline: none; border-color: var(--accent); }

  .empty-hint {
    text-align: center;
    padding: 48px 0;
  }
</style>
