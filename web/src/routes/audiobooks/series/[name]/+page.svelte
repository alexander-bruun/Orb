<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/stores";
  import { goto } from "$app/navigation";
  import { audiobooks as abApi } from "$lib/api/audiobooks";
  import type { Audiobook } from "$lib/types";
  import { getApiBase } from "$lib/api/base";
  import { playAudiobook } from "$lib/stores/player/audiobookPlayer";

  let books: Audiobook[] = [];
  let loading = true;
  let error = "";

  $: seriesName = $page.params.name ?? "";

  function fmtDuration(ms: number): string {
    const h = Math.floor(ms / 3_600_000);
    const m = Math.floor((ms % 3_600_000) / 60_000);
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
  }

  onMount(async () => {
    try {
      const res = await abApi.listBySeries(seriesName);
      books = res.audiobooks ?? [];
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : "Failed to load series";
    } finally {
      loading = false;
    }
  });
</script>

<svelte:head><title>{seriesName} – Orb</title></svelte:head>

<div class="page">
  <div class="page-header">
    <!-- svelte-ignore a11y-click-events-have-key-events -->
    <!-- svelte-ignore a11y-no-static-element-interactions -->
    <span class="back" on:click={() => goto("/audiobooks")}>← Audiobooks</span>
    <h1 class="page-title">{seriesName}</h1>
    {#if !loading}
      <span class="count">{books.length} book{books.length === 1 ? "" : "s"}</span>
    {/if}
  </div>

  {#if loading}
    <div class="grid">
      {#each { length: 6 } as _}
        <div class="card-skeleton">
          <div class="skeleton-cover"></div>
        </div>
      {/each}
    </div>
  {:else if error}
    <p class="error">{error}</p>
  {:else if books.length === 0}
    <div class="empty">
      <p>No books found in this series.</p>
    </div>
  {:else}
    <div class="grid">
      {#each books as book (book.id)}
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div class="book-card" on:click={() => goto(`/audiobooks/${book.id}`)}>
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
                <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"/>
                  <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"/>
                </svg>
              </div>
            {/if}
            <button
              class="play-btn"
              aria-label="Play {book.title}"
              on:click|stopPropagation={() => playAudiobook(book)}
            >
              <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
                <path d="M4 2.5l10 5.5-10 5.5V2.5z"/>
              </svg>
            </button>
          </div>
          <div class="info">
            {#if book.series_index != null}
              <span class="idx">Book {book.series_index}</span>
            {/if}
            <span class="title" title={book.title}>{book.title}</span>
            {#if book.author_name}
              <span class="author">{book.author_name}</span>
            {/if}
            {#if book.duration_ms}
              <span class="duration">{fmtDuration(book.duration_ms)}</span>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .page { padding-top: 4px; }

  .page-header {
    display: flex;
    align-items: baseline;
    gap: 12px;
    margin-bottom: 24px;
    flex-wrap: wrap;
  }

  .back {
    font-size: 0.8rem;
    color: var(--text-muted);
    cursor: pointer;
  }
  .back:hover { color: var(--text); }

  .page-title { font-size: 1.5rem; font-weight: 700; margin: 0; }
  .count { font-size: 0.8rem; color: var(--text-muted); }
  .error { color: var(--text-muted); text-align: center; padding: 48px; }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 20px 16px;
  }

  .card-skeleton {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .skeleton-cover {
    width: 100%;
    padding-bottom: 150%;
    border-radius: 8px;
    background: linear-gradient(
      90deg,
      var(--bg-3, #2a2a2a) 25%,
      var(--bg-2, #333) 50%,
      var(--bg-3, #2a2a2a) 75%
    );
    background-size: 200% 100%;
    animation: shimmer 1.4s ease-in-out infinite;
  }
  @keyframes shimmer {
    0%   { background-position: 200% 0; }
    100% { background-position: -200% 0; }
  }

  .book-card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    cursor: pointer;
  }

  .cover-wrap {
    position: relative;
    width: 100%;
    padding-bottom: 150%;
    border-radius: 8px;
    overflow: hidden;
    background: var(--bg-elevated);
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
  .book-card:hover .cover { transform: scale(1.03); }

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
    border: none;
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    opacity: 0;
    transform: translateY(4px);
    transition: opacity 0.2s, transform 0.2s;
    box-shadow: 0 2px 8px rgba(0,0,0,0.4);
  }
  .book-card:hover .play-btn { opacity: 1; transform: translateY(0); }
  .play-btn:hover { filter: brightness(1.1); }

  .info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .idx {
    font-size: 0.7rem;
    color: var(--accent);
    font-weight: 600;
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

  .duration {
    font-size: 0.72rem;
    color: var(--text-muted);
  }

  .empty {
    display: flex;
    justify-content: center;
    padding: 72px 16px;
    color: var(--text-muted);
  }

  @media (max-width: 640px) {
    .grid { grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: 16px 12px; }
  }
</style>
