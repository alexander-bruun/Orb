<script lang="ts">
  import { onMount } from 'svelte';
  import { playlists as playlistApi } from '$lib/api/playlists';
  import { getPlaylistCoverGrid } from '$lib/api/playlists';
  import { smartPlaylists as smartPlaylistApi } from '$lib/api/smartPlaylists';
  import PlaylistCard from '$lib/components/playlist/PlaylistCard.svelte';
  import SmartPlaylistCard from '$lib/components/playlist/SmartPlaylistCard.svelte';
  import type { Playlist, SmartPlaylist } from '$lib/types';
  import { goto } from '$app/navigation';

  let items: Playlist[] = [];
  let smartItems: SmartPlaylist[] = [];
  let coverGrids: Record<string, string[]> = {};
  let loading = true;
  let creating = false;
  let newName = '';
  let showSmartForm = false;
  let smartName = '';
  let isRestoring = false;

  export const snapshot = {
    capture: () => ({
      items,
      smartItems,
      coverGrids
    }),
    restore: (value) => {
      items = value.items;
      smartItems = value.smartItems;
      coverGrids = value.coverGrids;
      isRestoring = true;
      loading = false;
    }
  };

  $: systemSmartPlaylists = smartItems.filter(p => p.system);
  $: userSmartPlaylists = smartItems.filter(p => !p.system);

  onMount(async () => {
    if (isRestoring && (items.length > 0 || smartItems.length > 0)) {
      loading = false;
      return;
    }

    try {
      const [pls, spls] = await Promise.all([
        playlistApi.list(),
        smartPlaylistApi.list()
      ]);
      items = pls;
      smartItems = spls;

      // Fetch cover grids for all normal playlists in parallel
      await Promise.all(items.map(async (pl) => {
        try {
          coverGrids[pl.id] = await getPlaylistCoverGrid(pl.id);
        } catch (e) {
          coverGrids[pl.id] = [];
        }
      }));
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

  async function createSmartPlaylist(e: Event) {
    e.preventDefault();
    if (!smartName.trim()) return;
    creating = true;
    try {
      const pl = await smartPlaylistApi.create({ name: smartName.trim() });
      if (pl) goto(`/smart-playlists/${pl.id}`);
    } finally {
      creating = false;
    }
  }
</script>

<div class="playlists-page">
  <div class="header">
    <h2 class="title">Playlists</h2>
    <div class="header-actions">
       <button class="btn-secondary" on:click={() => showSmartForm = !showSmartForm}>
         {showSmartForm ? 'Cancel' : '+ Smart'}
       </button>
    </div>
  </div>

  {#if showSmartForm}
    <form class="create-form" on:submit={createSmartPlaylist}>
      <input type="text" placeholder="New smart playlist name…" bind:value={smartName} class="create-input" />
      <button type="submit" class="btn-create" disabled={creating || !smartName.trim()}>Create</button>
    </form>
  {:else}
    <form class="create-form" on:submit={createPlaylist}>
      <input type="text" placeholder="New playlist name…" bind:value={newName} class="create-input" />
      <button type="submit" class="btn-create" disabled={creating || !newName.trim()}>Create</button>
    </form>
  {/if}

  {#if loading}
    <p class="muted">Loading…</p>
  {:else}
    {#if systemSmartPlaylists.length > 0}
      <div class="section-label">Auto-Generated</div>
      <div class="list">
        {#each systemSmartPlaylists as pl (pl.id)}
          <SmartPlaylistCard playlist={pl} />
        {/each}
      </div>
    {/if}

    {#if userSmartPlaylists.length > 0}
      <div class="section-label" class:mt-24={systemSmartPlaylists.length > 0}>Smart Playlists</div>
      <div class="list">
        {#each userSmartPlaylists as pl (pl.id)}
          <SmartPlaylistCard playlist={pl} />
        {/each}
      </div>
    {/if}

    <div class="section-label" class:mt-24={smartItems.length > 0}>My Playlists</div>
    {#if items.length === 0}
      <p class="muted">No manual playlists yet</p>
    {:else}
      <div class="list">
        {#each items as pl (pl.id)}
          <PlaylistCard playlist={pl} coverGrid={coverGrids[pl.id]} />
        {/each}
      </div>
    {/if}
  {/if}
</div>

<svelte:head><title>Playlists – Orb</title></svelte:head>

<style>
  .playlists-page { max-width: 800px; }
  .header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 20px; }
  .title { font-size: 1.25rem; font-weight: 600; margin: 0; }
  .header-actions { display: flex; gap: 8px; }

  .create-form { display: flex; gap: 8px; margin-bottom: 24px; }
  .create-input {
    flex: 1;
    background: var(--bg-elevated);
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

  .btn-secondary {
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 12px;
    color: var(--text-muted);
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-secondary:hover { color: var(--text); border-color: var(--text-muted); }

  .section-label {
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
    margin-bottom: 8px;
  }
  .mt-24 { margin-top: 24px; }

  .list { display: flex; flex-direction: column; gap: 2px; }
  .muted { color: var(--text-muted); font-size: 0.875rem; padding: 0 12px; }
</style>
