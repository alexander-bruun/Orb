<script lang="ts">
  import { onMount } from 'svelte';
  import { playlists as playlistApi } from '$lib/api/playlists';
  import PlaylistCard from '$lib/components/playlist/PlaylistCard.svelte';
  import type { Playlist } from '$lib/types';

  let items: Playlist[] = [];
  let loading = true;
  let creating = false;
  let newName = '';

  onMount(async () => {
    try {
      items = await playlistApi.list();
    } finally {
      loading = false;
    }
  });

  async function createPlaylist(e: Event) {
    e.preventDefault();
    if (!newName.trim()) return;
    creating = true;
    try {
      const pl = await playlistApi.create(newName.trim());
      items = [...items, pl];
      newName = '';
    } finally {
      creating = false;
    }
  }
</script>

<div class="playlists-page">
  <h2 class="title">Playlists</h2>

  <form class="create-form" on:submit={createPlaylist}>
    <input type="text" placeholder="New playlist name…" bind:value={newName} class="create-input" />
    <button type="submit" class="btn-create" disabled={creating}>Create</button>
  </form>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if (items?.length ?? 0) === 0}
    <p class="muted">No playlists yet</p>
  {:else}
    {#each items as pl (pl.id)}
      <PlaylistCard playlist={pl} />
    {/each}
  {/if}
</div>

<style>
  .title { font-size: 1.25rem; font-weight: 600; margin-bottom: 20px; }
  .create-form { display: flex; gap: 8px; margin-bottom: 24px; }
  .create-input {
    flex: 1;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    color: var(--text);
    font-size: 0.875rem;
    outline: none;
  }
  .create-input:focus { border-color: var(--accent); }
  .btn-create {
    background: var(--accent);
    border: none;
    border-radius: 6px;
    padding: 8px 16px;
    color: #fff;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-create:hover { background: var(--accent-hover); }
  .btn-create:disabled { opacity: 0.6; cursor: not-allowed; }
  .muted { color: var(--text-muted); font-size: 0.875rem; }
</style>
