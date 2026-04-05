<script lang="ts">
  import type { Album } from "$lib/types";
  import AlbumCard from "./AlbumCard.svelte";

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
    margin-bottom: 36px;
  }

  .section-header {
    position: sticky;
    top: calc(-1 * var(--page-padding));
    z-index: 10;
    background: var(--bg);
    font-size: 0.7rem;
    font-weight: 700;
    color: var(--accent);
    padding: 8px 0;
    margin-bottom: 14px;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    opacity: 0.75;
  }

  @media (max-width: 640px) {
    .section-header {
      position: static;
    }
  }

  .album-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 20px 14px;
  }
</style>
