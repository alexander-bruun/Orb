<script lang="ts">
  import { page } from "$app/stores";
</script>

<nav class="mobile-nav" aria-label="Main navigation">
  <!-- Home -->
  <a href="/" class="nav-item" class:active={$page.url.pathname === "/"}>
    <svg
      width="22"
      height="22"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <path d="M3 9.5L12 3l9 6.5V20a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V9.5z" />
      <polyline points="9 21 9 12 15 12 15 21" />
    </svg>
    <span>Home</span>
  </a>

  <!-- Search -->
  <a
    href="/search"
    class="nav-item"
    class:active={$page.url.pathname === "/search"}
  >
    <svg
      width="22"
      height="22"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <circle cx="11" cy="11" r="8" />
      <path d="m21 21-4.35-4.35" />
    </svg>
    <span>Search</span>
  </a>

  <!-- Music -->
  <a
    href="/library"
    class="nav-item"
    class:active={$page.url.pathname.startsWith("/library") ||
      $page.url.pathname.startsWith("/artists") ||
      $page.url.pathname.startsWith("/genres")}
  >
    <svg
      width="22"
      height="22"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <path d="M9 18V5l12-2v13" />
      <circle cx="6" cy="18" r="3" />
      <circle cx="18" cy="16" r="3" />
    </svg>
    <span>Music</span>
  </a>

  <!-- Audiobooks -->
  <a
    href="/audiobooks"
    class="nav-item"
    class:active={$page.url.pathname.startsWith("/audiobooks")}
  >
    <svg
      width="22"
      height="22"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <path d="M3 18v-6a9 9 0 0 1 18 0v6" />
      <path d="M21 19a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3z" />
      <path d="M3 19a2 2 0 0 0 2 2h1a2 2 0 0 0 2-2v-3a2 2 0 0 0-2-2H3z" />
    </svg>
    <span>Audiobooks</span>
  </a>

  <!-- Podcasts -->
  <a
    href="/podcasts"
    class="nav-item"
    class:active={$page.url.pathname.startsWith("/podcasts")}
  >
    <svg
      width="22"
      height="22"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
      <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
      <line x1="12" y1="19" x2="12" y2="23" />
      <line x1="8" y1="23" x2="16" y2="23" />
    </svg>
    <span>Podcasts</span>
  </a>

  <!-- Playlists -->
  <a
    href="/playlists"
    class="nav-item"
    class:active={$page.url.pathname.startsWith("/playlists")}
  >
    <svg
      width="22"
      height="22"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <line x1="8" y1="6" x2="21" y2="6" />
      <line x1="8" y1="12" x2="21" y2="12" />
      <line x1="8" y1="18" x2="21" y2="18" />
      <line x1="3" y1="6" x2="3.01" y2="6" />
      <line x1="3" y1="12" x2="3.01" y2="12" />
      <line x1="3" y1="18" x2="3.01" y2="18" />
    </svg>
    <span>Playlists</span>
  </a>

  <!-- Favorites -->
  <a
    href="/favorites"
    class="nav-item"
    class:active={$page.url.pathname === "/favorites"}
  >
    <svg
      width="22"
      height="22"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <path
        d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"
      />
    </svg>
    <span>Favorites</span>
  </a>
</nav>

<style>
  .mobile-nav {
    display: none; /* desktop: hidden */
  }

  @media (max-width: 640px) {
    .mobile-nav {
      display: flex;
      position: fixed;
      bottom: 0;
      left: 0;
      right: 0;
      height: calc(60px + env(safe-area-inset-bottom));
      padding-bottom: env(safe-area-inset-bottom);
      background: var(--bg-elevated);
      border-top: 1px solid var(--border);
      z-index: 40;
      align-items: stretch;
    }

    .nav-item {
      flex: 1;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: 3px;
      color: var(--text-muted);
      text-decoration: none;
      font-size: 10px;
      font-weight: 500;
      letter-spacing: 0.02em;
      transition: color 0.15s;
      padding: 6px 0;
      -webkit-tap-highlight-color: transparent;
    }

    .nav-item.active {
      color: var(--accent);
    }

    .nav-item span {
      line-height: 1;
    }
  }
</style>
