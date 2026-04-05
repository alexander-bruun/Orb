<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { library as libApi } from "$lib/api/library";
  import { admin as adminApi } from "$lib/api/admin";
  import AlbumGrid from "$lib/components/library/AlbumGrid.svelte";
  import AlphaScrollbar from "$lib/components/library/AlphaScrollbar.svelte";
  import type { Album } from "$lib/types";
  import Spinner from "$lib/components/ui/Spinner.svelte";

  type SortMode = "title" | "artist" | "year" | "channels";

  const SORT_MODES: { mode: SortMode; label: string }[] = [
    { mode: "title", label: "Title" },
    { mode: "artist", label: "Artist" },
    { mode: "year", label: "Year" },
    { mode: "channels", label: "Channels" },
  ];

  const PAGE_SIZE = 500;
  const SORT_BY_KEY = "library_sort_by";
  const SORT_DIR_KEY = "library_sort_dir";

  let albums: Album[] = [];
  let totalCount = 0;
  let loading = true;
  let loadingMore = false;
  let nextOffset = 0;
  let sortBy: SortMode = "title";
  let sortDir: "asc" | "desc" = "asc";
  let activeKey = "";
  let scrollEl: HTMLElement | null = null;
  let sentinel: HTMLElement;
  let observer: IntersectionObserver | null = null;
  let ingestES: EventSource | null = null;
  let isRestoring = false;

  if (typeof localStorage !== "undefined") {
    const savedSortBy = localStorage.getItem(SORT_BY_KEY);
    if (
      savedSortBy &&
      ["title", "artist", "year", "channels"].includes(savedSortBy)
    ) {
      sortBy = savedSortBy as SortMode;
    }
    const savedSortDir = localStorage.getItem(SORT_DIR_KEY);
    if (savedSortDir === "asc" || savedSortDir === "desc") {
      sortDir = savedSortDir;
    }
  }

  export const snapshot = {
    capture: () => ({
      albums,
      totalCount,
      nextOffset,
      sortBy,
      sortDir,
      activeKey,
    }),
    restore: (value) => {
      albums = value.albums;
      totalCount = value.totalCount;
      nextOffset = value.nextOffset;
      sortBy = value.sortBy;
      sortDir = value.sortDir;
      activeKey = value.activeKey;
      isRestoring = true;
      loading = false;
    },
  };

  // ── Grouping (no client-side sort — DB returns in the right order) ─────────

  function getSortKey(album: Album, mode: SortMode): string {
    switch (mode) {
      case "title": {
        const first = album.title
          .replace(/^(the |a |an )\s*/i, "")
          .charAt(0)
          .toUpperCase();
        return /[A-Z]/.test(first) ? first : "#";
      }
      case "artist": {
        const name = album.artist_name ?? "";
        const first = name
          .replace(/^(the |a |an )\s*/i, "")
          .charAt(0)
          .toUpperCase();
        return first && /[A-Z]/.test(first) ? first : "#";
      }
      case "year":
        return album.release_year ? String(album.release_year) : "?";
      case "channels":
        if (album.max_channels === 8) return "7.1";
        if (album.max_channels === 6) return "5.1";
        if (album.max_channels && album.max_channels > 2) {
          return `${album.max_channels}ch`;
        }
        return "Stereo";
    }
  }

  // Albums arrive pre-sorted from the server; just bucket them in order.
  function computeGrouped(list: Album[], mode: SortMode): Map<string, Album[]> {
    const map = new Map<string, Album[]>();
    for (const album of list) {
      const key = getSortKey(album, mode);
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(album);
    }
    return map;
  }

  function computeKeys(map: Map<string, Album[]>, mode: SortMode): string[] {
    // Since the albums are already sorted by the backend and computeGrouped
    // preserves that order in the Map keys, we can just return the keys as-is.
    return [...map.keys()];
  }

  $: grouped = computeGrouped(albums, sortBy);
  $: keys = computeKeys(grouped, sortBy);
  $: hasMore = albums.length < totalCount;

  // Reset scroll + active key whenever keys changes (sort changed or first load)
  $: {
    keys; // declare dependency
    if (!isRestoring) {
      activeKey = keys[0] ?? "";
      scrollEl?.scrollTo({ top: 0 });
    }
  }

  // ── Scroll tracking ───────────────────────────────────────────────────────

  function updateActive() {
    if (!scrollEl) return;
    const sections = scrollEl.querySelectorAll("[data-scroll-key]");
    const containerTop = scrollEl.getBoundingClientRect().top;
    let current = keys[0] ?? "";
    for (const section of sections) {
      const top = section.getBoundingClientRect().top - containerTop;
      if (top <= 64) {
        current = section.getAttribute("data-scroll-key") ?? current;
      }
    }
    activeKey = current;
  }

  // ── Infinite scroll ───────────────────────────────────────────────────────

  async function loadNextPage() {
    if (loadingMore || !hasMore) return;
    loadingMore = true;
    try {
      const page = await libApi.albums(PAGE_SIZE, nextOffset, sortBy, sortDir);
      if (page.items.length === 0) {
        nextOffset = totalCount; // force-stop
        return;
      }
      albums = [...albums, ...page.items];
      nextOffset += PAGE_SIZE;
    } catch {
      // silently stop; scrolling again will retry via the observer
    } finally {
      loadingMore = false;
    }
  }

  function setupObserver() {
    observer?.disconnect();
    if (!sentinel) return;
    observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting) loadNextPage();
      },
      { root: scrollEl, rootMargin: "200px" },
    );
    observer.observe(sentinel);
  }

  // ── Sort change ───────────────────────────────────────────────────────────

  async function changeSortBy(mode: SortMode) {
    if (mode === sortBy) {
      sortDir = sortDir === "asc" ? "desc" : "asc";
    } else {
      sortBy = mode;
      sortDir = "asc";
    }

    if (typeof localStorage !== "undefined") {
      localStorage.setItem(SORT_BY_KEY, sortBy);
      localStorage.setItem(SORT_DIR_KEY, sortDir);
    }

    observer?.disconnect();
    observer = null;
    albums = [];
    nextOffset = 0;
    totalCount = 0;
    loading = true;
    try {
      const first = await libApi.albums(PAGE_SIZE, 0, sortBy, sortDir);
      totalCount = first.total;
      albums = first.items;
      nextOffset = PAGE_SIZE;
    } catch {
      // ignore; loading indicator will remain
    } finally {
      loading = false;
    }
    setupObserver();
  }

  // ── Mount / destroy ───────────────────────────────────────────────────────

  onMount(() => {
    scrollEl = document.querySelector("main.content");
    if (scrollEl)
      scrollEl.addEventListener("scroll", updateActive, { passive: true });
    loadFirstPage();
    ingestES = adminApi.openIngestStream((e) => {
      if (e.type === "complete") refreshAlbums();
    });
  });

  onDestroy(() => {
    scrollEl?.removeEventListener("scroll", updateActive);
    observer?.disconnect();
    ingestES?.close();
  });

  async function refreshAlbums() {
    try {
      const first = await libApi.albums(PAGE_SIZE, 0, sortBy, sortDir);
      if (first.total === totalCount) return; // nothing new
      totalCount = first.total;
      albums = first.items;
      nextOffset = PAGE_SIZE;
    } catch {
      // silently ignore
    }
    setupObserver();
  }

  async function loadFirstPage() {
    if (isRestoring && albums.length > 0) {
      loading = false;
      setupObserver();
      setTimeout(() => {
        isRestoring = false;
      }, 0);
      return;
    }

    try {
      const first = await libApi.albums(PAGE_SIZE, 0, sortBy, sortDir);
      totalCount = first.total;
      albums = first.items;
      nextOffset = PAGE_SIZE;
    } catch {
      // leave loading=true; user can refresh
    } finally {
      loading = false;
    }
    setupObserver();
  }
</script>

<div class="page">
  <div class="page-header">
    <div class="title-row">
      <h1 class="title">Albums</h1>
      {#if !loading && totalCount > 0}
        <span class="count">{totalCount}</span>
      {/if}
    </div>
    <div class="sort-controls">
      {#each SORT_MODES as { mode, label }}
        <button
          class="sort-btn"
          class:active={sortBy === mode}
          on:click={() => changeSortBy(mode)}
        >
          {label}
          {#if sortBy === mode}
            <span class="dir-arrow">{sortDir === "asc" ? "↑" : "↓"}</span>
          {/if}
        </button>
      {/each}
    </div>
  </div>

  {#if loading}
    <div class="sk-grid">
      {#each { length: 16 } as _}
        <div class="sk-card">
          <div class="sk-cover"></div>
          <div class="sk-line sk-line--title"></div>
          <div class="sk-line sk-line--sub"></div>
        </div>
      {/each}
    </div>
  {:else}
    <AlbumGrid {grouped} {keys} />
    <div bind:this={sentinel} class="sentinel"></div>
    {#if loadingMore}
      <p class="load-more-hint"><Spinner size={18} /></p>
    {/if}
  {/if}
</div>

<AlphaScrollbar {keys} {activeKey} {scrollEl} />

<svelte:head><title>Library – Orb</title></svelte:head>

<style>
  .page {
    padding-right: 40px; /* clear room for the alpha scrollbar */
    min-height: 100%;
  }

  .page-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 20px;
    flex-wrap: wrap;
    gap: 12px;
  }
  .title-row {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }
  .title {
    font-size: 1.25rem;
    font-weight: 700;
    margin: 0;
  }
  .count {
    font-size: 0.8rem;
    font-weight: 400;
    color: var(--text-muted);
  }

  .sort-controls {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .sort-btn {
    all: unset;
    cursor: pointer;
    font-size: 0.6875rem;
    font-weight: 600;
    padding: 4px 10px;
    border-radius: 4px;
    color: var(--text-muted);
    border: 1px solid transparent;
    transition: color 0.15s, border-color 0.15s, background 0.15s;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  .sort-btn:hover {
    color: var(--text);
    border-color: var(--border);
  }
  .sort-btn.active {
    color: var(--accent);
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .dir-arrow {
    display: inline-block;
    margin-left: 2px;
    font-size: 0.8em;
    font-weight: 700;
  }

  .sentinel { height: 1px; }

  .load-more-hint {
    text-align: center;
    padding: 16px 0;
    color: var(--text-muted);
    font-size: 0.875rem;
  }

  /* ── Skeleton grid ── */
  @keyframes sk-pulse { 0%,100%{opacity:.5} 50%{opacity:1} }
  .sk-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 16px;
  }
  .sk-card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px;
  }
  .sk-cover {
    width: 100%;
    padding-bottom: 100%;
    border-radius: 4px;
    background: var(--bg-hover);
    animation: sk-pulse 1.4s ease-in-out infinite;
  }
  .sk-line {
    border-radius: 4px;
    background: var(--bg-hover);
    animation: sk-pulse 1.4s ease-in-out infinite;
  }
  .sk-line--title { height: 13px; width: 80%; }
  .sk-line--sub   { height: 11px; width: 55%; }
</style>
