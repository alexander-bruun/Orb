<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { audiobooks as abApi } from "$lib/api/audiobooks";
  import type { Audiobook } from "$lib/types";
  import { getApiBase } from "$lib/api/base";
  import { playAudiobook } from "$lib/stores/player/audiobookPlayer";
  import Skeleton from "$lib/components/ui/Skeleton.svelte";
  import AlphaScrollbar from "$lib/components/library/AlphaScrollbar.svelte";
  import { goto } from "$app/navigation";
  import Spinner from "$lib/components/ui/Spinner.svelte";

  type SortMode = "title" | "author" | "year" | "series";

  const SORT_MODES: { mode: SortMode; label: string }[] = [
    { mode: "title", label: "Title" },
    { mode: "author", label: "Author" },
    { mode: "year", label: "Year" },
    { mode: "series", label: "Series" },
  ];

  let books: Audiobook[] = [];
  let loading = true;
  let loadingMore = false;
  let hasMore = true;
  let sortBy: SortMode = "title";
  let sortDir: "asc" | "desc" = "asc";
  let activeKey = "";
  let scrollEl: HTMLElement | null = null;
  let sentinel: HTMLElement;
  let observer: IntersectionObserver | null = null;
  let lastSortBy: SortMode = sortBy;
  let isRestoring = false;
  const PAGE = 48;

  const SORT_BY_KEY = "audiobooks_sort_by";
  const SORT_DIR_KEY = "audiobooks_sort_dir";

  if (typeof localStorage !== "undefined") {
    const savedSortBy = localStorage.getItem(SORT_BY_KEY);
    if (
      savedSortBy &&
      ["title", "author", "year", "series"].includes(savedSortBy)
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
      books,
      hasMore,
      sortBy,
      sortDir,
      activeKey,
      lastSortBy,
    }),
    restore: (value) => {
      books = value.books;
      hasMore = value.hasMore;
      sortBy = value.sortBy;
      sortDir = value.sortDir;
      activeKey = value.activeKey;
      lastSortBy = value.lastSortBy;
      isRestoring = true;
      loading = false;
    },
  };

  function fmtDuration(ms: number): string {
    const h = Math.floor(ms / 3_600_000);
    const m = Math.floor((ms % 3_600_000) / 60_000);
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
  }

  function getSortKey(book: Audiobook, mode: SortMode): string {
    switch (mode) {
      case "title": {
        const first = book.title
          .replace(/^(the |a |an )\s*/i, "")
          .charAt(0)
          .toUpperCase();
        return /[A-Z]/.test(first) ? first : "#";
      }
      case "author": {
        const name = book.author_name ?? "";
        const first = name
          .replace(/^(the |a |an )\s*/i, "")
          .charAt(0)
          .toUpperCase();
        return first && /[A-Z]/.test(first) ? first : "#";
      }
      case "year":
        return book.published_year ? String(book.published_year) : "?";
      case "series":
        return book.series ?? "Standalone";
    }
  }

  function computeGrouped(
    list: Audiobook[],
    mode: SortMode,
  ): Map<string, Audiobook[]> {
    const map = new Map<string, Audiobook[]>();
    for (const book of list) {
      const key = getSortKey(book, mode);
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(book);
    }
    return map;
  }

  function computeKeys(
    map: Map<string, Audiobook[]>,
    mode: SortMode,
  ): string[] {
    // The backend now returns the items in the correct sort order,
    // so we can just return the keys in their insertion order from the map.
    return [...map.keys()];
  }

  $: grouped = computeGrouped(books, sortBy);
  $: keys = computeKeys(grouped, sortBy);

  $: if (sortBy !== lastSortBy) {
    lastSortBy = sortBy;
    if (!isRestoring) {
      activeKey = keys[0] ?? "";
      scrollEl?.scrollTo({ top: 0 });
    }
  }

  $: if (!activeKey && keys.length > 0) {
    activeKey = keys[0];
  }

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

  async function loadMore() {
    if (loadingMore || !hasMore) return;
    loadingMore = true;
    try {
      const res = await abApi.list(PAGE, books.length, sortBy, sortDir);
      const fetched = res.audiobooks ?? [];
      books = [...books, ...fetched];
      if (fetched.length < PAGE) {
        hasMore = false;
        observer?.disconnect();
        observer = null;
      }
    } catch {
      // ignore
    } finally {
      loadingMore = false;
    }
  }

  function setupObserver() {
    observer?.disconnect();
    if (!sentinel || !hasMore) return;
    observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting) loadMore();
      },
      { root: scrollEl, rootMargin: "200px" },
    );
    observer.observe(sentinel);
  }

  async function changeSort(mode: SortMode) {
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

    loading = true;
    books = [];
    hasMore = true;
    observer?.disconnect();
    observer = null;
    try {
      const res = await abApi.list(PAGE, 0, sortBy, sortDir);
      books = res.audiobooks ?? [];
      if (books.length < PAGE) hasMore = false;
    } catch {
      // ignore
    } finally {
      loading = false;
      setTimeout(setupObserver, 0);
    }
  }

  onMount(async () => {
    scrollEl = document.querySelector("main.content");
    if (scrollEl)
      scrollEl.addEventListener("scroll", updateActive, { passive: true });

    if (isRestoring && books.length > 0) {
      loading = false;
      setTimeout(setupObserver, 0);
      // Let the reactive block skip one tick
      setTimeout(() => {
        isRestoring = false;
      }, 0);
      return;
    }

    try {
      const res = await abApi.list(PAGE, 0, sortBy, sortDir);
      books = res.audiobooks ?? [];
      if (books.length < PAGE) hasMore = false;
    } catch {
      // ignore
    } finally {
      loading = false;
      setTimeout(setupObserver, 0);
    }
  });

  onDestroy(() => {
    scrollEl?.removeEventListener("scroll", updateActive);
    observer?.disconnect();
  });
</script>

<svelte:head><title>Audiobooks – Orb</title></svelte:head>

<div class="page">
  <div class="page-header">
    <div class="title-row">
      <h1 class="page-title">Audiobooks</h1>
      {#if !loading}
        <span class="count"
          >{books.length}{hasMore ? "+" : ""} book{books.length === 1
            ? ""
            : "s"}</span
        >
      {/if}
    </div>
    <div class="sort-controls">
      <span class="sort-label">Sort by</span>
      {#each SORT_MODES as { mode, label }}
        <button
          class="sort-btn"
          class:active={sortBy === mode}
          on:click={() => changeSort(mode)}
        >
          {label}
          {#if sortBy === mode}
            <span class="dir-icon">{sortDir === "asc" ? "↑" : "↓"}</span>
          {/if}
        </button>
      {/each}
    </div>
  </div>

  {#if loading}
    <div class="grid">
      {#each { length: 12 } as _}
        <div class="card-skeleton">
          <div class="skeleton-cover"></div>
          <Skeleton width="70%" height="0.85rem" radius="4px" />
          <Skeleton width="50%" height="0.75rem" radius="4px" />
        </div>
      {/each}
    </div>
  {:else if books.length === 0}
    <div class="empty">
      <svg
        width="48"
        height="48"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.5"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
        <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
      </svg>
      <p>No audiobooks yet.</p>
      <p class="muted">Set <code>AUDIOBOOK_DIRS</code> and trigger a scan.</p>
    </div>
  {:else}
    {#each keys as key (key)}
      <section class="group" data-scroll-key={key}>
        <h2 class="group-label">{key}</h2>
        <div class="grid">
          {#each grouped.get(key) ?? [] as book (book.id)}
            <button
              class="book-card"
              aria-label={`Open ${book.title}`}
              on:click={() => goto(`/audiobooks/${book.id}`)}
            >
              <div class="cover-wrap">
                {#if book.cover_art_key}
                  <img
                    src="{getApiBase()}/covers/audiobook/{book.id}"
                    alt={book.title}
                    class="cover"
                    loading="lazy"
                  />
                {:else}
                  <div class="cover placeholder">
                    <svg
                      width="40"
                      height="40"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="1.5"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      aria-hidden="true"
                    >
                      <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
                      <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
                    </svg>
                  </div>
                {/if}
                <div
                  class="play-btn"
                  role="button"
                  tabindex="-1"
                  aria-label="Play {book.title}"
                  on:click|stopPropagation={() => playAudiobook(book)}
                  on:keydown|stopPropagation
                >
                  <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
                    <path d="M4 2.5l10 5.5-10 5.5V2.5z" />
                  </svg>
                </div>
              </div>
              <div class="info">
                <span class="title" title={book.title}>{book.title}</span>
                {#if book.author_name}
                  <span class="author" title={book.author_name}
                    >{book.author_name}</span
                  >
                {/if}
                <div class="meta-row">
                  {#if book.series}
                    <a
                      class="series"
                      href="/audiobooks/series/{encodeURIComponent(book.series)}"
                      title="View series: {book.series}"
                      on:click|stopPropagation
                    >
                      {book.series}{book.series_index != null ? ` #${book.series_index}` : ""}
                    </a>
                  {/if}
                  {#if book.duration_ms}
                    <span class="duration">{fmtDuration(book.duration_ms)}</span
                    >
                  {/if}
                </div>
              </div>
            </button>
          {/each}
        </div>
      </section>
    {/each}

    <div bind:this={sentinel} class="sentinel"></div>
    {#if loadingMore}
      <div class="load-more">
        <Spinner size={18} />
      </div>
    {/if}
  {/if}
</div>

<AlphaScrollbar {keys} {activeKey} {scrollEl} />

<style>
  .page {
    padding-top: 4px;
    padding-right: 40px;
    min-height: 100%;
  }

  .sentinel {
    height: 1px;
    width: 100%;
  }

  .page-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 12px;
    margin-bottom: 24px;
  }
  .title-row {
    display: flex;
    align-items: baseline;
    gap: 12px;
  }
  .page-title {
    font-size: 1.5rem;
    font-weight: 700;
    margin: 0;
  }
  .count {
    font-size: 0.8rem;
    color: var(--text-muted);
  }

  .sort-controls {
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .sort-label {
    font-size: 0.6875rem;
    color: var(--text-muted);
    margin-right: 6px;
    text-transform: uppercase;
    letter-spacing: 0.08em;
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
    transition:
      color 0.15s,
      border-color 0.15s,
      background 0.15s;
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
    background: var(--accent-dim);
  }

  .dir-icon {
    display: inline-block;
    margin-left: 2px;
    font-size: 0.8em;
    font-weight: 700;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 20px 16px;
  }

  .group {
    display: flex;
    flex-direction: column;
    gap: 12px;
    margin-bottom: 20px;
  }
  .group-label {
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    margin: 0;
  }

  /* skeleton cards */
  @keyframes sk-pulse { 0%,100%{opacity:.5} 50%{opacity:1} }
  .card-skeleton {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .skeleton-cover {
    width: 100%;
    padding-bottom: 150%;
    border-radius: 10px;
    background: var(--bg-elevated);
    animation: sk-pulse 1.4s ease-in-out infinite;
  }

  /* real book cards */
  .book-card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    cursor: pointer;
    background: none;
    border: none;
    padding: 0;
    text-align: left;
    transition: transform 0.18s, box-shadow 0.18s;
  }
  .book-card:hover {
    transform: translateY(-3px);
  }

  .cover-wrap {
    position: relative;
    width: 100%;
    padding-bottom: 150%;
    border-radius: 10px;
    overflow: hidden;
    background: var(--bg-elevated);
    box-shadow: 0 2px 8px rgba(0,0,0,0.25);
    transition: box-shadow 0.18s;
  }
  .book-card:hover .cover-wrap {
    box-shadow: 0 8px 24px rgba(0,0,0,0.4);
  }

  .cover {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.25s;
  }
  .book-card:hover .cover {
    transform: scale(1.03);
  }

  .placeholder {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    opacity: 0.4;
  }

  .play-btn {
    position: absolute;
    bottom: 8px;
    right: 8px;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    background: var(--accent);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    opacity: 0;
    transform: translateY(4px);
    transition: opacity 0.2s, transform 0.2s, filter 0.15s;
    box-shadow: 0 2px 8px rgba(0,0,0,0.4);
  }
  .book-card:hover .play-btn {
    opacity: 1;
    transform: translateY(0);
  }
  .play-btn:hover { filter: brightness(1.12); }

  .info {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
    padding: 0 2px;
  }

  .title {
    font-size: 0.875rem;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--text);
    line-height: 1.3;
  }

  .author {
    font-size: 0.78rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .meta-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .series {
    font-size: 0.72rem;
    color: var(--accent);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 120px;
    text-decoration: none;
  }
  .series:hover { text-decoration: underline; }

  .duration {
    font-size: 0.72rem;
    color: var(--text-muted);
    margin-left: auto;
  }

  /* empty state */
  .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 72px 16px;
    color: var(--text-muted);
    text-align: center;
  }
  .empty svg {
    opacity: 0.35;
  }
  .empty p {
    margin: 0;
    font-size: 0.95rem;
  }
  .empty code {
    font-size: 0.85rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 1px 5px;
  }
  .muted {
    color: var(--text-muted);
  }

  /* load more */
  .load-more {
    display: flex;
    justify-content: center;
    padding: 32px 0;
  }

  @media (max-width: 640px) {
    .page {
      padding-right: 36px;
    }
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
      gap: 16px 12px;
    }
  }
</style>
