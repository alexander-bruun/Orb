<script lang="ts">
  import { goto } from '$app/navigation';
  import { authStore, isAuthenticated } from '$lib/stores/auth';
  import { searchQuery } from '$lib/stores/library';
  import { formattedFormat } from '$lib/stores/player';
  import { avatarStore } from '$lib/stores/theme';

  let query = '';
  let menuOpen = false;

  function handleSearch(e: Event) {
    e.preventDefault();
    if (query.trim()) {
      searchQuery.set(query.trim());
      goto('/search');
    }
  }

  function toggleMenu(e: MouseEvent) {
    e.stopPropagation();
    menuOpen = !menuOpen;
  }

  function closeMenu() {
    menuOpen = false;
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

<svelte:window on:click={closeMenu} />

<header class="topbar">
  <a href="/" class="wordmark">orb</a>

  <form class="search-box" on:submit={handleSearch}>
    <svg width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="11" cy="11" r="8"/>
      <path d="m21 21-4.35-4.35"/>
    </svg>
    <input
      type="search"
      placeholder="Search your library..."
      bind:value={query}
      aria-label="Search library"
    />
  </form>

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
    font-family: 'Instrument Serif', serif;
    font-style: italic;
    font-size: 22px;
    color: var(--accent);
    letter-spacing: -0.02em;
    flex-shrink: 0;
    margin-right: 4px;
  }

  .search-box {
    flex: 1;
    max-width: 380px;
    height: 32px;
    background: var(--surface-2);
    border: 1px solid var(--border-2);
    border-radius: 8px;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 12px;
    color: var(--muted);
    transition: border-color 0.15s;
  }
  .search-box:focus-within { border-color: var(--accent); }
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
</style>
