<script lang="ts">
  export let keys: string[] = [];
  export let activeKey: string = '';
  export let scrollEl: HTMLElement | null = null;

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
  <nav class="alpha-bar" aria-label="Quick navigation">
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
</style>
