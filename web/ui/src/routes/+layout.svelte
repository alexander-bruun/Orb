<script lang="ts">
  import '../app.css';
  import { onMount } from 'svelte';
  import TopBar from '$lib/components/layout/TopBar.svelte';
  import Sidebar from '$lib/components/layout/Sidebar.svelte';
  import BottomBar from '$lib/components/layout/BottomBar.svelte';
  import ContextMenu from '$lib/components/ui/ContextMenu.svelte';
  import ToastContainer from '$lib/components/ui/ToastContainer.svelte';
  import QueueModal from '$lib/components/ui/QueueModal.svelte';
  import ListenPartyPanel from '$lib/components/layout/ListenPartyPanel.svelte';
  import LyricsModal from '$lib/components/layout/LyricsModal.svelte';
  import { isAuthenticated } from '$lib/stores/auth';
  import { favorites } from '$lib/stores/favorites';
  import { setupRequired } from '$lib/stores/setup';
  import { apiFetch } from '$lib/api/client';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { togglePlayPause, next, previous, currentTrack } from '$lib/stores/player';
  import { themeStore } from '$lib/stores/theme';
  import { isTauri } from '$lib/utils/platform';
  import { getServerUrl } from '$lib/api/base';
  import { loadEQProfiles, getProfileForGenre, applyEQProfile, eqProfiles, genreEQMappings } from '$lib/stores/eq';
  import { library as libraryApi } from '$lib/api/library';
  import { get } from 'svelte/store';

  function onKeydown(e: KeyboardEvent) {
    // Ignore when focus is inside a text field.
    const tag = (e.target as HTMLElement | null)?.tagName ?? '';
    if (tag === 'INPUT' || tag === 'TEXTAREA' || (e.target as HTMLElement | null)?.isContentEditable) return;

    if (e.key === ' ') {
      e.preventDefault();
      togglePlayPause();
    }
  }

  let { children } = $props();
  let dataLoaded = false;

  onMount(async () => {
    themeStore.init();

    // Tauri first-launch: redirect to /connect to configure server URL.
    if (isTauri() && !getServerUrl()) {
      goto('/connect');
      return;
    }

    try {
      const data = await apiFetch<{ setup_required: boolean }>('/auth/setup');
      setupRequired.set(data.setup_required);
    } catch {
      // In Tauri with no configured server URL, the API call hit the local
      // static frontend and returned HTML instead of JSON. Redirect to the
      // server-configuration page rather than showing a broken login.
      if (isTauri() && !getServerUrl()) {
        goto('/connect');
        return;
      }
      // If the check fails, assume setup is done and fall through to login guard.
      setupRequired.set(false);
    }
  });

  $effect(() => {
    if ($setupRequired === null) return;

    const path = $page.url.pathname;

    // Public pages — skip all auth / setup guards.
    if (path.startsWith('/listen/') || path === '/connect' || path.startsWith('/share/')) return;

    // Tauri without a configured server URL — send to /connect first.
    if (isTauri() && !getServerUrl()) {
      goto('/connect');
      return;
    }

    if ($setupRequired) {
      // No users yet — only /setup is accessible.
      if (path !== '/setup') goto('/setup');
    } else {
      // Setup done — /setup is no longer accessible.
      if (path === '/setup') {
        goto($isAuthenticated ? '/' : '/login');
      } else if (path !== '/login' && !$isAuthenticated) {
        // Token expired or logged out — send to login.
        dataLoaded = false;
        goto('/login');
      } else if ($isAuthenticated) {
        if (!dataLoaded) {
          dataLoaded = true;
          favorites.load();
          loadEQProfiles().catch(() => {});
        }
      }
    }
  });

  // ── Per-genre EQ auto-switch ────────────────────────────
  // When the playing track changes, look up genre mappings and apply the
  // first matching EQ profile (falls back to the user's default profile).
  $effect(() => {
    const track = $currentTrack;
    if (!track || !$isAuthenticated) return;

    const mappings = $genreEQMappings;
    if (mappings.length === 0) return; // no genre mappings set → nothing to do

    if (!track.album_id) return;

    libraryApi.album(track.album_id)
      .then((data) => {
        const genres = data.genres ?? [];
        for (const genre of genres) {
          const profile = getProfileForGenre(genre.id);
          if (profile) {
            applyEQProfile(profile);
            return;
          }
        }
        // No genre-specific mapping — fall back to the user's default profile.
        const defaultProfile = get(eqProfiles).find(p => p.is_default) ?? null;
        if (defaultProfile) applyEQProfile(defaultProfile);
      })
      .catch(() => {});
  });
</script>

<svelte:window onkeydown={onKeydown} />

<svelte:head>
  <title>Orb</title>
</svelte:head>

{#if $page.url.pathname.startsWith('/listen/') || $page.url.pathname === '/connect' || $page.url.pathname.startsWith('/share/')}
  <!-- Public page: render without shell or auth guards -->
  {@render children()}
{:else if $setupRequired === null}
  <!-- Checking setup status; render nothing to avoid a flash of wrong content. -->
{:else if $setupRequired && $page.url.pathname === '/setup'}
  {@render children()}
{:else if !$setupRequired && $page.url.pathname === '/login'}
  {@render children()}
{:else if !$setupRequired && $isAuthenticated}
  <div class="shell">
    <TopBar />
    <Sidebar />
    <main class="content">
      {@render children()}
    </main>
    <BottomBar />
  </div>
  <ContextMenu />
  <QueueModal />
  <ListenPartyPanel />
  <LyricsModal />
  <ToastContainer />
{/if}

<style>
  .shell {
    display: grid;
    height: 100dvh;
    grid-template-rows: var(--top-h) 1fr var(--bottom-h);
    grid-template-columns: var(--sidebar-w) 1fr;
    grid-template-areas:
      "top    top"
      "sidebar content"
      "bottom bottom";
    overflow: hidden;
  }
  .content {
    grid-area: content;
    overflow-y: auto;
    background: var(--bg);
    padding: var(--page-padding);
    box-sizing: border-box;
  }
  /* Push grid-area assignments into child components via :global */
  :global(header.topbar)    { grid-area: top; }
  :global(aside.sidebar)    { grid-area: sidebar; }
  :global(footer.bottom-bar) { grid-area: bottom; }

  /* ── Mobile layout (no sidebar column in grid) ─────────────────────────── */
  @media (max-width: 640px) {
    .shell {
      grid-template-rows: var(--top-h) 1fr auto;
      grid-template-columns: 1fr;
      grid-template-areas:
        "top"
        "content"
        "bottom";
    }
  }
</style>
