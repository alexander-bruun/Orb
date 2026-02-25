<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { library as libApi } from '$lib/api/library';
  import AlbumGrid from '$lib/components/library/AlbumGrid.svelte';
  import AlphaScrollbar from '$lib/components/library/AlphaScrollbar.svelte';
  import type { Album } from '$lib/types';

  type SortMode = 'title' | 'artist' | 'year' | 'label';

  const SORT_MODES: { mode: SortMode; label: string }[] = [
    { mode: 'title',  label: 'Title'  },
    { mode: 'artist', label: 'Artist' },
    { mode: 'year',   label: 'Year'   },
    { mode: 'label',  label: 'Label'  },
  ];

  let albums: Album[] = [];
  let loading = true;
  let sortBy: SortMode = 'title';
  let activeKey = '';
  let scrollEl: HTMLElement | null = null;

  // ── Grouping helpers ──────────────────────────────────────────────────────

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
      case 'label': {
        const first = (album.label ?? '').charAt(0).toUpperCase();
        return /[A-Z]/.test(first) ? first : '#';
      }
    }
  }

  function getSortValue(album: Album, mode: SortMode): string {
    switch (mode) {
      case 'title':
        return album.title.replace(/^(the |a |an )\s*/i, '').toLowerCase();
      case 'artist':
        return (album.artist_name ?? '').toLowerCase() + '\x00' + album.title.toLowerCase();
      case 'year': {
        // Invert year so that sort ascending gives newest first
        const y = album.release_year ?? 0;
        return String(9999 - y).padStart(4, '0') + '\x00' + album.title.toLowerCase();
      }
      case 'label':
        return (album.label ?? '').toLowerCase() + '\x00' + album.title.toLowerCase();
    }
  }

  function computeGrouped(list: Album[], mode: SortMode): Map<string, Album[]> {
    const sorted = [...list].sort((a, b) =>
      getSortValue(a, mode).localeCompare(getSortValue(b, mode))
    );
    const map = new Map<string, Album[]>();
    for (const album of sorted) {
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

  // Reset scroll + active key whenever keys changes (albums loaded or sort changed)
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

  onMount(() => {
    scrollEl = document.querySelector('main.content');
    if (scrollEl) scrollEl.addEventListener('scroll', updateActive, { passive: true });
    loadAlbums();
  });

  onDestroy(() => {
    scrollEl?.removeEventListener('scroll', updateActive);
  });

  async function loadAlbums() {
    try {
      albums = await libApi.albums(100);
    } finally {
      loading = false;
    }
  }
</script>

<div class="page">
  <div class="page-header">
    <h2 class="title">Albums</h2>
    <div class="sort-controls">
      <span class="sort-label">Sort by</span>
      {#each SORT_MODES as { mode, label }}
        <button
          class="sort-btn"
          class:active={sortBy === mode}
          on:click={() => (sortBy = mode)}
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
  {/if}
</div>

<AlphaScrollbar {keys} {activeKey} {scrollEl} />

<style>
  .page {
    padding-right: 32px; /* clear room for the alpha scrollbar */
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
</style>
