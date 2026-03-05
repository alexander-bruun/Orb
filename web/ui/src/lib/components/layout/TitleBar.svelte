<script lang="ts">
  async function startDrag(e: MouseEvent) {
    // Only drag when clicking the bare titlebar, not any child element
    if (e.target !== e.currentTarget) return;
    const { getCurrentWindow } = await import(/* @vite-ignore */ '@tauri-apps/api/window');
    await getCurrentWindow().startDragging();
  }

  async function minimizeWindow() {
    const { getCurrentWindow } = await import(/* @vite-ignore */ '@tauri-apps/api/window');
    await getCurrentWindow().minimize();
  }

  async function maximizeWindow() {
    const { getCurrentWindow } = await import(/* @vite-ignore */ '@tauri-apps/api/window');
    await getCurrentWindow().toggleMaximize();
  }

  async function closeWindow() {
    const { getCurrentWindow } = await import(/* @vite-ignore */ '@tauri-apps/api/window');
    await getCurrentWindow().close();
  }
</script>

<div class="titlebar" on:mousedown={startDrag} aria-label="Window title bar">
  <div class="window-controls">
    <button class="wc-btn wc-minimize" on:click={minimizeWindow} aria-label="Minimize" title="Minimize">
      <svg width="10" height="2" viewBox="0 0 10 2" fill="currentColor"><rect width="10" height="1.5" rx="0.75"/></svg>
    </button>
    <button class="wc-btn wc-maximize" on:click={maximizeWindow} aria-label="Maximize" title="Maximize">
      <svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.2"><rect x="0.6" y="0.6" width="8.8" height="8.8" rx="1.4"/></svg>
    </button>
    <button class="wc-btn wc-close" on:click={closeWindow} aria-label="Close" title="Close">
      <svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><line x1="1" y1="1" x2="9" y2="9"/><line x1="9" y1="1" x2="1" y2="9"/></svg>
    </button>
  </div>
</div>

<style>
  .titlebar {
    grid-area: titlebar;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    height: var(--titlebar-h);
    background: var(--bg);
    border-bottom: 1px solid var(--border);
    padding: 0 8px;
    -webkit-app-region: drag;
    app-region: drag;
    user-select: none;
    -webkit-user-select: none;
  }

  .window-controls {
    display: flex;
    align-items: center;
    gap: 6px;
    -webkit-app-region: no-drag;
    app-region: no-drag;
  }

  .wc-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border-radius: 6px;
    background: var(--surface-2);
    border: 1px solid var(--border-2);
    color: var(--muted);
    cursor: pointer;
    padding: 0;
    transition: background 0.12s, color 0.12s, border-color 0.12s;
    flex-shrink: 0;
    -webkit-app-region: no-drag;
    app-region: no-drag;
  }

  .wc-btn:hover {
    background: var(--surface-3, var(--surface-2));
    color: var(--text);
    border-color: var(--border);
  }

  .wc-close:hover {
    background: rgba(248, 113, 113, 0.15);
    border-color: rgba(248, 113, 113, 0.4);
    color: #f87171;
  }
</style>
