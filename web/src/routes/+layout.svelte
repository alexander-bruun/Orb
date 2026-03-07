<script lang="ts">
  import "../app.css";
  import { onMount } from "svelte";
  import TopBar from "$lib/components/layout/desktop/TopBar.svelte";
  import Sidebar from "$lib/components/layout/desktop/Sidebar.svelte";
  import BottomBar from "$lib/components/layout/desktop/BottomBar.svelte";
  import ContextMenu from "$lib/components/ui/ContextMenu.svelte";
  import ToastContainer from "$lib/components/ui/ToastContainer.svelte";
  import QueueModal from "$lib/components/ui/QueueModal.svelte";
  import ListenPartyPanel from "$lib/components/layout/shared/ListenPartyPanel.svelte";
  import LyricsModal from "$lib/components/layout/shared/LyricsModal.svelte";
  import { isAuthenticated } from "$lib/stores/auth";
  import { favorites } from "$lib/stores/library/favorites";
  import { setupRequired } from "$lib/stores/auth/setup";
  import { apiFetch } from "$lib/api/client";
  import { page } from "$app/stores";
  import { goto } from "$app/navigation";
  import {
    togglePlayPause,
    next,
    previous,
    currentTrack,
    toggleRepeat,
    toggleShuffle,
    queueModalOpen,
    volume,
    setVolume,
  } from "$lib/stores/player";
  import { themeStore } from "$lib/stores/settings/theme";
  import { isTauri, isNative } from "$lib/utils/platform";
  import TitleBar from "$lib/components/layout/tauri/TitleBar.svelte";
  import MobileNav from "$lib/components/layout/mobile/MobileNav.svelte";
  import MobilePlayer from "$lib/components/layout/mobile/MobilePlayer.svelte";
  import MobileAvatar from "$lib/components/layout/mobile/MobileAvatar.svelte";
  import { getServerUrl } from "$lib/api/base";
  import {
    loadEQProfiles,
    getProfileForGenre,
    applyEQProfile,
    eqProfiles,
    genreEQMappings,
  } from "$lib/stores/settings/eq";
  import { library as libraryApi } from "$lib/api/library";
  import { get } from "svelte/store";
  import { restoreDownloads, downloads } from "$lib/stores/offline/downloads";
  import {
    isOffline,
    startConnectivityMonitor,
    checkConnectivity,
  } from "$lib/stores/offline/connectivity";
  import { lpPanelOpen, lpRole } from "$lib/stores/social/listenParty";
  import { lyricsOpen } from "$lib/stores/player/lyrics";
  import KeyboardShortcuts from "$lib/components/ui/KeyboardShortcuts.svelte";

  // ── Keyboard shortcuts ────────────────────────────────────────────────────
  let shortcutsOpen = $state(false);
  let premuteVolume = 1;

  const SHORTCUTS: { key: string; label: string; description: string; action: () => void }[] = [
    { key: " ", label: "Space", description: "Play / Pause", action: togglePlayPause },
    { key: "n", label: "N", description: "Next track", action: () => next() },
    { key: "b", label: "B", description: "Previous track", action: () => previous() },
    {
      key: "m",
      label: "M",
      description: "Mute / Unmute",
      action: () => {
        const v = get(volume);
        if (v > 0) { premuteVolume = v; setVolume(0); }
        else setVolume(premuteVolume || 1);
      },
    },
    { key: "r", label: "R", description: "Cycle repeat mode", action: toggleRepeat },
    { key: "s", label: "S", description: "Toggle shuffle", action: toggleShuffle },
    { key: "q", label: "Q", description: "Toggle queue panel", action: () => queueModalOpen.update((v) => !v) },
    { key: "l", label: "L", description: "Toggle lyrics", action: () => lyricsOpen.update((v) => !v) },
    { key: "?", label: "?", description: "Show keyboard shortcuts", action: () => { shortcutsOpen = !shortcutsOpen; } },
  ];

  function onKeydown(e: KeyboardEvent) {
    // Ignore when focus is inside a text field.
    const tag = (e.target as HTMLElement | null)?.tagName ?? "";
    if (
      tag === "INPUT" ||
      tag === "TEXTAREA" ||
      (e.target as HTMLElement | null)?.isContentEditable
    )
      return;

    // Ignore when modifier keys are held — allow browser/OS shortcuts (e.g. Ctrl+R).
    if (e.ctrlKey || e.metaKey || e.altKey) return;

    for (const sc of SHORTCUTS) {
      if (e.key === sc.key) {
        e.preventDefault();
        sc.action();
        return;
      }
    }
  }

  let { children } = $props();
  let dataLoaded = false;

  onMount(async () => {
    themeStore.init();
    restoreDownloads();
    startConnectivityMonitor();

    // Native first-launch: redirect to /connect to configure server URL.
    if (isNative() && !getServerUrl()) {
      goto("/connect");
      return;
    }

    try {
      const data = await apiFetch<{ setup_required: boolean }>("/auth/setup");
      setupRequired.set(data.setup_required);
      isOffline.set(false);
    } catch {
      // In native shells with no configured server URL, the API call hit the
      // local static frontend and returned HTML instead of JSON. Redirect to
      // the server-configuration page rather than showing a broken login.
      if (isNative() && !getServerUrl()) {
        goto("/connect");
        return;
      }

      // Check if the backend is actually unreachable.
      const offline = await checkConnectivity();
      if (offline) {
        // Backend down — check if there are downloaded tracks to show.
        const dlMap = get(downloads);
        const hasDownloads = [...dlMap.values()].some(
          (e) => e.status === "done",
        );
        if (hasDownloads) {
          goto("/offline");
          return;
        }
      }

      // If the check fails, assume setup is done and fall through to login guard.
      setupRequired.set(false);
    }
  });

  $effect(() => {
    if ($setupRequired === null) return;

    const path = $page.url.pathname;

    // Public pages — skip all auth / setup guards.
    if (
      path.startsWith("/listen/") ||
      path === "/connect" ||
      path.startsWith("/share/") ||
      path === "/offline"
    )
      return;

    // Native shell without a configured server URL — send to /connect first.
    if (isNative() && !getServerUrl()) {
      goto("/connect");
      return;
    }

    if ($setupRequired) {
      // No users yet — only /setup is accessible.
      if (path !== "/setup") goto("/setup");
    } else {
      // Setup done — /setup is no longer accessible.
      if (path === "/setup") {
        goto($isAuthenticated ? "/" : "/login");
      } else if (
        path !== "/login" &&
        path !== "/offline" &&
        !$isAuthenticated
      ) {
        // Token expired or logged out — send to login (unless offline page).
        dataLoaded = false;
        goto("/login");
      } else if ($isAuthenticated) {
        if (!dataLoaded) {
          dataLoaded = true;
          favorites.load();
          loadEQProfiles().catch(() => {});
        }
      }
    }
  });

  // ── Auto-navigate away from /offline when connectivity is restored ────────
  $effect(() => {
    const offline = $isOffline;
    const path = $page.url.pathname;
    if (!offline && path === "/offline") {
      goto("/");
    }
  });

  // ── Auto-navigate TO /offline when backend becomes unreachable ────────────
  // Uses a confirmation check to avoid reacting to transient glitches.
  let offlineConfirmTimeout: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    const offline = $isOffline;
    const path = $page.url.pathname;
    // Already on a public/offline page — nothing to do.
    if (
      path === "/offline" ||
      path === "/login" ||
      path === "/setup" ||
      path === "/connect" ||
      path.startsWith("/listen/") ||
      path.startsWith("/share/")
    ) {
      if (offlineConfirmTimeout) {
        clearTimeout(offlineConfirmTimeout);
        offlineConfirmTimeout = null;
      }
      return;
    }
    if (!offline) {
      if (offlineConfirmTimeout) {
        clearTimeout(offlineConfirmTimeout);
        offlineConfirmTimeout = null;
      }
      return;
    }
    // Offline detected — wait 2 s then re-verify before actually redirecting.
    if (offlineConfirmTimeout) return; // already waiting
    offlineConfirmTimeout = setTimeout(async () => {
      offlineConfirmTimeout = null;
      const stillOffline = await checkConnectivity();
      if (!stillOffline) return; // was just a transient glitch
      const dlMap = get(downloads);
      const hasDownloads = [...dlMap.values()].some((e) => e.status === "done");
      if (hasDownloads) {
        goto("/offline");
      }
    }, 2000);
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

    libraryApi
      .album(track.album_id)
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
        const defaultProfile =
          get(eqProfiles).find((p) => p.is_default) ?? null;
        if (defaultProfile) applyEQProfile(defaultProfile);
      })
      .catch(() => {});
  });
</script>

<svelte:window onkeydown={onKeydown} />

<svelte:head>
  <title>Orb</title>
</svelte:head>

{#if isTauri()}
  <div class="window-frame" aria-hidden="true"></div>
{/if}

{#if $page.url.pathname.startsWith("/listen/") || $page.url.pathname === "/connect" || $page.url.pathname.startsWith("/share/") || $page.url.pathname === "/offline"}
  <!-- Public page: render without shell or auth guards -->
  {@render children()}
{:else if $setupRequired === null}
  <!-- Checking setup status; render nothing to avoid a flash of wrong content. -->
{:else if $setupRequired && $page.url.pathname === "/setup"}
  {@render children()}
{:else if !$setupRequired && $page.url.pathname === "/login"}
  {@render children()}
{:else if !$setupRequired && $isAuthenticated}
  <div
    class="shell"
    class:tauri={isTauri()}
    class:party-open={$lpPanelOpen && $lpRole === "host"}
  >
    {#if isTauri()}<TitleBar />{/if}
    <TopBar />
    <Sidebar />
    <main class="content">
      {@render children()}
    </main>
    <BottomBar />
    <ListenPartyPanel />
  </div>
  <MobilePlayer />
  <MobileNav />
  <MobileAvatar />
  <ContextMenu />
  <QueueModal />
  <LyricsModal />
  <KeyboardShortcuts bind:open={shortcutsOpen} shortcuts={SHORTCUTS.map(({ label, description }) => ({ label, description }))} />
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

  .shell.tauri {
    grid-template-rows: var(--titlebar-h) var(--top-h) 1fr var(--bottom-h);
    grid-template-areas:
      "titlebar titlebar"
      "top      top"
      "sidebar  content"
      "bottom   bottom";
  }
  .content {
    grid-area: content;
    overflow-y: auto;
    background: var(--bg);
    padding: var(--page-padding);
    box-sizing: border-box;
  }
  /* Push grid-area assignments into child components via :global */
  :global(header.topbar) {
    grid-area: top;
  }
  :global(aside.sidebar) {
    grid-area: sidebar;
  }
  :global(footer.bottom-bar) {
    grid-area: bottom;
  }
  :global(aside.party-panel) {
    grid-area: party;
  }

  /* ── Desktop: expand grid to include party panel column when open ─────── */
  @media (min-width: 641px) {
    .shell.party-open {
      grid-template-columns: var(--sidebar-w) 1fr 280px;
      grid-template-areas:
        "top    top    top"
        "sidebar content party"
        "bottom bottom  bottom";
    }
    .shell.tauri.party-open {
      grid-template-columns: var(--sidebar-w) 1fr 280px;
      grid-template-areas:
        "titlebar titlebar titlebar"
        "top      top      top"
        "sidebar  content  party"
        "bottom   bottom   bottom";
    }
  }

  /* ── Mobile layout: full-screen content, fixed bottom mobile UI ─────────── */
  @media (max-width: 640px) {
    .shell {
      grid-template-rows: 1fr;
      grid-template-columns: 1fr;
      grid-template-areas: "content";
    }
    .shell.tauri {
      grid-template-rows: var(--titlebar-h) 1fr;
      grid-template-areas:
        "titlebar"
        "content";
    }
    /* Hide desktop navigation on mobile */
    :global(header.topbar) {
      display: none !important;
    }
    :global(footer.bottom-bar) {
      display: none !important;
    }
    /* Sidebar stays off-screen (its own transform handles it) */
    .content {
      /* Pad below content so it doesn't hide behind mobile player + nav */
      padding-bottom: calc(
        64px + env(safe-area-inset-bottom, 0px) + /* mini player height */ 66px
      );
    }
  }

  /* ── Tauri frameless window border overlay ────────────────────────────── */
  .window-frame {
    position: fixed;
    inset: 0;
    border: 1px solid var(--border-2);
    border-radius: 0;
    pointer-events: none;
    z-index: 99999;
  }
</style>
