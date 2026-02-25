<script lang="ts">
  import { page } from '$app/stores';
  import { currentTrack } from '$lib/stores/player';
  import { expanded } from './coverExpandStore';

  function toggleExpand() {
    expanded.update(v => !v);
  }

  const BASE = import.meta.env.VITE_API_BASE ?? '/api';
</script>

<aside class="sidebar">
  <nav class="nav">
    <a href="/" class:active={$page.url.pathname === '/'}>Home</a>
    <a href="/library" class:active={$page.url.pathname.startsWith('/library')}>Library</a>
    <a href="/playlists" class:active={$page.url.pathname.startsWith('/playlists')}>Playlists</a>
    <a href="/search" class:active={$page.url.pathname === '/search'}>Search</a>
  </nav>

  <div class="spacer"></div>

  {#if $currentTrack && $expanded}
    <div class="sidebar-bottom">
      <div class="cover-wrap">
        {#if $currentTrack.album_id}
          <img src="{BASE}/covers/{$currentTrack.album_id}"
               alt="album art"
               class="full-image" />
        {:else}
          <div class="placeholder full-image"></div>
        {/if}
        <button class="expand-btn overlay" on:click={toggleExpand} aria-label="Shrink cover">
          <svg width="20" height="20" viewBox="0 0 20 20"><path d="M6 14h8v-8H6v8zm2-6h4v4H8v-4z" fill="currentColor"/></svg>
        </button>
      </div>
    </div>
  {/if}
</aside>

<style>
  .sidebar {
    width: var(--sidebar-w);
    flex-shrink: 0;
    background: var(--bg-elevated);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    padding: 16px 0;
    overflow-y: auto;
  }
  .nav { display: flex; flex-direction: column; gap: 2px; padding: 0 12px; }
  .nav a {
    padding: 8px 12px;
    border-radius: 6px;
    color: var(--text-muted);
    text-decoration: none;
    font-size: 0.875rem;
    transition: color 0.15s, background 0.15s;
  }
  .nav a:hover { color: var(--text); background: var(--bg-hover); }
  .nav a.active { color: var(--text); background: var(--bg-hover); }
  .spacer { flex: 1; }
  .sidebar-bottom {
    padding: 12px;
    border-top: 1px solid var(--border);
  }
  .now-playing-cover { width: 100%; }
  .cover-wrap { position: relative; }
  .full-image {
    width: 100%;
    aspect-ratio: 1;
    border-radius: 8px;
    object-fit: contain;
    display: block;
    background: var(--bg-hover);
  }
  .placeholder {
    background: var(--bg-hover);
    border-radius: 8px;
  }
  .expand-btn {
    background: rgba(0,0,0,0.5);
    border: none;
    border-radius: 50%;
    color: #fff;
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: background 0.15s;
  }
  .expand-btn:hover { background: rgba(0,0,0,0.7); }
  .expand-btn.overlay {
    position: absolute;
    top: 8px;
    right: 8px;
  }
</style>
