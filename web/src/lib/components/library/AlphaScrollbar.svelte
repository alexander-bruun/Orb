<script lang="ts">
  import { derived } from 'svelte/store';
  import { lpPanelOpen, lpRole } from '$lib/stores/social/listenParty';

  export let keys: string[] = [];
  export let activeKey: string = '';
  export let scrollEl: HTMLElement | null = null;

  // Shift the bar left on desktop when the listen party panel is open
  const partyOpen = derived([lpPanelOpen, lpRole], ([$open, $role]) => $open && $role === 'host');

  function jumpTo(key: string) {
    if (!scrollEl) return;
    const section = scrollEl.querySelector(`[data-scroll-key="${key}"]`);
    if (!section) return;
    const sectionRect = (section as HTMLElement).getBoundingClientRect();
    const containerRect = scrollEl.getBoundingClientRect();
    const offset = sectionRect.top - containerRect.top + scrollEl.scrollTop - 8;
    scrollEl.scrollTo({ top: offset, behavior: 'smooth' });
  }
</script>

{#if keys.length > 0}
  <nav class="alpha-bar" aria-label="Quick navigation" style="right: {$partyOpen ? '280px' : '0'}">
    {#each keys as key}
      <button
        class="key-item"
        class:active={activeKey === key}
        on:click={() => jumpTo(key)}
        aria-label="Jump to {key}"
      >
        {key}
      </button>
    {/each}
  </nav>
{/if}

<style>
  .alpha-bar {
    position: fixed;
    right: 0;
    top: var(--top-h);
    bottom: var(--bottom-h);

    width: 36px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: space-evenly;
    z-index: 30;
    padding: 8px 0;
    background: var(--bg);
    border-left: 1px solid var(--border);
    user-select: none;
  }

  :global(.tauri-body) .alpha-bar {
    top: calc(var(--top-h) + var(--titlebar-h));
  }

  .key-item {
    all: unset;
    cursor: pointer;
    font-size: 10px;
    font-weight: 600;
    font-family: 'DM Mono', monospace;
    color: var(--text-2);
    line-height: 1;
    width: 100%;
    text-align: center;
    transition: color 0.1s, transform 0.1s;
  }

  .key-item:hover {
    color: var(--text);
    transform: scale(1.25);
  }

  .key-item.active {
    color: var(--accent);
    font-size: 11px;
  }

  /* On mobile the real bottom is: nav (60px) + mini-player (~68px) + safe-area */
  @media (max-width: 640px) {
    .alpha-bar {
      right: 0 !important; /* party panel is full-screen on mobile, don't shift */
      bottom: calc(128px + env(safe-area-inset-bottom, 0px));
    }
  }
</style>
