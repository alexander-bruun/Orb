<script lang="ts">
  import '../app.css';
  import TopBar from '$lib/components/layout/TopBar.svelte';
  import Sidebar from '$lib/components/layout/Sidebar.svelte';
  import BottomBar from '$lib/components/layout/BottomBar.svelte';
  import ContextMenu from '$lib/components/ui/ContextMenu.svelte';
  import QueueModal from '$lib/components/ui/QueueModal.svelte';
  import { isAuthenticated } from '$lib/stores/auth';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';

  let { children } = $props();

  $effect(() => {
    if ($page.url.pathname !== '/login' && !$isAuthenticated) {
      goto('/login');
    }
  });
</script>

<svelte:head>
  <title>Orb</title>
</svelte:head>

{#if $page.url.pathname === '/login'}
  {@render children()}
{:else if $isAuthenticated}
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
