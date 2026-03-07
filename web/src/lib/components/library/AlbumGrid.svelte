<script lang="ts">
  import type { Album } from '$lib/types';
  import AlbumCard from './AlbumCard.svelte';

  export let grouped: Map<string, Album[]>;
  export let keys: string[];
</script>

{#each keys as key}
  <section class="album-section" data-scroll-key={key}>
    <div class="section-header">{key}</div>
    <div class="album-grid">
      {#each grouped.get(key) ?? [] as album (album.id)}
        <AlbumCard {album} />
      {/each}
    </div>
  </section>
{/each}

<style>
  .album-section {
    margin-bottom: 40px;
  }

  .section-header {
    position: sticky;
    top: calc(-1 * var(--page-padding));
    z-index: 10;
    background: var(--bg);
    font-size: 0.8125rem;
    font-weight: 700;
    color: var(--accent);
    padding-top: 10px;
    padding-bottom: 10px;
    margin-bottom: 16px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    border-bottom: 1px solid var(--border);
  }

  .album-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 16px;
  }
</style>
