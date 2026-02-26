<script lang="ts">
  import { contextMenu, closeContextMenu } from '$lib/stores/contextMenu';
  import { playTrack, playNext, addToQueue } from '$lib/stores/player';
  import { playlists as playlistsApi } from '$lib/api/playlists';
  import { favorites } from '$lib/stores/favorites';
  import type { Playlist } from '$lib/types';

  let showPlaylists = false;
  let playlists: Playlist[] = [];
  let loadingPlaylists = false;
  let addedId: string | null = null;

  $: if (!$contextMenu.visible) {
    showPlaylists = false;
    playlists = [];
    addedId = null;
  }

  $: isFav = $contextMenu.track ? $favorites.has($contextMenu.track.id) : false;

  async function handleFavorite() {
    const t = $contextMenu.track;
    if (!t) return;
    await favorites.toggle(t.id);
  }

  async function loadPlaylists() {
    loadingPlaylists = true;
    showPlaylists = true;
    try {
      playlists = await playlistsApi.list();
    } finally {
      loadingPlaylists = false;
    }
  }

  async function handleAddToPlaylist(playlistId: string) {
    const t = $contextMenu.track;
    if (!t) return;
    await playlistsApi.addTrack(playlistId, t.id);
    addedId = playlistId;
    setTimeout(closeContextMenu, 800);
  }

  function handlePlay() {
    const t = $contextMenu.track;
    if (t) playTrack(t, [t]);
    closeContextMenu();
  }

  function handlePlayNext() {
    const t = $contextMenu.track;
    if (t) playNext(t);
    closeContextMenu();
  }

  function handleAddToQueue() {
    const t = $contextMenu.track;
    if (t) addToQueue(t);
    closeContextMenu();
  }

  function handleShare() {
    const t = $contextMenu.track;
    if (t) navigator.clipboard?.writeText(`${location.origin}/tracks/${t.id}`);
    closeContextMenu();
  }

  function onWindowPointerDown(e: PointerEvent) {
    if (!$contextMenu.visible) return;
    const el = e.target as Element;
    if (!el.closest('.ctx-menu')) closeContextMenu();
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') closeContextMenu();
  }

  // Clamp position so the menu stays within the viewport
  $: style = (() => {
    const menuW = 172;
    const menuH = showPlaylists ? 280 : 234;
    const vw = typeof window !== 'undefined' ? window.innerWidth : 1920;
    const vh = typeof window !== 'undefined' ? window.innerHeight : 1080;
    const left = Math.min($contextMenu.x, vw - menuW - 8);
    const top = Math.min($contextMenu.y, vh - menuH - 8);
    return `left:${left}px;top:${top}px`;
  })();
</script>

<svelte:window on:pointerdown={onWindowPointerDown} on:keydown={onKeydown} />

{#if $contextMenu.visible && $contextMenu.track}
  <!-- svelte-ignore a11y-interactive-supports-focus -->
  <div class="ctx-menu" style={style} role="menu" on:click|stopPropagation={() => {}}>
    {#if !showPlaylists}
      <button class="item" on:click={handlePlay} role="menuitem">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
          <polygon points="5,3 19,12 5,21"/>
        </svg>
        Play
      </button>
      <button class="item" on:click={handlePlayNext} role="menuitem">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <polygon points="4,3 14,12 4,21" fill="currentColor" stroke="none"/>
          <line x1="19" y1="3" x2="19" y2="21"/>
        </svg>
        Play Next
      </button>
      <button class="item" on:click={handleAddToQueue} role="menuitem">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <line x1="9" y1="6" x2="21" y2="6"/>
          <line x1="9" y1="12" x2="21" y2="12"/>
          <line x1="9" y1="18" x2="21" y2="18"/>
          <circle cx="4" cy="6" r="1.5" fill="currentColor" stroke="none"/>
          <circle cx="4" cy="12" r="1.5" fill="currentColor" stroke="none"/>
          <circle cx="4" cy="18" r="1.5" fill="currentColor" stroke="none"/>
        </svg>
        Add to Queue
      </button>

      <div class="sep"></div>

      <button class="item" on:click={loadPlaylists} role="menuitem">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <path d="M9 18V5l12-2v13"/>
          <circle cx="6" cy="18" r="3"/>
          <circle cx="18" cy="16" r="3"/>
        </svg>
        Add to Playlist
        <svg class="ml-auto" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
          <polyline points="9,18 15,12 9,6"/>
        </svg>
      </button>

      <div class="sep"></div>

      <button class="item" class:fav={isFav} on:click={handleFavorite} role="menuitem">
        {#if isFav}
          <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <polygon points="12,2 15.09,8.26 22,9.27 17,14.14 18.18,21.02 12,17.77 5.82,21.02 7,14.14 2,9.27 8.91,8.26"/>
          </svg>
          Unfavorite
        {:else}
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <polygon points="12,2 15.09,8.26 22,9.27 17,14.14 18.18,21.02 12,17.77 5.82,21.02 7,14.14 2,9.27 8.91,8.26"/>
          </svg>
          Favorite
        {/if}
      </button>
      <button class="item" on:click={handleShare} role="menuitem">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <circle cx="18" cy="5" r="3"/>
          <circle cx="6" cy="12" r="3"/>
          <circle cx="18" cy="19" r="3"/>
          <line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/>
          <line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/>
        </svg>
        Share
      </button>
    {:else}
      <button class="item dim" on:click={() => (showPlaylists = false)} role="menuitem">
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
          <polyline points="15,18 9,12 15,6"/>
        </svg>
        Add to Playlist
      </button>
      <div class="sep"></div>
      {#if loadingPlaylists}
        <div class="hint">Loading…</div>
      {:else if playlists.length === 0}
        <div class="hint">No playlists yet</div>
      {:else}
        {#each playlists as pl (pl.id)}
          <button
            class="item"
            class:done={addedId === pl.id}
            on:click={() => handleAddToPlaylist(pl.id)}
            role="menuitem"
          >
            {#if addedId === pl.id}
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
                <polyline points="20,6 9,17 4,12"/>
              </svg>
            {:else}
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
                <line x1="12" y1="5" x2="12" y2="19"/>
                <line x1="5" y1="12" x2="19" y2="12"/>
              </svg>
            {/if}
            <span class="pl-name">{pl.name}</span>
          </button>
        {/each}
      {/if}
    {/if}
  </div>
{/if}

<style>
  .ctx-menu {
    position: fixed;
    z-index: 9000;
    background: var(--surface);
    border: 1px solid var(--border-2);
    border-radius: 8px;
    padding: 4px;
    min-width: 164px;
    box-shadow: 0 8px 32px rgba(0,0,0,0.55), 0 2px 8px rgba(0,0,0,0.3);
    animation: pop 0.1s ease-out;
  }

  @keyframes pop {
    from { opacity: 0; transform: scale(0.94); }
    to   { opacity: 1; transform: scale(1); }
  }

  .item {
    display: flex;
    align-items: center;
    gap: 9px;
    width: 100%;
    padding: 6px 10px;
    background: none;
    border: none;
    color: var(--text);
    font-size: 0.81rem;
    font-family: inherit;
    text-align: left;
    cursor: pointer;
    border-radius: 5px;
    transition: background 0.1s;
    white-space: nowrap;
  }
  .item:hover { background: var(--surface-2); }
  .item.dim { color: var(--text-2); }
  .item.done { color: var(--accent); }
  .item.fav { color: var(--accent); }

  .ml-auto { margin-left: auto; color: var(--text-2); }
  .pl-name { overflow: hidden; text-overflow: ellipsis; max-width: 120px; }

  .sep { height: 1px; background: var(--border); margin: 3px 6px; }
  .hint { padding: 6px 10px; font-size: 0.78rem; color: var(--text-2); }
</style>
