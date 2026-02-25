<script lang="ts">
  import type { Track } from '$lib/types';
  import { playTrack, currentTrack, playbackState } from '$lib/stores/player';
  import { openContextMenu } from '$lib/stores/contextMenu';
  import { onMount } from 'svelte';
  import { getArtistName, preloadArtists } from '$lib/stores/artists';

  export let track: Track;
  export let trackList: Track[] = [];
  export let index: number = 0;

  $: isPlaying = $currentTrack?.id === track.id && $playbackState === 'playing';

  let artistName: string = '';
  let featuredNames: string[] = [];

  onMount(async () => {
    if (track.artist_name) {
      artistName = track.artist_name;
    } else if (track.artist_id) {
      artistName = await getArtistName(track.artist_id);
    }

    if (track.featured_artists && track.featured_artists.length) {
      featuredNames = track.featured_artists.map((a) => a.name);
    } else if (track.featured_artist_ids && track.featured_artist_ids.length) {
      // preload then resolve names
      preloadArtists(track.featured_artist_ids);
      featuredNames = await Promise.all(track.featured_artist_ids.map((id) => getArtistName(id)));
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
  <span class="index">{isPlaying ? '▶' : index + 1}</span>
  <div class="track-info">
    <span class="title">{track.title}</span>
    {#if artistName || featuredNames.length}
      <span class="meta">
        {#if artistName}{artistName}{/if}
        {#if featuredNames.length}
          {#if artistName} — {/if}
          feat. {featuredNames.join(', ')}
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
  .index { width: 24px; text-align: center; font-size: 0.8rem; color: var(--text-muted); flex-shrink: 0; }
  .playing .index { color: var(--accent); }
  .track-info { flex: 1; overflow: hidden; }
  .title { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; display: block; font-size: 0.9rem; }
  .meta { display: block; font-size: 0.8rem; color: var(--text-muted); margin-top: 2px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .duration { font-size: 0.8rem; color: var(--text-muted); flex-shrink: 0; }
</style>
