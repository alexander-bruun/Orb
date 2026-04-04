<script lang="ts">
  import { page } from "$app/stores";
  import { afterNavigate } from "$app/navigation";
  import { currentTrack } from "$lib/stores/player";
  import { currentAudiobook } from "$lib/stores/player/audiobookPlayer";
  import { currentEpisode } from "$lib/stores/player/podcastPlayer";
  import { activePlayer } from "$lib/stores/player/engine";
  import { expanded } from "./coverExpandStore";
  import { authStore } from "$lib/stores/auth";
  import {
    sidebarOpen,
    sidebarWidth,
    SIDEBAR_MIN_WIDTH,
    SIDEBAR_MAX_WIDTH,
  } from "$lib/stores/ui/sidebar";

  function toggleExpand() {
    expanded.update((v) => !v);
  }

  // Close sidebar on navigation (mobile drawer behaviour)
  afterNavigate(() => sidebarOpen.set(false));

  import { getApiBase } from "$lib/api/base";

  function clampSidebarWidth(width: number) {
    return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, width));
  }

  function startSidebarResize(event: PointerEvent) {
    if (window.matchMedia("(max-width: 640px)").matches) return;

    event.preventDefault();
    const startX = event.clientX;
    let startWidth = SIDEBAR_MIN_WIDTH;
    const unsubscribe = sidebarWidth.subscribe((value) => {
      startWidth = value;
    });
    unsubscribe();

    document.body.classList.add("sidebar-resizing");

    const onPointerMove = (moveEvent: PointerEvent) => {
      const delta = moveEvent.clientX - startX;
      const nextWidth = clampSidebarWidth(startWidth + delta);
      sidebarWidth.set(nextWidth);
    };

    const stop = () => {
      document.body.classList.remove("sidebar-resizing");
      window.removeEventListener("pointermove", onPointerMove);
      window.removeEventListener("pointerup", stop);
      window.removeEventListener("pointercancel", stop);
    };

    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", stop);
    window.addEventListener("pointercancel", stop);
  }
</script>

{#if $sidebarOpen}
  <div
    class="sidebar-backdrop"
    role="presentation"
    on:click={() => sidebarOpen.set(false)}
  ></div>
{/if}

<aside class="sidebar" class:mobile-open={$sidebarOpen}>
  <nav class="nav">
    <a href="/" class:active={$page.url.pathname === "/"}>Home</a>
    <a href="/library" class:active={$page.url.pathname.startsWith("/library")}
      >Music</a
    >
    <a
      href="/audiobooks"
      class:active={$page.url.pathname.startsWith("/audiobooks")}>Audiobooks</a
    >
    <a
      href="/podcasts"
      class:active={$page.url.pathname.startsWith("/podcasts")}>Podcasts</a
    >
    <a
      href="/playlists"
      class:active={$page.url.pathname.startsWith("/playlists") ||
        $page.url.pathname.startsWith("/smart-playlists")}>Playlists</a
    >
    <a href="/favorites" class:active={$page.url.pathname === "/favorites"}
      >Favorites</a
    >
    <a href="/search" class:active={$page.url.pathname === "/search"}>Search</a>
  </nav>

  <div class="spacer"></div>

  {#if $expanded && ($currentTrack || $currentAudiobook || $currentEpisode)}
    <div class="sidebar-bottom">
      <div class="cover-wrap">
        {#if $activePlayer === "audiobook" && $currentAudiobook}
          {#if $currentAudiobook.cover_art_key}
            <img
              src="{getApiBase()}/covers/audiobook/{$currentAudiobook.id}"
              alt="audiobook cover"
              class="full-image"
            />
          {:else}
            <div class="placeholder full-image"></div>
          {/if}
        {:else if $activePlayer === "podcast" && $currentEpisode}
          <img
            src="{getApiBase()}/covers/podcast/{$currentEpisode.podcast_id}"
            alt="podcast cover"
            class="full-image"
          />
        {:else if $currentTrack}
          {#if $currentTrack.album_id}
            <img
              src="{getApiBase()}/covers/{$currentTrack.album_id}"
              alt="album art"
              class="full-image"
            />
          {:else}
            <div class="placeholder full-image"></div>
          {/if}
        {/if}
        <button
          class="expand-btn overlay"
          on:click={toggleExpand}
          aria-label="Shrink cover"
        >
          <svg width="20" height="20" viewBox="0 0 20 20"
            ><path d="M6 14h8v-8H6v8zm2-6h4v4H8v-4z" fill="currentColor" /></svg
          >
        </button>
      </div>
    </div>
  {/if}
  <div
    class="sidebar-resize-handle"
    role="separator"
    aria-label="Resize sidebar"
    aria-orientation="vertical"
    on:pointerdown={startSidebarResize}
  ></div>
</aside>

<style>
  .sidebar {
    position: relative;
    width: var(--sidebar-w);
    flex-shrink: 0;
    background: var(--bg-elevated);
    display: flex;
    flex-direction: column;
    padding-top: 16px;
    overflow: hidden;
  }
  .nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 0 12px;
  }
  .nav a {
    padding: 8px 12px;
    border-radius: 6px;
    color: var(--text-muted);
    text-decoration: none;
    font-size: 0.875rem;
    transition:
      color 0.15s,
      background 0.15s;
  }
  .nav a:hover {
    color: var(--text);
    background: var(--bg-hover);
  }
  .nav a.active {
    color: var(--text);
    background: var(--bg-hover);
  }
  .spacer {
    flex: 1;
  }
  .sidebar-bottom {
    padding: 12px;
    border-top: 1px solid var(--border);
  }
  .cover-wrap {
    position: relative;
  }
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
    background: rgba(0, 0, 0, 0.5);
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
  .expand-btn:hover {
    background: rgba(0, 0, 0, 0.7);
  }
  .expand-btn.overlay {
    position: absolute;
    top: 8px;
    right: 8px;
  }

  .sidebar-resize-handle {
    position: absolute;
    top: 0;
    right: -4px;
    width: 8px;
    height: 100%;
    cursor: col-resize;
    z-index: 3;
  }

  .sidebar-resize-handle::after {
    content: "";
    position: absolute;
    top: 0;
    bottom: 0;
    left: 3px;
    width: 2px;
    background: transparent;
    transition: background 0.15s ease;
  }

  .sidebar-resize-handle:hover::after {
    background: var(--border-2);
  }

  :global(body.sidebar-resizing) {
    user-select: none;
    cursor: col-resize;
  }

  /* ── Mobile overlay drawer ──────────────────────────────── */
  .sidebar-backdrop {
    display: none;
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    z-index: 49;
    backdrop-filter: blur(2px);
  }

  @media (max-width: 640px) {
    .sidebar {
      position: fixed;
      left: 0;
      top: 0;
      height: 100dvh;
      z-index: 50;
      transform: translateX(-100%);
      transition: transform 0.25s cubic-bezier(0.4, 0, 0.2, 1);
      /* keep the sidebar visible above the grid's sidebar cell (which is gone on mobile) */
    }
    .sidebar.mobile-open {
      transform: translateX(0);
    }
    .sidebar-backdrop {
      display: block;
    }

    .sidebar-resize-handle {
      display: none;
    }
  }
</style>
