<script lang="ts">
  import type { SmartPlaylist } from "$lib/types";
  import { goto } from "$app/navigation";

  export let playlist: SmartPlaylist;

  const systemIcons: Record<string, string> = {
    "Most Played": "M9 18V5l12-2v13",
    "Top Rated":
      "M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z",
    "Recently Added": "M12 5v14M5 12l7-7 7 7",
    "Never Played": "M9 18V5l12-2v13M6 15H3M6 12H3M6 9H3",
    "Forgotten Favorites":
      "M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z",
  };
</script>

<button
  class="playlist-card"
  on:click={() => goto(`/smart-playlists/${playlist.id}`)}
  aria-label="View smart playlist {playlist.name}"
>
  <div class="cover" class:system={playlist.system}>
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <path
        d={systemIcons[playlist.name] ??
          "M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"}
      />
    </svg>
  </div>
  <div class="info">
    <span class="name">{playlist.name}</span>
    <span class="desc">
      {#if playlist.system}
        {playlist.description || "Auto-generated"}
      {:else}
        {playlist.rules.length} rule{playlist.rules.length === 1 ? "" : "s"}
      {/if}
    </span>
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
  .playlist-card:hover {
    background: var(--bg-hover);
  }
  .cover {
    width: 48px;
    height: 48px;
    background: var(--bg-3);
    border-radius: 4px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    flex-shrink: 0;
  }
  .cover.system {
    background: color-mix(in srgb, var(--accent) 15%, var(--bg-3));
    color: var(--accent);
  }
  .info {
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .name {
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .desc {
    font-size: 0.75rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
