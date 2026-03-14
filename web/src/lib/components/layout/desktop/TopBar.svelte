<script lang="ts">
  import { goto } from '$app/navigation';
  import { authStore, isAuthenticated } from '$lib/stores/auth';
  import { searchQuery } from '$lib/stores/library';
  import { formattedFormat } from '$lib/stores/player';
  import { avatarStore } from '$lib/stores/settings/theme';
  import { sidebarOpen } from '$lib/stores/ui/sidebar';
  import { library as libApi } from '$lib/api/library';
  import { getApiBase } from '$lib/api/base';
  import type { Track, Album, Artist } from '$lib/types';

  let query = '';
  let menuOpen = false;
  let searchFocused = false;
  let quickResults: { tracks: Track[]; albums: Album[]; artists: Artist[] } | null = null;
  let searchDebounce: ReturnType<typeof setTimeout> | null = null;
  let searchLoading = false;
  let searchEl: HTMLInputElement;

  $: dropdownVisible = searchFocused && query.trim().length > 0 && quickResults !== null;
  $: hasQuickResults = quickResults && (
    quickResults.tracks.length > 0 ||
    quickResults.albums.length > 0 ||
    quickResults.artists.length > 0
  );

  function handleSearchInput() {
    if (searchDebounce) clearTimeout(searchDebounce);
    if (!query.trim()) {
      quickResults = null;
      searchLoading = false;
      return;
    }
    searchLoading = true;
    searchDebounce = setTimeout(async () => {
      try {
        const res = await libApi.search(query.trim(), { types: ['tracks', 'albums', 'artists'] });
        quickResults = res;
      } finally {
        searchLoading = false;
      }
    }, 250);
  }

  function handleSearchKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      searchFocused = false;
      searchEl?.blur();
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      goSearchAll();
    }
  }

  function goSearchAll() {
    if (!query.trim()) return;
    searchQuery.set(query.trim());
    searchFocused = false;
    goto('/search');
  }

  function goTrack(t: Track) {
    searchFocused = false;
    // play track — for now just navigate to album
    if (t.album_id) goto(`/library/albums/${t.album_id}`);
  }

  function goAlbum(a: Album) {
    searchFocused = false;
    goto(`/library/albums/${a.id}`);
  }

  function goArtist(a: Artist) {
    searchFocused = false;
    goto(`/artists/${a.id}`);
  }

  function formatDuration(ms: number): string {
    const s = Math.floor(ms / 1000);
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
  }

  function toggleMenu(e: MouseEvent) {
    e.stopPropagation();
    menuOpen = !menuOpen;
  }

  function closeAll() {
    menuOpen = false;
    searchFocused = false;
  }

  function goAdmin(e: MouseEvent) {
    e.stopPropagation();
    menuOpen = false;
    goto('/admin');
  }

  function goSettings(e: MouseEvent) {
    e.stopPropagation();
    menuOpen = false;
    goto('/settings');
  }

  function doLogout(e: MouseEvent) {
    e.stopPropagation();
    menuOpen = false;
    authStore.logout();
  }
</script>

<svelte:window on:click={closeAll} />

<header class="topbar">
  <!-- Hamburger: only visible on mobile -->
  <button class="hamburger" on:click|stopPropagation={() => sidebarOpen.update(v => !v)} aria-label="Toggle navigation">
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <line x1="3" y1="6" x2="21" y2="6"/>
      <line x1="3" y1="12" x2="21" y2="12"/>
      <line x1="3" y1="18" x2="21" y2="18"/>
    </svg>
  </button>

  <a href="/" class="wordmark" aria-label="Orb">
    <svg viewBox="0 0 52 28" height="30" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <circle cx="12" cy="14" r="10" stroke="currentColor" stroke-width="1.4" opacity="0.3"/>
      <circle cx="12" cy="14" r="4.5" fill="currentColor"/>
      <text x="27" y="21" font-family="'Instrument Serif', Georgia, serif" font-style="italic" font-size="22" fill="currentColor" letter-spacing="-0.02em">orb</text>
    </svg>
  </a>

  <!-- Quick search -->
  <div class="search-wrap" on:click|stopPropagation>
    <div class="search-box" class:focused={searchFocused}>
      <svg width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true">
        <circle cx="11" cy="11" r="8"/>
        <path d="m21 21-4.35-4.35"/>
      </svg>
      <input
        bind:this={searchEl}
        type="search"
        placeholder="Quick search…"
        bind:value={query}
        on:input={handleSearchInput}
        on:focus={() => { searchFocused = true; }}
        on:keydown={handleSearchKeydown}
        aria-label="Quick search"
        aria-autocomplete="list"
        aria-expanded={dropdownVisible}
      />
      {#if query && !searchLoading}
        <button class="clear-x" on:click|stopPropagation={() => { query = ''; quickResults = null; searchEl?.focus(); }} aria-label="Clear search">×</button>
      {/if}
    </div>

    {#if dropdownVisible}
      <div class="quick-dropdown" role="listbox">
        {#if !hasQuickResults}
          <div class="qd-empty">No results for "{query}"</div>
        {:else}
          {#if quickResults && quickResults.artists.length > 0}
            <div class="qd-section-label">Artists</div>
            {#each quickResults.artists.slice(0, 3) as artist (artist.id)}
              <button class="qd-row" on:click={() => goArtist(artist)} role="option" aria-selected="false">
                <div class="qd-thumb qd-thumb--artist">
                  {#if artist.image_key}
                    <img src="{getApiBase()}/objects/{artist.image_key}" alt={artist.name} />
                  {:else}
                    <svg width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24"><circle cx="12" cy="8" r="4"/><path d="M4 20c0-4 3.6-7 8-7s8 3 8 7"/></svg>
                  {/if}
                </div>
                <span class="qd-primary">{artist.name}</span>
                <span class="qd-meta">Artist</span>
              </button>
            {/each}
          {/if}

          {#if quickResults && quickResults.albums.length > 0}
            <div class="qd-section-label">Albums</div>
            {#each quickResults.albums.slice(0, 3) as album (album.id)}
              <button class="qd-row" on:click={() => goAlbum(album)} role="option" aria-selected="false">
                <div class="qd-thumb">
                  {#if album.cover_art_key}
                    <img src="{getApiBase()}/covers/{album.id}" alt={album.title} />
                  {:else}
                    <svg width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="12" cy="12" r="3"/></svg>
                  {/if}
                </div>
                <div class="qd-text">
                  <span class="qd-primary">{album.title}</span>
                  {#if album.artist_name}<span class="qd-sub">{album.artist_name}</span>{/if}
                </div>
                {#if album.release_year}<span class="qd-meta">{album.release_year}</span>{/if}
              </button>
            {/each}
          {/if}

          {#if quickResults && quickResults.tracks.length > 0}
            <div class="qd-section-label">Tracks</div>
            {#each quickResults.tracks.slice(0, 5) as track (track.id)}
              <button class="qd-row" on:click={() => goTrack(track)} role="option" aria-selected="false">
                <div class="qd-thumb">
                  {#if track.cover_art_key}
                    <img src="{getApiBase()}/covers/{track.album_id}" alt={track.title} />
                  {:else}
                    <svg width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>
                  {/if}
                </div>
                <div class="qd-text">
                  <span class="qd-primary">{track.title}</span>
                  {#if track.artist_name}<span class="qd-sub">{track.artist_name}</span>{/if}
                </div>
                <span class="qd-meta">{formatDuration(track.duration_ms)}</span>
              </button>
            {/each}
          {/if}
        {/if}

        <button class="qd-view-all" on:click={goSearchAll}>
          <svg width="11" height="11" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/></svg>
          Advanced search &amp; filters
          <svg width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M5 12h14M12 5l7 7-7 7"/></svg>
        </button>
      </div>
    {/if}
  </div>

  <div class="spacer"></div>

  {#if $formattedFormat}
    <div class="format-badge">{$formattedFormat}</div>
  {/if}

  {#if $isAuthenticated}
    <div class="avatar-wrap">
      <button class="avatar" on:click={toggleMenu} aria-label="User menu" title={$authStore.user?.username ?? 'User'}>
        {#if $avatarStore}
          <img src={$avatarStore} alt="avatar" class="avatar-img" />
        {:else}
          {($authStore.user?.username ?? 'U').slice(0, 2).toUpperCase()}
        {/if}
      </button>

      {#if menuOpen}
        <div class="menu" on:click|stopPropagation>
          <div class="menu-user">
            <div class="menu-avatar">
              {#if $avatarStore}
                <img src={$avatarStore} alt="avatar" class="avatar-img" />
              {:else}
                {($authStore.user?.username ?? 'U').slice(0, 2).toUpperCase()}
              {/if}
            </div>
            <div class="menu-info">
              <span class="menu-name">{$authStore.user?.username ?? 'User'}</span>
              <span class="menu-email">{$authStore.user?.email ?? ''}</span>
            </div>
          </div>
          {#if $authStore.user?.is_admin}
            <div class="menu-divider"></div>
            <button class="menu-item" on:click={goAdmin}>
              <svg width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                <path d="M12 2L2 7l10 5 10-5-10-5z"/>
                <path d="M2 17l10 5 10-5"/>
                <path d="M2 12l10 5 10-5"/>
              </svg>
              Admin
            </button>
          {/if}
          <div class="menu-divider"></div>
          <button class="menu-item" on:click={goSettings}>
            <svg width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
              <circle cx="12" cy="12" r="3"/>
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
            </svg>
            Settings
          </button>
          <div class="menu-divider"></div>
          <button class="menu-item menu-item--danger" on:click={doLogout}>
            <svg width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
              <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
              <polyline points="16 17 21 12 16 7"/>
              <line x1="21" y1="12" x2="9" y2="12"/>
            </svg>
            Sign out
          </button>
        </div>
      {/if}
    </div>
  {/if}

</header>

<style>
  .topbar {
    display: flex;
    align-items: center;
    padding: 0 20px;
    gap: 16px;
    border-bottom: 1px solid var(--border);
    background: rgba(8,8,9,0.95);
    backdrop-filter: blur(12px);
    z-index: 20;
  }

  :global([data-theme="light"]) .topbar {
    background: rgba(240,240,245,0.95);
  }

  .wordmark {
    color: var(--accent);
    flex-shrink: 0;
    margin-right: 4px;
    display: flex;
    align-items: center;
  }

  /* ── Quick search ───────────────────────────────────────── */
  .search-wrap {
    position: relative;
    flex: 1;
    max-width: 380px;
  }

  .search-box {
    height: 32px;
    background: var(--surface-2);
    border: 1px solid var(--border-2);
    border-radius: 8px;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 10px 0 12px;
    color: var(--muted);
    transition: border-color 0.15s;
  }
  .search-box.focused { border-color: var(--accent); }
  .search-box input {
    background: none;
    border: none;
    outline: none;
    color: var(--text);
    font-size: 12px;
    font-family: 'DM Mono', monospace;
    width: 100%;
  }
  .search-box input::placeholder { color: var(--muted); }

  .clear-x {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 14px;
    line-height: 1;
    padding: 0 2px;
    flex-shrink: 0;
    opacity: 0.6;
  }
  .clear-x:hover { opacity: 1; }

  /* ── Quick dropdown ─────────────────────────────────────── */
  .quick-dropdown {
    position: absolute;
    top: calc(100% + 6px);
    left: 0;
    right: 0;
    background: var(--surface);
    border: 1px solid var(--border-2);
    border-radius: 10px;
    box-shadow: 0 12px 40px rgba(0,0,0,0.45);
    overflow: hidden;
    z-index: 200;
    animation: dropdown-in 0.1s ease;
    min-width: 300px;
  }

  @keyframes dropdown-in {
    from { opacity: 0; transform: translateY(-4px); }
    to   { opacity: 1; transform: translateY(0); }
  }

  .qd-section-label {
    font-size: 0.6rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--text-muted);
    padding: 8px 12px 4px;
  }

  .qd-row {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 6px 12px;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
  }
  .qd-row:hover { background: var(--surface-2); }

  .qd-thumb {
    width: 30px;
    height: 30px;
    border-radius: 4px;
    background: var(--surface-2);
    flex-shrink: 0;
    overflow: hidden;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
  }
  .qd-thumb--artist { border-radius: 50%; }
  .qd-thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .qd-text {
    display: flex;
    flex-direction: column;
    gap: 1px;
    flex: 1;
    overflow: hidden;
  }

  .qd-primary {
    font-size: 12px;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
    font-family: 'Syne', sans-serif;
  }

  .qd-sub {
    font-size: 10px;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-family: 'DM Mono', monospace;
  }

  .qd-meta {
    font-size: 10px;
    color: var(--text-muted);
    font-family: 'DM Mono', monospace;
    flex-shrink: 0;
    margin-left: auto;
  }

  .qd-empty {
    padding: 16px 12px;
    font-size: 12px;
    color: var(--text-muted);
  }

  .qd-view-all {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 10px 12px;
    background: none;
    border: none;
    border-top: 1px solid var(--border);
    color: var(--accent);
    font-size: 11px;
    font-family: 'DM Mono', monospace;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
    margin-top: 4px;
  }
  .qd-view-all:hover { background: var(--accent-dim); }
  .qd-view-all svg:last-child { margin-left: auto; }

  .spacer { flex: 1; }

  .format-badge {
    font-family: 'DM Mono', monospace;
    font-size: 10px;
    letter-spacing: 0.08em;
    color: var(--accent);
    background: var(--accent-dim);
    border: 1px solid var(--accent-glow);
    border-radius: 4px;
    padding: 3px 8px;
  }

  /* ── Avatar / user menu ─────────────────────────────────── */
  .avatar-wrap {
    position: relative;
    flex-shrink: 0;
  }

  .avatar {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: linear-gradient(135deg, var(--accent), #818cf8);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 11px;
    font-weight: 700;
    color: white;
    cursor: pointer;
    border: none;
    font-family: 'Syne', sans-serif;
    overflow: hidden;
    transition: box-shadow 0.15s;
  }
  .avatar:hover { box-shadow: 0 0 0 2px var(--accent-glow); }

  .avatar-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    border-radius: 50%;
    display: block;
  }

  .menu {
    position: absolute;
    top: calc(100% + 10px);
    right: 0;
    min-width: 200px;
    background: var(--surface);
    border: 1px solid var(--border-2);
    border-radius: 10px;
    box-shadow: 0 8px 32px rgba(0,0,0,0.4);
    overflow: hidden;
    z-index: 100;
    animation: menu-in 0.12s ease;
  }

  @keyframes menu-in {
    from { opacity: 0; transform: translateY(-6px) scale(0.97); }
    to   { opacity: 1; transform: translateY(0) scale(1); }
  }

  .menu-user {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 14px 14px 12px;
  }

  .menu-avatar {
    width: 34px;
    height: 34px;
    border-radius: 50%;
    background: linear-gradient(135deg, var(--accent), #818cf8);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 13px;
    font-weight: 700;
    color: white;
    flex-shrink: 0;
    overflow: hidden;
    font-family: 'Syne', sans-serif;
  }

  .menu-info {
    display: flex;
    flex-direction: column;
    gap: 1px;
    overflow: hidden;
  }

  .menu-name {
    font-size: 13px;
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .menu-email {
    font-size: 11px;
    color: var(--text-2);
    font-family: 'DM Mono', monospace;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .menu-divider {
    height: 1px;
    background: var(--border);
    margin: 0;
  }

  .menu-item {
    display: flex;
    align-items: center;
    gap: 9px;
    width: 100%;
    padding: 10px 14px;
    background: none;
    border: none;
    color: var(--text-2);
    font-size: 13px;
    font-family: 'Syne', sans-serif;
    cursor: pointer;
    text-align: left;
    transition: background 0.12s, color 0.12s;
  }
  .menu-item:hover {
    background: var(--surface-2);
    color: var(--text);
  }

  .menu-item--danger:hover {
    color: #f87171;
    background: rgba(248,113,113,0.08);
  }

  /* ── Hamburger (mobile only) ────────────────────────────── */
  .hamburger {
    display: none;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    color: var(--text-2);
    cursor: pointer;
    padding: 4px;
    border-radius: 6px;
    flex-shrink: 0;
    transition: color 0.15s, background 0.15s;
  }
  .hamburger:hover { color: var(--text); background: var(--surface-2); }

  @media (max-width: 640px) {
    .hamburger { display: flex; }
    .format-badge { display: none; }
    .search-wrap { max-width: none; }
    .topbar { padding: 0 12px; gap: 10px; }
    .quick-dropdown { min-width: unset; }
  }
</style>
