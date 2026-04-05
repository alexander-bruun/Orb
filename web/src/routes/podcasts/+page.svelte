<script lang="ts">
  import { onMount } from "svelte";
  import { podcasts as podcastApi } from "$lib/api/podcasts";
  import type { Podcast } from "$lib/types";
  import { getApiBase } from "$lib/api/base";
  import { goto } from "$app/navigation";

  let subscriptions: Podcast[] = [];
  let allPodcasts: Podcast[] = [];
  let loading = true;
  let loadingMore = false;
  let rssUrl = "";
  let subscribing = false;
  let offset = 0;
  const limit = 50;
  let hasMore = true;

  async function loadPodcasts() {
    loading = true;
    offset = 0;
    try {
      const [subRes, allRes] = await Promise.all([
        podcastApi.getSubscriptions(1000, 0), // Get all subs for the checkmarks
        podcastApi.list(limit, offset),
      ]);
      subscriptions = subRes.podcasts ?? [];
      allPodcasts = allRes.podcasts ?? [];
      hasMore = allPodcasts.length === limit;
    } catch (err) {
      console.error("Failed to load podcasts", err);
    } finally {
      loading = false;
    }
  }

  async function loadMore() {
    if (loadingMore || !hasMore) return;
    loadingMore = true;
    try {
      offset += limit;
      const res = await podcastApi.list(limit, offset);
      const newPodcasts = res.podcasts ?? [];
      allPodcasts = [...allPodcasts, ...newPodcasts];
      hasMore = newPodcasts.length === limit;
    } catch (err) {
      console.error("Failed to load more podcasts", err);
    } finally {
      loadingMore = false;
    }
  }

  async function handleSubscribe() {
    if (!rssUrl) return;
    subscribing = true;
    try {
      await podcastApi.subscribe(rssUrl);
      rssUrl = "";
      await loadPodcasts();
    } catch (err) {
      alert("Failed to subscribe: " + err);
    } finally {
      subscribing = false;
    }
  }

  onMount(loadPodcasts);
</script>

<svelte:head><title>Podcasts – Orb</title></svelte:head>

<div class="page">
  <div class="page-header">
    <div class="title-row">
      <h1 class="page-title">Podcasts</h1>
      {#if !loading && allPodcasts.length > 0}
        <span class="count">{allPodcasts.length}{hasMore ? "+" : ""}</span>
      {/if}
    </div>
    <form class="subscribe-bar" on:submit|preventDefault={handleSubscribe}>
      <svg class="rss-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M4 11a9 9 0 0 1 9 9"/><path d="M4 4a16 16 0 0 1 16 16"/><circle cx="5" cy="19" r="1" fill="currentColor" stroke="none"/>
      </svg>
      <input
        type="url"
        class="rss-input"
        placeholder="Paste RSS feed URL…"
        bind:value={rssUrl}
        disabled={subscribing}
      />
      <button class="rss-btn" type="submit" disabled={subscribing || !rssUrl}>
        {#if subscribing}
          <svg class="spin-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><circle cx="12" cy="12" r="9" stroke-dasharray="44 13"/></svg>
        {:else}
          Subscribe
        {/if}
      </button>
    </form>
  </div>

  {#if loading}
    <div class="grid">
      {#each { length: 12 } as _}
        <div class="card-skeleton">
          <div class="skeleton-cover"></div>
          <div class="sk-line sk-title"></div>
          <div class="sk-line sk-author"></div>
        </div>
      {/each}
    </div>
  {:else if allPodcasts.length === 0}
    <div class="empty">
      <svg width="52" height="52" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
        <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
        <line x1="12" y1="19" x2="12" y2="23"/>
        <line x1="8" y1="23" x2="16" y2="23"/>
      </svg>
      <p>No podcasts yet.</p>
      <p class="empty-sub">Paste an RSS feed URL above to subscribe.</p>
    </div>
  {:else}
    <div class="grid">
      {#each allPodcasts as podcast (podcast.id)}
        {@const isSubscribed = subscriptions.some((s) => s.id === podcast.id)}
        <button type="button" class="podcast-card" on:click={() => goto(`/podcasts/${podcast.id}`)}>
          <div class="cover-wrap">
            {#if podcast.cover_art_key}
              <img src="{getApiBase()}/covers/podcast/{podcast.id}" alt={podcast.title} class="cover" loading="lazy" />
            {:else}
              <div class="cover placeholder">
                <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
                  <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
                  <line x1="12" y1="19" x2="12" y2="23"/>
                  <line x1="8" y1="23" x2="16" y2="23"/>
                </svg>
              </div>
            {/if}
            {#if isSubscribed}
              <div class="sub-badge" title="Subscribed">
                <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="20 6 9 17 4 12"/></svg>
              </div>
            {/if}
            <div class="card-overlay" aria-hidden="true"></div>
          </div>
          <div class="info">
            <span class="title" title={podcast.title}>{podcast.title}</span>
            {#if podcast.author}<span class="author">{podcast.author}</span>{/if}
          </div>
        </button>
      {/each}
    </div>

    {#if hasMore}
      <div class="load-more">
        <button class="load-more-btn" on:click={loadMore} disabled={loadingMore}>
          {loadingMore ? "Loading…" : "Load more"}
        </button>
      </div>
    {/if}
  {/if}
</div>

<style>
  .page {
    padding: 24px;
  }
  .page-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 28px;
    flex-wrap: wrap;
  }
  .title-row {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }
  .page-title {
    font-size: 1.25rem;
    font-weight: 700;
    margin: 0;
  }
  .count {
    font-size: 0.8rem;
    color: var(--text-muted);
  }

  /* ── Subscribe bar ── */
  .subscribe-bar {
    display: flex;
    align-items: center;
    gap: 0;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 4px 4px 4px 14px;
    transition: border-color 0.15s, box-shadow 0.15s;
  }
  .subscribe-bar:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 15%, transparent);
  }
  .rss-icon {
    color: var(--text-muted);
    flex-shrink: 0;
    margin-right: 8px;
  }
  .rss-input {
    background: none;
    border: none;
    outline: none;
    color: var(--text);
    font: inherit;
    font-size: 0.875rem;
    width: 260px;
    min-width: 0;
  }
  .rss-input::placeholder { color: var(--text-muted); }
  .rss-input:disabled { opacity: 0.6; }
  .rss-btn {
    background: var(--accent);
    color: #fff;
    border: none;
    border-radius: 999px;
    padding: 7px 16px;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    white-space: nowrap;
    display: flex;
    align-items: center;
    gap: 6px;
    transition: opacity 0.15s;
    flex-shrink: 0;
  }
  .rss-btn:hover:not(:disabled) { opacity: 0.88; }
  .rss-btn:disabled { opacity: 0.5; cursor: not-allowed; }
  @keyframes spin-anim { to { transform: rotate(360deg); } }
  .spin-icon { animation: spin-anim 0.8s linear infinite; }

  /* ── Grid ── */
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 20px 16px;
  }

  /* ── Podcast card ── */
  .podcast-card {
    appearance: none;
    border: none;
    background: transparent;
    text-align: left;
    width: 100%;
    cursor: pointer;
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 0;
    transition: transform 0.18s;
  }
  .podcast-card:hover { transform: translateY(-3px); }

  .cover-wrap {
    width: 100%;
    aspect-ratio: 1;
    border-radius: 10px;
    overflow: hidden;
    background: var(--bg-elevated);
    position: relative;
    box-shadow: 0 2px 8px rgba(0,0,0,0.2);
    transition: box-shadow 0.18s;
  }
  .podcast-card:hover .cover-wrap {
    box-shadow: 0 8px 24px rgba(0,0,0,0.35);
  }
  .cover {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.25s;
  }
  .podcast-card:hover .cover { transform: scale(1.03); }

  .placeholder {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    opacity: 0.45;
  }

  .card-overlay {
    position: absolute;
    inset: 0;
    background: linear-gradient(to top, rgba(0,0,0,0.35) 0%, transparent 50%);
    opacity: 0;
    transition: opacity 0.2s;
    pointer-events: none;
  }
  .podcast-card:hover .card-overlay { opacity: 1; }

  .sub-badge {
    position: absolute;
    top: 8px;
    right: 8px;
    background: var(--accent);
    color: #fff;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    box-shadow: 0 1px 4px rgba(0,0,0,0.3);
  }

  .info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 0 2px;
  }
  .title {
    font-weight: 600;
    font-size: 0.875rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
  }
  .author {
    font-size: 0.78rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* ── Skeleton ── */
  @keyframes sk-pulse { 0%,100%{opacity:.5} 50%{opacity:1} }
  .card-skeleton { display: flex; flex-direction: column; gap: 8px; }
  .skeleton-cover {
    width: 100%;
    aspect-ratio: 1;
    border-radius: 10px;
    background: var(--bg-elevated);
    animation: sk-pulse 1.4s ease-in-out infinite;
  }
  .sk-line {
    border-radius: 4px;
    background: var(--bg-elevated);
    animation: sk-pulse 1.4s ease-in-out infinite;
  }
  .sk-title  { height: 13px; width: 75%; }
  .sk-author { height: 11px; width: 50%; }

  /* ── Load more ── */
  .load-more { display: flex; justify-content: center; padding: 32px 0; }
  .load-more-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--text-muted);
    padding: 8px 24px;
    border-radius: 20px;
    cursor: pointer;
    font-size: 0.875rem;
    transition: color 0.15s, border-color 0.15s, background 0.15s;
  }
  .load-more-btn:hover:not(:disabled) { color: var(--text); border-color: var(--text-muted); background: var(--bg-elevated); }
  .load-more-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  /* ── Empty state ── */
  .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 80px 16px;
    color: var(--text-muted);
    text-align: center;
  }
  .empty svg { opacity: 0.3; }
  .empty p { margin: 0; font-size: 1rem; font-weight: 500; color: var(--text); }
  .empty-sub { font-size: 0.875rem; font-weight: 400; color: var(--text-muted) !important; }

  @media (max-width: 640px) {
    .page-header { flex-direction: column; align-items: stretch; margin-bottom: 20px; }
    .subscribe-bar { width: 100%; }
    .rss-input { width: 100%; }
    .grid { grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: 14px 10px; }
  }
</style>
