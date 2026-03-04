<script lang="ts">
  import type { Track } from '$lib/types';
  import { playTrack, currentTrack, playbackState } from '$lib/stores/player';
  import { openContextMenu } from '$lib/stores/contextMenu';
  import { onMount } from 'svelte';
  import { getArtistName, preloadArtists } from '$lib/stores/artists';

  export let track: Track;
  export let trackList: Track[] = [];
  export let index: number = 0;
  export let showCover: boolean = false;

  import { getApiBase } from '$lib/api/base';

  $: isPlaying = $currentTrack?.id === track.id && $playbackState === 'playing';

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
  on:click={() => playTrack(track, trackList)}
  on:keydown={(e) => e.key === 'Enter' && playTrack(track, trackList)}
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
      {index + 1}
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
            <a class="artist-link" href="/artists/{mainArtist.id}" on:click|stopPropagation>{mainArtist.name}</a>
          {:else}
            <span>{mainArtist.name}</span>
          {/if}
        {/if}
        {#if featuredArtists.length}
          <span class="feat-sep">{mainArtist ? ' feat. ' : 'feat. '}</span>
          {#each featuredArtists as fa, i}
            {#if i > 0}<span class="comma">, </span>{/if}
            {#if fa.id}
              <a class="artist-link" href="/artists/{fa.id}" on:click|stopPropagation>{fa.name}</a>
            {:else}
              <span>{fa.name}</span>
            {/if}
          {/each}
        {/if}
      </span>
    {/if}
  </div>
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
  .duration { font-size: 0.8rem; color: var(--text-muted); flex-shrink: 0; }
</style>
