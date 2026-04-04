<script lang="ts">
  import type { Artist } from "$lib/types";
  import { goto } from "$app/navigation";
  import { getApiBase } from "$lib/api/base";
  import TrackRow from "$lib/components/library/TrackRow.svelte";

  export let artists: Artist[] = [];
  export let onSelect: ((artist: Artist) => void) | undefined = undefined;
</script>

<div class="artist-list">
  {#each artists as artist (artist.id)}
    <div class="artist-card">
      <button
        class="artist-row"
        on:click={() => onSelect ? onSelect(artist) : goto(`/artists/${artist.id}`)}
        aria-label="View artist {artist.name}"
      >
        {#if artist.image_key}
          <img
            class="artist-thumb"
            src="{getApiBase()}/covers/artist/{artist.id}"
            alt={artist.name}
            loading="lazy"
          />
        {:else}
          <div class="monogram">{artist.name.slice(0, 1).toUpperCase()}</div>
        {/if}
        <span class="name">{artist.name}</span>
        <svg
          class="chevron"
          width="14"
          height="14"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          viewBox="0 0 24 24"
          aria-hidden="true"
        >
          <polyline points="9 18 15 12 9 6" />
        </svg>
      </button>

      {#if artist.top_tracks && artist.top_tracks.length > 0}
        <div class="top-tracks">
          {#each artist.top_tracks as track, i (track.id)}
            <TrackRow
              {track}
              trackList={artist.top_tracks}
              index={i}
              useRankIndex={true}
            />
          {/each}
        </div>
      {/if}
    </div>
  {/each}
</div>

<style>
  .artist-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .artist-card {
    border-radius: 8px;
    overflow: hidden;
    border: 1px solid transparent;
    transition: border-color 0.15s;
  }
  .artist-card:has(.top-tracks) {
    border-color: var(--border);
    background: var(--surface);
  }

  .artist-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 10px;
    border-radius: 6px;
    cursor: pointer;
    background: none;
    border: none;
    text-align: left;
    color: var(--text);
    transition: background 0.1s;
    width: 100%;
    min-width: 0;
  }
  .artist-card:has(.top-tracks) .artist-row {
    border-radius: 8px 8px 0 0;
  }
  .artist-row:hover {
    background: var(--bg-hover);
  }

  .artist-thumb {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    object-fit: cover;
    flex-shrink: 0;
    display: block;
  }

  .monogram {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: var(--accent-dim);
    border: 1px solid var(--accent-glow);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.875rem;
    font-weight: 700;
    color: var(--accent);
    flex-shrink: 0;
    font-family: "Syne", sans-serif;
  }

  .name {
    flex: 1;
    font-size: 0.875rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .chevron {
    color: var(--text-muted);
    flex-shrink: 0;
    opacity: 0.5;
  }
  .artist-row:hover .chevron {
    opacity: 1;
    color: var(--accent);
  }

  /* Top tracks */
  .top-tracks {
    border-top: 1px solid var(--border);
    padding: 4px 0;
  }
</style>
