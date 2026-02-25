<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { playlists as playlistApi } from '$lib/api/playlists';
  import TrackList from '$lib/components/library/TrackList.svelte';
  import type { Playlist, Track } from '$lib/types';
  import { playTrack } from '$lib/stores/player';

  let playlist: Playlist | null = null;
  let tracks: Track[] = [];
  let loading = true;

  onMount(async () => {
    const id = $page.params.id;
    try {
      const res = await playlistApi.get(id);
      playlist = res.playlist;
      tracks = res.tracks;
    } finally {
      loading = false;
    }
  });

  function playAll() {
    if ((tracks?.length ?? 0) > 0) playTrack(tracks[0], tracks);
  }
</script>

{#if loading}
  <p class="muted">Loading…</p>
{:else if playlist}
  <div class="header">
    <div class="cover-placeholder">♪</div>
    <div class="meta">
      <p class="type">Playlist</p>
      <h1 class="title">{playlist.name}</h1>
      {#if playlist.description}
        <p class="desc">{playlist.description}</p>
      {/if}
      <button class="btn-play" on:click={playAll} disabled={(tracks?.length ?? 0) === 0}>▶ Play</button>
    </div>
  </div>
  <TrackList {tracks} />
{/if}

<style>
  .header { display: flex; gap: 24px; align-items: flex-end; margin-bottom: 32px; }
  .cover-placeholder {
    width: 180px; height: 180px;
    background: var(--bg-hover);
    border-radius: 8px;
    display: flex; align-items: center; justify-content: center;
    font-size: 3rem; color: var(--text-muted);
    flex-shrink: 0;
  }
  .meta { display: flex; flex-direction: column; gap: 6px; }
  .type { font-size: 0.75rem; text-transform: uppercase; color: var(--text-muted); }
  .title { font-size: 2rem; font-weight: 700; margin: 0; }
  .desc { color: var(--text-muted); font-size: 0.875rem; }
  .btn-play {
    background: var(--accent);
    border: none;
    border-radius: 20px;
    padding: 8px 20px;
    color: #fff;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    align-self: flex-start;
    margin-top: 4px;
  }
  .btn-play:hover { background: var(--accent-hover); }
  .btn-play:disabled { opacity: 0.6; cursor: not-allowed; }
  .muted { color: var(--text-muted); }
</style>
