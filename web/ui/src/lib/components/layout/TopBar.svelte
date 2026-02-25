<script lang="ts">
  import { goto } from '$app/navigation';
  import { authStore, isAuthenticated } from '$lib/stores/auth';
  import { searchQuery } from '$lib/stores/library';
  import { formattedFormat } from '$lib/stores/player';

  let query = '';

  function handleSearch(e: Event) {
    e.preventDefault();
    if (query.trim()) {
      searchQuery.set(query.trim());
      goto('/search');
    }
  }
</script>

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
    <div class="avatar" title={$authStore.user?.username ?? 'User'}>
      {($authStore.user?.username ?? 'U').slice(0, 2).toUpperCase()}
    </div>
    <button class="icon-btn" on:click={() => authStore.logout()} aria-label="Sign out" title="Sign out">
      <svg width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
        <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
        <polyline points="16 17 21 12 16 7"/>
        <line x1="21" y1="12" x2="9" y2="12"/>
      </svg>
    </button>
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

  .avatar {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: linear-gradient(135deg, #c084fc, #818cf8);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 11px;
    font-weight: 700;
    color: white;
    cursor: default;
    flex-shrink: 0;
    font-family: 'Syne', sans-serif;
  }

  .icon-btn {
    width: 30px;
    height: 30px;
    border-radius: 7px;
    border: 1px solid var(--border-2);
    background: transparent;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--muted);
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s;
    flex-shrink: 0;
  }
  .icon-btn:hover { color: var(--text); border-color: var(--accent); }
</style>
