<script lang="ts">
  import { onMount } from 'svelte';
  import { library } from '$lib/api/library';
  import TrackList from '$lib/components/library/TrackList.svelte';
  import { playTrack, shuffle } from '$lib/stores/player';
  import type { Track } from '$lib/types';

  let tracks: Track[] = [];
  let loading = true;

  onMount(async () => {
    try {
      tracks = await library.favorites();
    } finally {
      loading = false;
    }
  });

  function playAll() {
    if (tracks.length > 0) playTrack(tracks[0], tracks);
  }

  function shuffleAll() {
    if (tracks.length === 0) return;
    shuffle.set(true);
    playTrack(tracks[Math.floor(Math.random() * tracks.length)], tracks);
  }
</script>

<div class="favorites-page">
  <div class="header">
    <h2 class="title">Favorites</h2>
    {#if !loading && tracks.length > 0}
      <div class="actions">
        <button class="btn-play" on:click={playAll}>▶ Play</button>
        <button class="btn-shuffle" on:click={shuffleAll} title="Shuffle">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/>
            <polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/>
            <line x1="4" y1="4" x2="9" y2="9"/>
          </svg>
          Shuffle
        </button>
      </div>
    {/if}
  </div>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if tracks.length === 0}
    <p class="muted">No favorites yet — right-click a track to add one.</p>
  {:else}
    <TrackList {tracks} showCover={true} />
  {/if}
</div>

<style>
  .header { display: flex; align-items: center; gap: 16px; margin-bottom: 20px; }
  .title { font-size: 1.25rem; font-weight: 600; margin: 0; }
  .actions { display: flex; gap: 8px; align-items: center; }
  .btn-play {
    background: var(--accent);
    border: none;
    border-radius: 20px;
    padding: 7px 18px;
    color: #fff;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-play:hover { background: var(--accent-hover); }
  .btn-shuffle {
    display: flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 6px 14px;
    color: var(--text-muted);
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-shuffle:hover { color: var(--text); border-color: var(--text); }
  .muted { color: var(--text-muted); font-size: 0.875rem; }
</style>
