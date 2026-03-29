<script lang="ts">
  import { contextMenu, closeContextMenu } from '$lib/stores/ui/contextMenu';
  import { playTrack, playNext, addToQueue } from '$lib/stores/player';
  import { audioEngine } from '$lib/audio/engine';
  import { playlists as playlistsApi } from '$lib/api/playlists';
  import { recommend } from '$lib/api/recommend';
  import { share as shareApi } from '$lib/api/share';
  import { favorites } from '$lib/stores/library/favorites';
  import { downloads, downloadTrack, deleteDownload } from '$lib/stores/offline/downloads';
  import type { Playlist } from '$lib/types';
  import Spinner from '$lib/components/ui/Spinner.svelte';

  let showPlaylists = false;
  let playlists: Playlist[] = [];
  let loadingPlaylists = false;
  let addedId: string | null = null;
  let sharingTrack = false;
  let sharingAlbum = false;

  $: if (!$contextMenu.visible) {
    showPlaylists = false;
    playlists = [];
    addedId = null;
    sharingTrack = false;
    sharingAlbum = false;
  }

  $: isFav = $contextMenu.track ? $favorites.has($contextMenu.track.id) : false;
  $: dlEntry = $contextMenu.track ? $downloads.get($contextMenu.track.id) : undefined;

  async function handleFavorite() {
    const t = $contextMenu.track;
    if (!t) return;
    await favorites.toggle(t.id, t);
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

  async function handleStartRadio() {
    const t = $contextMenu.track;
    if (!t) return;
    // Prime the AudioContext synchronously while still inside the user gesture.
    // Without this, the async network fetch below breaks the browser's gesture
    // activation window and audio is silently blocked.
    audioEngine.prime(t.sample_rate);
    closeContextMenu();
    try {
      const similar = await recommend.similar(t.id, 30, t.album_id);
      const tracks = similar ?? [];
      await playTrack(t, [t, ...tracks]);
    } catch {
      await playTrack(t, [t]);
    }
  }

  async function handleShare() {
    const t = $contextMenu.track;
    if (!t) return;
    sharingTrack = true;
    try {
      const resp = await shareApi.create('track', t.id);
      await navigator.clipboard?.writeText(`${location.origin}/share/${resp.token}`);
    } catch {
      // fallback: nothing (error silently)
    } finally {
      sharingTrack = false;
    }
    closeContextMenu();
  }

  async function handleShareAlbum() {
    const t = $contextMenu.track;
    if (!t?.album_id) return;
    sharingAlbum = true;
    try {
      const resp = await shareApi.create('album', t.album_id);
      await navigator.clipboard?.writeText(`${location.origin}/share/${resp.token}`);
    } catch {
      // fallback: nothing
    } finally {
      sharingAlbum = false;
    }
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

  function onTouchStart(e: TouchEvent) {
    e.stopPropagation();
  }

  function onTouchMove(e: TouchEvent) {
    e.stopPropagation();
  }

  function onTouchEnd(e: TouchEvent) {
    e.stopPropagation();
  }

  // Clamp position so the menu stays within the viewport
  $: style = (() => {
    const menuW = 172;
    const menuH = showPlaylists ? 280 : 260;
    const vw = typeof window !== 'undefined' ? window.innerWidth : 1920;
    const vh = typeof window !== 'undefined' ? window.innerHeight : 1080;
    const left = Math.min($contextMenu.x, vw - menuW - 8);
    const top = Math.min($contextMenu.y, vh - menuH - 8);
    return `left:${left}px;top:${top}px`;
  })();
</script>

<svelte:window on:pointerdown={onWindowPointerDown} on:keydown={onKeydown} />

{#if $contextMenu.visible && $contextMenu.track}
  
  
  <div class="ctx-menu" style={style} role="menu" tabindex="-1" on:click|stopPropagation={() => {}} on:keydown|stopPropagation={() => {}} on:touchstart={onTouchStart} on:touchmove={onTouchMove} on:touchend={onTouchEnd}>
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

      {#if dlEntry?.status === 'done'}
        <button class="item" role="menuitem"
          on:click={() => { if ($contextMenu.track) { deleteDownload($contextMenu.track.id); closeContextMenu(); } }}>
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <polyline points="3,6 5,6 21,6"/>
            <path d="M19,6l-1,14a2,2,0,0,1-2,2H8a2,2,0,0,1-2-2L5,6"/>
            <path d="M10,11v6m4-6v6"/>
          </svg>
          Remove download
        </button>
      {:else if dlEntry?.status === 'downloading'}
        <button class="item" role="menuitem" disabled>
          <svg class="spin" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true">
            <circle cx="12" cy="12" r="9" stroke-dasharray="44 13"/>
          </svg>
          Downloading {dlEntry.progress}%…
        </button>
      {:else}
        <button class="item" role="menuitem"
          on:click={() => { if ($contextMenu.track) { downloadTrack($contextMenu.track); closeContextMenu(); } }}>
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
            <polyline points="7,10 12,15 17,10"/>
            <line x1="12" y1="15" x2="12" y2="3"/>
          </svg>
          Download offline
        </button>
      {/if}

      <button class="item" on:click={handleStartRadio} role="menuitem">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M2 12a10 10 0 1 0 10-10"/>
          <polyline points="12 8 12 12 14 14"/>
          <polyline points="2 8 2 2 8 2"/>
        </svg>
        Start Radio
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
      <button class="item" on:click={handleShare} disabled={sharingTrack} role="menuitem">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <circle cx="18" cy="5" r="3"/>
          <circle cx="6" cy="12" r="3"/>
          <circle cx="18" cy="19" r="3"/>
          <line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/>
          <line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/>
        </svg>
        {sharingTrack ? 'Copying…' : 'Share Track'}
      </button>
      {#if $contextMenu.track?.album_id}
        <button class="item" on:click={handleShareAlbum} disabled={sharingAlbum} role="menuitem">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <circle cx="18" cy="5" r="3"/>
            <circle cx="6" cy="12" r="3"/>
            <circle cx="18" cy="19" r="3"/>
            <line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/>
            <line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/>
          </svg>
          {sharingAlbum ? 'Copying…' : 'Share Album'}
        </button>
      {/if}
    {:else}
      <button class="item dim" on:click={() => (showPlaylists = false)} role="menuitem">
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
          <polyline points="15,18 9,12 15,6"/>
        </svg>
        Add to Playlist
      </button>
      <div class="sep"></div>
      {#if loadingPlaylists}
        <div class="hint"><Spinner size={14} /></div>
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

  @keyframes spin { to { transform: rotate(360deg); } }
  .spin { animation: spin 1s linear infinite; }
</style>
