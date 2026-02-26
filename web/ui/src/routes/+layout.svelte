<script lang="ts">
  import '../app.css';
  import { onMount } from 'svelte';
  import TopBar from '$lib/components/layout/TopBar.svelte';
  import Sidebar from '$lib/components/layout/Sidebar.svelte';
  import BottomBar from '$lib/components/layout/BottomBar.svelte';
  import ContextMenu from '$lib/components/ui/ContextMenu.svelte';
  import QueueModal from '$lib/components/ui/QueueModal.svelte';
  import ListenPartyPanel from '$lib/components/layout/ListenPartyPanel.svelte';
  import { isAuthenticated } from '$lib/stores/auth';
  import { favorites } from '$lib/stores/favorites';
  import { setupRequired } from '$lib/stores/setup';
  import { apiFetch } from '$lib/api/client';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { togglePlayPause, next, previous } from '$lib/stores/player';

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

  onMount(async () => {
    try {
      const data = await apiFetch<{ setup_required: boolean }>('/auth/setup');
      setupRequired.set(data.setup_required);
    } catch {
      // If the check fails, assume setup is done and fall through to login guard.
      setupRequired.set(false);
    }
  });

  $effect(() => {
    if ($setupRequired === null) return;

    const path = $page.url.pathname;

    // Listen-along guest pages are public — skip all auth / setup guards.
    if (path.startsWith('/listen/')) return;

    if ($setupRequired) {
      // No users yet — only /setup is accessible.
      if (path !== '/setup') goto('/setup');
    } else {
      // Setup done — /setup is no longer accessible.
      if (path === '/setup') {
        goto($isAuthenticated ? '/' : '/login');
      } else if (path !== '/login' && !$isAuthenticated) {
        // Token expired or logged out — send to login.
        goto('/login');
      } else if ($isAuthenticated) {
        favorites.load();
      }
    }
  });
</script>

<svelte:window onkeydown={onKeydown} />

<svelte:head>
  <title>Orb</title>
</svelte:head>

{#if $page.url.pathname.startsWith('/listen/')}
  <!-- Guest listen-along page: render without any shell or auth guards -->
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
</style>
