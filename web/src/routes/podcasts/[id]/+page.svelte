<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/stores";
  import { podcasts as podcastApi } from "$lib/api/podcasts";
  import type { Podcast, PodcastEpisode, PodcastEpisodeProgress } from "$lib/types";
  import { getApiBase } from "$lib/api/base";
  import Spinner from "$lib/components/ui/Spinner.svelte";
  import {
    playEpisode,
    currentEpisode,
    podcastPlaybackState,
  } from "$lib/stores/player/podcastPlayer";

  let podcast: Podcast | null = null;
  let episodes: PodcastEpisode[] = [];
  let progress: Record<string, PodcastEpisodeProgress> = {};
  let loading = true;

  async function load() {
    loading = true;
    try {
      const id = $page.params.id ?? "";
      const [pRes, eRes] = await Promise.all([
        podcastApi.get(id),
        podcastApi.listEpisodes(id)
      ]);
      podcast = pRes.podcast;
      episodes = eRes.episodes ?? [];

      // Fetch progress for all episodes in parallel
      const progResults = await Promise.allSettled(
        episodes.map((ep) => podcastApi.getProgress(ep.id))
      );
      const map: Record<string, PodcastEpisodeProgress> = {};
      progResults.forEach((r, i) => {
        if (r.status === "fulfilled") map[episodes[i].id] = r.value.progress;
      });
      progress = map;
    } catch (err) {
      console.error("Failed to load podcast", err);
    } finally {
      loading = false;
    }
  }

  function fmtDate(date: string) {
    return new Date(date).toLocaleDateString(undefined, {
      year: "numeric", month: "short", day: "numeric",
    });
  }

  function fmtDuration(ms: number) {
    if (!ms) return "";
    const h = Math.floor(ms / 3600000);
    const m = Math.floor((ms % 3600000) / 60000);
    const s = Math.floor((ms % 60000) / 1000);
    if (h > 0) return `${h}h ${m}m`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
  }

  function progressPct(ep: PodcastEpisode): number {
    const p = progress[ep.id];
    if (!p || !ep.duration_ms) return 0;
    return Math.min(100, (p.position_ms / ep.duration_ms) * 100);
  }

  async function handlePlay(ep: PodcastEpisode) {
    if (!podcast) return;
    await playEpisode(ep, podcast);
  }

  async function handleTogglePlayed(ep: PodcastEpisode) {
    const p = progress[ep.id];
    const nowCompleted = !(p?.completed ?? false);
    await podcastApi.updateProgress(ep.id, p?.position_ms ?? 0, nowCompleted);
    progress = {
      ...progress,
      [ep.id]: { ...p, episode_id: ep.id, user_id: "", completed: nowCompleted, position_ms: p?.position_ms ?? 0, updated_at: new Date().toISOString() },
    };
  }

  async function handleDownload(ep: PodcastEpisode) {
    await podcastApi.download(ep.id);
    // Optimistically update the episode to show it's being downloaded
    episodes = episodes.map((e) =>
      e.id === ep.id ? { ...e, file_key: "downloading" } : e
    );
  }

  onMount(load);
</script>

<svelte:head><title>{podcast?.title ?? "Podcast"} – Orb</title></svelte:head>

<div class="page">
  {#if loading}
    <div class="loading">
      <Spinner size={32} />
    </div>
  {:else if podcast}
    <div class="header">
      <div class="cover-wrap">
        {#if podcast.cover_art_key}
          <img src="{getApiBase()}/covers/podcast/{podcast.id}" alt={podcast.title} class="cover" />
        {:else}
          <div class="cover placeholder">
            <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1" stroke-linecap="round" stroke-linejoin="round" style="opacity:0.3">
              <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
              <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
              <line x1="12" y1="19" x2="12" y2="23"/>
              <line x1="8" y1="23" x2="16" y2="23"/>
            </svg>
          </div>
        {/if}
      </div>
      <div class="info">
        <h1>{podcast.title}</h1>
        {#if podcast.author}
          <p class="author">{podcast.author}</p>
        {/if}
        {#if podcast.description}
          <p class="description">{podcast.description}</p>
        {/if}
        <p class="ep-count">{episodes.length} episode{episodes.length !== 1 ? "s" : ""}</p>
      </div>
    </div>

    <div class="episodes">
      <h2>Episodes</h2>
      {#each episodes as ep (ep.id)}
        {@const isPlaying = $currentEpisode?.id === ep.id && $podcastPlaybackState === "playing"}
        {@const isLoading = $currentEpisode?.id === ep.id && $podcastPlaybackState === "loading"}
        {@const isActive = $currentEpisode?.id === ep.id}
        {@const pct = progressPct(ep)}
        {@const played = progress[ep.id]?.completed ?? false}

        <div class="episode-row" class:active={isActive} class:played={played}>
          <div class="ep-info">
            <span class="ep-title" class:muted={played}>{ep.title}</span>
            <span class="ep-meta">
              {fmtDate(ep.pub_date)}
              {#if ep.duration_ms}
                <span>·</span>
                <span>{fmtDuration(ep.duration_ms)}</span>
              {/if}
              {#if ep.file_key}
                <span>·</span>
                <span class="downloaded-badge">Downloaded</span>
              {/if}
            </span>
            {#if pct > 0 && !played}
              <div class="ep-progress-bar">
                <div class="ep-progress-fill" style="width: {pct}%"></div>
              </div>
            {/if}
          </div>
          <div class="actions">
            <button
              class="play-btn"
              class:playing={isPlaying}
              on:click={() => handlePlay(ep)}
              aria-label={isPlaying ? "Pause" : "Play episode"}
            >
              {#if isLoading}
                <div class="spin-sm"></div>
              {:else if isPlaying}
                <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                  <rect x="6" y="4" width="4" height="16" rx="1" />
                  <rect x="14" y="4" width="4" height="16" rx="1" />
                </svg>
              {:else}
                <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                  <polygon points="5,3 19,12 5,21" />
                </svg>
              {/if}
            </button>

            <button
              class="icon-btn"
              title={played ? "Mark unplayed" : "Mark played"}
              on:click={() => handleTogglePlayed(ep)}
              aria-label={played ? "Mark as unplayed" : "Mark as played"}
            >
              {#if played}
                <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M9 11l3 3L22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/>
                </svg>
              {:else}
                <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <rect x="3" y="3" width="18" height="18" rx="2"/>
                </svg>
              {/if}
            </button>

            {#if !ep.file_key}
              <button
                class="icon-btn"
                title="Download episode"
                on:click={() => handleDownload(ep)}
                aria-label="Download episode"
              >
                <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                  <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                  <polyline points="7 10 12 15 17 10"/>
                  <line x1="12" y1="15" x2="12" y2="3"/>
                </svg>
              </button>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .page { padding: 24px; }
  .loading { display: flex; justify-content: center; padding: 64px; }

  .header { display: flex; gap: 32px; margin-bottom: 48px; }
  .cover-wrap { width: 200px; height: 200px; flex-shrink: 0; border-radius: 12px; overflow: hidden; background: var(--bg-elevated); }
  .cover { width: 100%; height: 100%; object-fit: cover; }
  .placeholder { display: flex; align-items: center; justify-content: center; }
  .info { display: flex; flex-direction: column; gap: 8px; }
  .info h1 { margin: 0; font-size: 2rem; }
  .author { color: var(--accent); font-weight: 600; margin: 0; }
  .description { color: var(--text-muted); font-size: 0.9rem; line-height: 1.5; max-width: 700px; margin: 0; }
  .ep-count { color: var(--text-muted); font-size: 0.85rem; margin: 0; }

  .episodes h2 { font-size: 1.1rem; font-weight: 600; margin: 0 0 8px 0; }

  .episode-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 12px;
    border-bottom: 1px solid var(--border);
    border-radius: 6px;
    gap: 12px;
    transition: background 0.1s;
  }
  .episode-row:hover { background: var(--bg-elevated); }
  .episode-row.active { background: color-mix(in srgb, var(--accent) 8%, transparent); }

  .ep-info { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 0; }
  .ep-title {
    font-weight: 500;
    font-size: 0.9rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .ep-title.muted { color: var(--text-muted); }
  .ep-meta {
    font-size: 0.78rem;
    color: var(--text-muted);
    display: flex;
    gap: 6px;
    align-items: center;
  }
  .downloaded-badge {
    color: var(--accent);
    font-size: 0.72rem;
  }

  .ep-progress-bar {
    height: 2px;
    background: var(--border);
    border-radius: 1px;
    margin-top: 2px;
    max-width: 200px;
  }
  .ep-progress-fill {
    height: 100%;
    background: var(--accent);
    border-radius: 1px;
  }

  .actions { display: flex; gap: 6px; align-items: center; flex-shrink: 0; }

  .play-btn {
    background: var(--accent);
    color: white;
    border: none;
    width: 32px;
    height: 32px;
    border-radius: 50%;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: opacity 0.15s;
  }
  .play-btn:hover { opacity: 0.85; }

  .icon-btn {
    background: transparent;
    color: var(--text-muted);
    border: none;
    width: 28px;
    height: 28px;
    border-radius: 4px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: color 0.15s;
  }
  .icon-btn:hover { color: var(--text); }

  .spin-sm {
    width: 12px;
    height: 12px;
    border: 2px solid rgba(255,255,255,0.4);
    border-top-color: white;
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
