<script lang="ts">
  import type { Track } from '$lib/types';
  import { playTrack, currentTrack, playbackState } from '$lib/stores/player';
  import { openContextMenu } from '$lib/stores/ui/contextMenu';
  import { downloads, deleteDownload } from '$lib/stores/offline/downloads';
  import { onMount } from 'svelte';
  import { getArtistName, preloadArtists } from '$lib/stores/library/artists';
  import { favorites } from '$lib/stores/library/favorites';
  import StarRating from '$lib/components/ui/StarRating.svelte';

  export let track: Track;
  export let trackList: Track[] = [];
  export let index: number = 0;
  export let showCover: boolean = false;
  export let useRankIndex: boolean = false;

  import { getApiBase } from '$lib/api/base';

  $: isPlaying = $currentTrack?.id === track.id && $playbackState === 'playing';
  $: dlStatus = $downloads.get(track.id)?.status;

  interface ArtistRef { id?: string; name: string; }

  let mainArtist: ArtistRef | null = null;
  let featuredArtists: ArtistRef[] = [];

  onMount(async () => {
    // Resolve main artist
    if (track.artist) {
      mainArtist = { id: track.artist.id, name: track.artist.name };
    } else if (track.artist_name) {
      mainArtist = { id: track.artist_id, name: track.artist_name };
    } else if (track.artist_id) {
      const name = await getArtistName(track.artist_id);
      mainArtist = { id: track.artist_id, name };
    }

    // Resolve featured artists — use full objects when available, fall back to IDs
    if (track.featured_artists && track.featured_artists.length) {
      featuredArtists = track.featured_artists.map((a) => ({ id: a.id, name: a.name }));
    } else if (track.featured_artist_ids && track.featured_artist_ids.length) {
      preloadArtists(track.featured_artist_ids);
      featuredArtists = await Promise.all(
        track.featured_artist_ids.map(async (id) => ({ id, name: await getArtistName(id) }))
      );
    }
  });

  function formatDuration(ms: number): string {
    const s = Math.floor(ms / 1000);
    return `${Math.floor(s / 60)}:${(s % 60).toString().padStart(2, '0')}`;
  }
</script>

<div
  class="track-row"
  class:playing={isPlaying}
  on:click={(e) => {
    // Don't trigger play when clicking on internal links (allow SvelteKit to handle navigation)
    const el = e.target as HTMLElement | null;
    if (el && el.closest && el.closest('a')) return;
    playTrack(track, trackList);
  }}
  on:keydown={(e) => {
    if (e.key === 'Enter') {
      const el = e.target as HTMLElement | null;
      if (el && el.closest && el.closest('a')) return;
      playTrack(track, trackList);
    }
  }}
  on:contextmenu={(e) => openContextMenu(e, track)}
  role="button"
  tabindex="0"
>
  <span class="index">
    {#if isPlaying}
      <span class="playing-dots">
        <span class="dot"></span>
        <span class="dot"></span>
        <span class="dot"></span>
      </span>
    {:else}
      {useRankIndex ? (index + 1) : (track.track_number ?? (index + 1))}
    {/if}
  </span>
  {#if showCover}
    <div class="cover-thumb">
      {#if track.album_id}
        <img src="{getApiBase()}/covers/{track.album_id}" alt="" loading="lazy" />
      {:else}
        <div class="cover-placeholder"></div>
      {/if}
    </div>
  {/if}
  <div class="track-info">
    <span class="title">{track.title}</span>
    {#if mainArtist || featuredArtists.length}
      <span class="meta">
        {#if mainArtist}
          {#if mainArtist.id}
            <a class="artist-link" href="/artists/{mainArtist.id}">{mainArtist.name}</a>
          {:else}
            <span>{mainArtist.name}</span>
          {/if}
        {/if}
        {#if featuredArtists.length}
          <span class="feat-sep">{mainArtist ? ' feat. ' : 'feat. '}</span>
          {#each featuredArtists as fa, i}
            {#if i > 0}<span class="comma">, </span>{/if}
            {#if fa.id}
              <a class="artist-link" href="/artists/{fa.id}">{fa.name}</a>
            {:else}
              <span>{fa.name}</span>
            {/if}
          {/each}
        {/if}
      </span>
    {/if}
  </div>
  {#if track.bpm}
    <span class="bpm" title="BPM">{Math.round(track.bpm)}</span>
  {/if}
  <span class="row-actions">
    <button
      class="fav-btn"
      class:fav-active={$favorites.has(track.id)}
      on:click|stopPropagation={() => favorites.toggle(track.id, track)}
      aria-label={$favorites.has(track.id) ? 'Remove from favorites' : 'Add to favorites'}
      title={$favorites.has(track.id) ? 'Remove from favorites' : 'Add to favorites'}
    >
      <svg width="13" height="13" viewBox="0 0 24 24" fill={$favorites.has(track.id) ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/>
      </svg>
    </button>
    <StarRating trackId={track.id} size={13} />
  </span>
  {#if dlStatus === 'done'}
    <button
      class="dl-icon dl-done-btn"
      title="Remove download"
      on:click|stopPropagation={() => deleteDownload(track.id)}
      aria-label="Remove download"
    >
      <svg class="dl-dl-icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M12 16l-5-5h3V4h4v7h3l-5 5zm-7 2h14v2H5v-2z"/></svg>
      <svg class="dl-x-icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/></svg>
    </button>
  {:else if dlStatus === 'downloading'}
    <span class="dl-icon dl-progress" title="Downloading">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M12 16l-5-5h3V4h4v7h3l-5 5zm-7 2h14v2H5v-2z"/></svg>
    </span>
  {/if}
  <span class="duration">{formatDuration(track.duration_ms)}</span>
</div>

<style>
  .track-row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 12px;
    border-radius: 6px;
    cursor: pointer;
    transition: background 0.1s;
  }
  .track-row:hover { background: var(--bg-hover); }
  .track-row.playing { background: var(--bg-hover); color: var(--accent); }
  .index { width: 24px; text-align: center; font-size: 0.8rem; color: var(--text-muted); flex-shrink: 0; display: flex; align-items: center; justify-content: center; }
  .playing .index { color: var(--accent); }
  .playing-dots { display: flex; align-items: flex-end; gap: 2px; height: 12px; }
  .dot { width: 3px; height: 3px; background: currentColor; border-radius: 50%; animation: dot-jump 1.2s ease-in-out infinite; }
  .dot:nth-child(2) { animation-delay: 0.4s; }
  .dot:nth-child(3) { animation-delay: 0.8s; }
  @keyframes dot-jump {
    0%, 100% { transform: translateY(0); }
    25% { transform: translateY(-5px); }
    50% { transform: translateY(0); }
  }
  .cover-thumb { width: 38px; height: 38px; flex-shrink: 0; }
  .cover-thumb img { width: 38px; height: 38px; border-radius: 4px; object-fit: cover; display: block; }
  .cover-placeholder { width: 38px; height: 38px; border-radius: 4px; background: var(--bg-hover); }
  .track-info { flex: 1; overflow: hidden; }
  .title { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; display: block; font-size: 0.9rem; }
  .meta { display: block; font-size: 0.8rem; color: var(--text-muted); margin-top: 2px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .artist-link {
    color: inherit;
    text-decoration: none;
  }
  .artist-link:hover {
    text-decoration: underline;
    color: var(--text-primary, currentColor);
  }
  .feat-sep, .comma { color: var(--text-muted); }
  .bpm { font-size: 0.75rem; color: var(--text-muted); flex-shrink: 0; opacity: 0.7; }
  .duration { font-size: 0.8rem; color: var(--text-muted); flex-shrink: 0; width: 3.5rem; text-align: right; }
  .row-actions {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 2px;
    opacity: 0;
    transition: opacity 0.15s;
  }
  @media (max-width: 640px) {
    .row-actions {
      display: none !important;
    }
  }
  .track-row:hover .row-actions { opacity: 1; }
  .track-row.playing .row-actions { opacity: 1; }
  .fav-btn {
    background: none;
    border: none;
    padding: 2px 3px;
    cursor: pointer;
    color: var(--text-muted);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: color 0.15s;
    line-height: 1;
  }
  .fav-btn:hover { color: var(--text); }
  .fav-btn.fav-active { color: #e05; }
  .dl-icon { display: flex; align-items: center; flex-shrink: 0; color: var(--accent); opacity: 0.8; }
  .dl-done-btn { background: none; border: none; padding: 0; cursor: pointer; }
  .dl-done-btn .dl-x-icon { display: none; color: var(--text-muted); }
  .dl-done-btn:hover .dl-dl-icon { display: none; }
  .dl-done-btn:hover .dl-x-icon { display: block; }
  .dl-done-btn:hover { opacity: 1; }
  .dl-progress { opacity: 0.45; animation: dl-pulse 1.5s ease-in-out infinite; }
  @keyframes dl-pulse { 0%, 100% { opacity: 0.45; } 50% { opacity: 0.9; } }
</style>
