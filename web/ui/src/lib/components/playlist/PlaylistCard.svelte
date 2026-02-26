
<script lang="ts">
  import type { Playlist } from '$lib/types';
  import { goto } from '$app/navigation';

  const BASE = import.meta.env.VITE_API_BASE ?? '/api';

  export let playlist: Playlist;
  export let coverGrid: string[] | undefined;
</script>

<button class="playlist-card" on:click={() => goto(`/playlists/${playlist.id}`)}>
  <div class="cover cover-grid">
    {#if coverGrid && coverGrid.length > 0}
      <div class="grid">
        {#each Array(4) as _, i}
          {#if coverGrid[i]}
            <img src={coverGrid[i]} alt="cover" class="grid-img" />
          {:else}
            <span class="grid-fallback">♪</span>
          {/if}
        {/each}
      </div>
    {:else}
      <img src="{BASE}/covers/playlist/{playlist.id}" alt="cover" style="width:100%;height:100%;object-fit:cover;border-radius:4px;" on:error={(e) => { e.target.style.display = 'none'; }} />
      <span class="placeholder" style="position:absolute;left:0;top:0;width:100%;height:100%;display:flex;align-items:center;justify-content:center;">♪</span>
    {/if}
  </div>
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
  .cover-grid {
    position: relative;
    width: 48px;
    height: 48px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg-hover);
    border-radius: 4px;
    overflow: hidden;
  }
  .cover-grid .grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    grid-template-rows: 1fr 1fr;
    width: 100%;
    height: 100%;
    gap: 0;
  }
  .cover-grid .grid-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    border-radius: 0;
    display: block;
  }
  .cover-grid .grid-fallback {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 1.2rem;
    color: var(--text-muted);
    background: var(--bg-hover);
  }
</style>
