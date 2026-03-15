<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { smartPlaylists } from '$lib/api/smartPlaylists';
  import type { SmartPlaylist } from '$lib/types';

  let items: SmartPlaylist[] = [];
  let loading = true;
  let creating = false;
  let newName = '';
  let showForm = false;
  let error = '';

  $: systemItems = items.filter(p => p.system);
  $: userItems = items.filter(p => !p.system);

  onMount(async () => {
    try {
      items = (await smartPlaylists.list()) ?? [];
    } catch {
      // ignore
    } finally {
      loading = false;
    }
  });

  async function create() {
    if (!newName.trim()) return;
    creating = true;
    error = '';
    try {
      const pl = await smartPlaylists.create({ name: newName.trim() });
      if (pl) goto(`/smart-playlists/${pl.id}`);
    } catch (e: any) {
      error = e?.message ?? 'Failed to create';
    } finally {
      creating = false;
    }
  }

  async function remove(id: string) {
    await smartPlaylists.delete(id);
    items = items.filter(p => p.id !== id);
  }

  function formatDate(iso: string) {
    return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
  }

  const systemIcons: Record<string, string> = {
    'Most Played': 'M9 18V5l12-2v13',
    'Top Rated': 'M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z',
    'Recently Added': 'M12 5v14M5 12l7-7 7 7',
    'Never Played': 'M9 18V5l12-2v13M6 15H3M6 12H3M6 9H3',
    'Forgotten Favorites': 'M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z',
  };
</script>

<svelte:head><title>Smart Playlists – Orb</title></svelte:head>

<div class="page">
  <div class="header">
    <h1 class="title">Smart Playlists</h1>
    <button class="btn-primary" on:click={() => { showForm = !showForm; newName = ''; error = ''; }}>
      {showForm ? 'Cancel' : '+ New'}
    </button>
  </div>

  {#if showForm}
    <form class="create-form" on:submit|preventDefault={create}>
      <input
        class="input"
        bind:value={newName}
        placeholder="Playlist name…"
        autofocus
      />
      <button class="btn-primary" type="submit" disabled={creating || !newName.trim()}>
        {creating ? 'Creating…' : 'Create'}
      </button>
      {#if error}<span class="error">{error}</span>{/if}
    </form>
  {/if}

  {#if loading}
    <p class="muted">Loading…</p>
  {:else}
    {#if systemItems.length > 0}
      <div class="section-label">Auto-Generated</div>
      <ul class="list system-list">
        {#each systemItems as pl (pl.id)}
          <li class="item">
            <a class="item-link" href="/smart-playlists/{pl.id}">
              <div class="item-icon system-icon" aria-hidden="true">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d={systemIcons[pl.name] ?? 'M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01'}/>
                </svg>
              </div>
              <div class="item-info">
                <span class="item-name">{pl.name}</span>
                {#if pl.description}
                  <span class="item-meta">{pl.description}</span>
                {/if}
              </div>
            </a>
          </li>
        {/each}
      </ul>
    {/if}

    {#if userItems.length > 0 || systemItems.length > 0}
      <div class="section-label" class:section-label-gap={systemItems.length > 0}>
        My Playlists
        {#if userItems.length === 0}<span class="muted" style="font-weight:400"> — none yet</span>{/if}
      </div>
    {/if}

    {#if userItems.length > 0}
      <ul class="list">
        {#each userItems as pl (pl.id)}
          <li class="item">
            <a class="item-link" href="/smart-playlists/{pl.id}">
              <div class="item-icon" aria-hidden="true">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/>
                  <line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/>
                </svg>
              </div>
              <div class="item-info">
                <span class="item-name">{pl.name}</span>
                <span class="item-meta">
                  {pl.rules.length} rule{pl.rules.length === 1 ? '' : 's'} ·
                  {pl.last_built_at ? `Built ${formatDate(pl.last_built_at)}` : 'Never built'}
                </span>
              </div>
            </a>
            <button class="btn-delete" title="Delete" on:click={() => remove(pl.id)}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14H6L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4h6v2"/>
              </svg>
            </button>
          </li>
        {/each}
      </ul>
    {:else if systemItems.length === 0}
      <div class="empty">
        <p>No smart playlists yet.</p>
        <p class="muted">Create one to build a dynamic playlist from filter rules — genre, year, play count, and more.</p>
      </div>
    {/if}
  {/if}
</div>

<style>
  .page { max-width: 680px; }
  .header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 24px; }
  .title { font-size: 1.25rem; font-weight: 600; margin: 0; }
  .section-label {
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
    margin-bottom: 6px;
  }
  .section-label-gap { margin-top: 20px; }
  .system-list { margin-bottom: 4px; }
  .system-icon { background: color-mix(in srgb, var(--accent) 15%, var(--bg-3)); color: var(--accent); }
  .btn-primary {
    background: var(--accent);
    border: none;
    border-radius: 20px;
    color: #fff;
    cursor: pointer;
    font-size: 0.85rem;
    font-weight: 600;
    padding: 7px 18px;
  }
  .btn-primary:hover { background: var(--accent-hover, var(--accent)); filter: brightness(1.1); }
  .btn-primary:disabled { opacity: 0.5; cursor: default; }
  .create-form {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 24px;
    flex-wrap: wrap;
  }
  .input {
    flex: 1;
    min-width: 200px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font-size: 0.9rem;
    padding: 8px 12px;
  }
  .input:focus { outline: none; border-color: var(--accent); }
  .error { color: var(--error, #f87171); font-size: 0.8rem; }
  .empty { padding: 40px 0; }
  .empty p { margin: 0 0 6px; }
  .muted { color: var(--text-muted); font-size: 0.875rem; }
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
  .item {
    display: flex;
    align-items: center;
    border-radius: 8px;
    padding: 2px 4px 2px 2px;
  }
  .item:hover { background: var(--bg-hover); }
  .item-link {
    display: flex;
    align-items: center;
    gap: 12px;
    flex: 1;
    padding: 10px 8px;
    text-decoration: none;
    color: inherit;
    border-radius: 6px;
    min-width: 0;
  }
  .item-icon {
    width: 36px;
    height: 36px;
    background: var(--bg-3);
    border-radius: 6px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    color: var(--text-muted);
  }
  .item-info { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .item-name { font-size: 0.9rem; font-weight: 500; color: var(--text); }
  .item-meta { font-size: 0.75rem; color: var(--text-muted); }
  .btn-delete {
    background: none;
    border: none;
    border-radius: 6px;
    color: var(--text-muted);
    cursor: pointer;
    padding: 6px;
    opacity: 0;
    transition: opacity 0.15s, color 0.15s;
  }
  .item:hover .btn-delete { opacity: 1; }
  .btn-delete:hover { color: var(--error, #f87171); }
</style>
