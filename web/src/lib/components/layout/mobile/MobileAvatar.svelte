<script lang="ts">
  import { goto } from '$app/navigation';
  import { authStore } from '$lib/stores/auth';
  import { avatarStore } from '$lib/stores/settings/theme';

  let menuOpen = false;

  function toggleMenu(e: MouseEvent) {
    e.stopPropagation();
    menuOpen = !menuOpen;
  }

  function closeMenu() {
    menuOpen = false;
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

<svelte:window on:click={closeMenu} />

<div class="mobile-avatar-wrap">
  <button class="avatar" on:click={toggleMenu} aria-label="User menu">
    {#if $avatarStore}
      <img src={$avatarStore} alt="avatar" class="avatar-img" />
    {:else}
      {($authStore.user?.username ?? 'U').slice(0, 2).toUpperCase()}
    {/if}
  </button>

  {#if menuOpen}
    <div class="menu" on:click|stopPropagation role="menu">
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

<style>
  /* Only visible on mobile */
  .mobile-avatar-wrap {
    display: none;
  }

  @media (max-width: 640px) {
    .mobile-avatar-wrap {
      display: block;
      position: fixed;
      top: calc(env(safe-area-inset-top, 0px) + 10px);
      right: 14px;
      z-index: 45;
    }

    .avatar {
      width: 30px;
      height: 30px;
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
      box-shadow: 0 2px 8px rgba(0,0,0,0.4);
      -webkit-tap-highlight-color: transparent;
    }

    .avatar-img {
      width: 100%;
      height: 100%;
      object-fit: cover;
      border-radius: 50%;
      display: block;
    }

    .menu {
      position: absolute;
      top: calc(100% + 8px);
      right: 0;
      min-width: 200px;
      background: var(--surface);
      border: 1px solid var(--border-2);
      border-radius: 10px;
      box-shadow: 0 8px 32px rgba(0,0,0,0.5);
      overflow: hidden;
      z-index: 46;
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
      -webkit-tap-highlight-color: transparent;
    }
    .menu-item:hover, .menu-item:active {
      background: var(--surface-2);
      color: var(--text);
    }

    .menu-item--danger:hover, .menu-item--danger:active {
      color: #f87171;
      background: rgba(248,113,113,0.08);
    }
  }
</style>
