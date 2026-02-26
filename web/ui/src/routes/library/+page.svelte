<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { library as libApi } from '$lib/api/library';
  import AlbumGrid from '$lib/components/library/AlbumGrid.svelte';
  import AlphaScrollbar from '$lib/components/library/AlphaScrollbar.svelte';
  import type { Album } from '$lib/types';

  type SortMode = 'title' | 'artist' | 'year';

  const SORT_MODES: { mode: SortMode; label: string }[] = [
    { mode: 'title',  label: 'Title'  },
    { mode: 'artist', label: 'Artist' },
    { mode: 'year',   label: 'Year'   },
  ];

  const PAGE_SIZE = 500;

  let albums: Album[] = [];
  let totalCount = 0;
  let loading = true;
  let loadingMore = false;
  let nextOffset = 0;
  let sortBy: SortMode = 'title';
  let activeKey = '';
  let scrollEl: HTMLElement | null = null;
  let sentinel: HTMLElement;
  let observer: IntersectionObserver | null = null;

  // ── Grouping (no client-side sort — DB returns in the right order) ─────────

  function getSortKey(album: Album, mode: SortMode): string {
    switch (mode) {
      case 'title': {
        const first = album.title.replace(/^(the |a |an )\s*/i, '').charAt(0).toUpperCase();
        return /[A-Z]/.test(first) ? first : '#';
      }
      case 'artist': {
        const name = album.artist_name ?? '';
        const first = name.replace(/^(the |a |an )\s*/i, '').charAt(0).toUpperCase();
        return first && /[A-Z]/.test(first) ? first : '#';
      }
      case 'year':
        return album.release_year ? String(album.release_year) : '?';
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
    return [...map.keys()].sort((a, b) => {
      if (mode === 'year') {
        if (a === '?') return 1;
        if (b === '?') return -1;
        return Number(b) - Number(a); // newest first
      }
      if (a === '#') return -1; // # before A
      if (b === '#') return 1;
      return a.localeCompare(b);
    });
  }

  $: grouped = computeGrouped(albums, sortBy);
  $: keys    = computeKeys(grouped, sortBy);
  $: hasMore = albums.length < totalCount;

  // Reset scroll + active key whenever keys changes (sort changed or first load)
  $: {
    keys; // declare dependency
    activeKey = keys[0] ?? '';
    scrollEl?.scrollTo({ top: 0 });
  }

  // ── Scroll tracking ───────────────────────────────────────────────────────

  function updateActive() {
    if (!scrollEl) return;
    const sections = scrollEl.querySelectorAll('[data-scroll-key]');
    const containerTop = scrollEl.getBoundingClientRect().top;
    let current = keys[0] ?? '';
    for (const section of sections) {
      const top = section.getBoundingClientRect().top - containerTop;
      if (top <= 64) {
        current = section.getAttribute('data-scroll-key') ?? current;
      }
    }
    activeKey = current;
  }

  // ── Infinite scroll ───────────────────────────────────────────────────────

  async function loadNextPage() {
    if (loadingMore || !hasMore) return;
    loadingMore = true;
    try {
      const page = await libApi.albums(PAGE_SIZE, nextOffset, sortBy);
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
      (entries) => { if (entries[0].isIntersecting) loadNextPage(); },
      { root: scrollEl, rootMargin: '200px' }
    );
    observer.observe(sentinel);
  }

  // ── Sort change ───────────────────────────────────────────────────────────

  async function changeSortBy(mode: SortMode) {
    if (mode === sortBy) return;
    sortBy = mode;
    observer?.disconnect();
    observer = null;
    albums = [];
    nextOffset = 0;
    totalCount = 0;
    loading = true;
    try {
      const first = await libApi.albums(PAGE_SIZE, 0, sortBy);
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
    scrollEl = document.querySelector('main.content');
    if (scrollEl) scrollEl.addEventListener('scroll', updateActive, { passive: true });
    loadFirstPage();
  });

  onDestroy(() => {
    scrollEl?.removeEventListener('scroll', updateActive);
    observer?.disconnect();
  });

  async function loadFirstPage() {
    try {
      const first = await libApi.albums(PAGE_SIZE, 0, sortBy);
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
    <h2 class="title">
      Albums
      <span class="count">
        {#if !loading && totalCount > 0 && albums.length < totalCount}
          {albums.length} / {totalCount}
        {:else}
          {albums.length}
        {/if}
      </span>
    </h2>
    <div class="sort-controls">
      <span class="sort-label">Sort by</span>
      {#each SORT_MODES as { mode, label }}
        <button
          class="sort-btn"
          class:active={sortBy === mode}
          on:click={() => changeSortBy(mode)}
        >
          {label}
        </button>
      {/each}
    </div>
  </div>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else}
    <AlbumGrid {grouped} {keys} />
    <div bind:this={sentinel} class="sentinel"></div>
    {#if loadingMore}
      <p class="muted load-more-hint">Loading more…</p>
    {/if}
  {/if}
</div>

<AlphaScrollbar {keys} {activeKey} {scrollEl} />

<style>
  .page {
    padding-right: 40px; /* clear room for the alpha scrollbar */
    min-height: 100%;
  }

  .page-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 28px;
    flex-wrap: wrap;
    gap: 12px;
  }

  .title {
    font-size: 1.25rem;
    font-weight: 600;
  }

  .count {
    font-size: 0.875rem;
    font-weight: 400;
    color: var(--muted);
    margin-left: 6px;
    vertical-align: middle;
  }

  .sort-controls {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .sort-label {
    font-size: 0.6875rem;
    color: var(--muted);
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
    color: var(--text-2);
    border: 1px solid transparent;
    transition: color 0.15s, border-color 0.15s, background 0.15s;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .sort-btn:hover {
    color: var(--text);
    border-color: var(--border-2);
  }

  .sort-btn.active {
    color: var(--accent);
    border-color: var(--accent);
    background: var(--accent-dim);
  }

  .muted {
    color: var(--text-2);
    font-size: 0.875rem;
  }

  .sentinel {
    height: 1px;
  }

  .load-more-hint {
    text-align: center;
    padding: 16px 0;
  }
</style>
