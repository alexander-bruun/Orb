<script lang="ts">
  import type { Playlist } from '$lib/types';
  import { goto } from '$app/navigation';

  export let playlist: Playlist;
</script>

<button class="playlist-card" on:click={() => goto(`/playlists/${playlist.id}`)}>
  <div class="cover placeholder">♪</div>
  <div class="info">
    <span class="name">{playlist.name}</span>
    {#if playlist.description}
      <span class="desc">{playlist.description}</span>
    {/if}
  </div>
</button>

<style>
  .playlist-card {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    border-radius: 6px;
    cursor: pointer;
    background: none;
    border: none;
    width: 100%;
    text-align: left;
    transition: background 0.1s;
  }
  .playlist-card:hover { background: var(--bg-hover); }
  .cover {
    width: 48px; height: 48px;
    background: var(--bg-hover);
    border-radius: 4px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 1.2rem;
    color: var(--text-muted);
    flex-shrink: 0;
  }
  .info { display: flex; flex-direction: column; overflow: hidden; }
  .name { font-size: 0.9rem; font-weight: 600; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .desc { font-size: 0.75rem; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>
