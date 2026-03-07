<script lang="ts">
  import type { Artist } from '$lib/types';
  import { goto } from '$app/navigation';

  export let artists: Artist[] = [];
</script>

<div class="artist-list">
  {#each artists as artist (artist.id)}
    <button class="artist-row" on:click={() => goto(`/artists/${artist.id}`)}>
      <div class="monogram">{artist.name.slice(0, 1).toUpperCase()}</div>
      <span class="name">{artist.name}</span>
      <svg class="chevron" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true">
        <polyline points="9 18 15 12 9 6"/>
      </svg>
    </button>
  {/each}
</div>

<style>
  .artist-list {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: 4px;
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
  .artist-row:hover { background: var(--bg-hover); }

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
    font-family: 'Syne', sans-serif;
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
  .artist-row:hover .chevron { opacity: 1; color: var(--accent); }
</style>
