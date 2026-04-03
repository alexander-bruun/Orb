<script lang="ts">
  export interface ShortcutEntry {
    label: string;
    description: string;
  }

  let {
    open = $bindable(false),
    shortcuts,
  }: { open: boolean; shortcuts: ShortcutEntry[] } = $props();

  function close() {
    open = false;
  }

  function onBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) close();
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") close();
  }
</script>

{#if open}
  <div
    class="backdrop"
    role="presentation"
    onclick={onBackdropClick}
    onkeydown={onKeydown}
  >
    <div
      class="sheet"
      role="dialog"
      aria-modal="true"
      aria-label="Keyboard shortcuts"
    >
      <div class="sheet-head">
        <span class="sheet-title">Keyboard Shortcuts</span>
        <button class="close-btn" onclick={close} aria-label="Close">
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            aria-hidden="true"
          >
            <line x1="18" y1="6" x2="6" y2="18" />
            <line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>
      </div>
      <ul class="shortcut-list">
        {#each shortcuts as { label, description }}
          <li class="shortcut-row">
            <kbd class="key">{label}</kbd>
            <span class="desc">{description}</span>
          </li>
        {/each}
      </ul>
    </div>
  </div>
{/if}

<svelte:window onkeydown={onKeydown} />

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 9000;
    background: rgba(0, 0, 0, 0.55);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .sheet {
    background: var(--surface-2);
    border: 1px solid var(--border-1);
    border-radius: 12px;
    padding: 0;
    min-width: 320px;
    max-width: 440px;
    width: 90%;
    box-shadow: 0 24px 64px rgba(0, 0, 0, 0.5);
  }

  .sheet-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 20px 12px;
    border-bottom: 1px solid var(--border-1);
  }

  .sheet-title {
    font-size: 0.85rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--text-2);
  }

  .close-btn {
    all: unset;
    cursor: pointer;
    color: var(--text-2);
    display: flex;
    align-items: center;
    padding: 4px;
    border-radius: 6px;
    transition:
      color 0.15s,
      background 0.15s;
  }

  .close-btn:hover {
    color: var(--text-1);
    background: var(--surface-3);
  }

  .shortcut-list {
    list-style: none;
    margin: 0;
    padding: 12px 0;
  }

  .shortcut-row {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 8px 20px;
  }

  .key {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 36px;
    padding: 3px 8px;
    background: var(--surface-3);
    border: 1px solid var(--border-2);
    border-radius: 6px;
    font-family: inherit;
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-1);
    box-shadow: 0 1px 0 var(--border-2);
    flex-shrink: 0;
  }

  .desc {
    font-size: 0.875rem;
    color: var(--text-2);
  }
</style>
