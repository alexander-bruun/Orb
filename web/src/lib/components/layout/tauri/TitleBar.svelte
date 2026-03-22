<script lang="ts">
    import { onMount } from "svelte";
    import { getCurrentWindow } from "@tauri-apps/api/window";

    const appWindow = getCurrentWindow();

    let title = $state(typeof document !== "undefined" ? document.title : "Orb");

    onMount(() => {
        const titleEl = document.querySelector("title");
        if (!titleEl) return;
        const observer = new MutationObserver(() => {
            title = document.title;
        });
        observer.observe(titleEl, { childList: true });
        return () => observer.disconnect();
    });

    function minimizeWindow() {
        appWindow.minimize();
    }

    function maximizeWindow() {
        appWindow.toggleMaximize();
    }

    function closeWindow() {
        appWindow.close();
    }
</script>

<div class="titlebar" data-tauri-drag-region aria-label="Window title bar">
    <div class="wordmark" data-tauri-drag-region>
        <svg viewBox="0 0 52 28" height="18" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
            <circle cx="12" cy="14" r="10" stroke="currentColor" stroke-width="1.4" opacity="0.3"/>
            <circle cx="12" cy="14" r="4.5" fill="currentColor"/>
            <text x="27" y="21" font-family="'Instrument Serif', Georgia, serif" font-style="italic" font-size="22" fill="currentColor" letter-spacing="-0.02em">orb</text>
        </svg>
    </div>

    <div class="window-title" data-tauri-drag-region>{title}</div>

    <div class="window-controls">
        <button
            class="wc-btn wc-minimize"
            on:click={minimizeWindow}
            aria-label="Minimize"
            title="Minimize"
        >
            <svg width="10" height="2" viewBox="0 0 10 2" fill="currentColor"
                ><rect width="10" height="1.5" rx="0.75" /></svg
            >
        </button>
        <button
            class="wc-btn wc-maximize"
            on:click={maximizeWindow}
            aria-label="Maximize"
            title="Maximize"
        >
            <svg
                width="10"
                height="10"
                viewBox="0 0 10 10"
                fill="none"
                stroke="currentColor"
                stroke-width="1.2"
                ><rect x="0.6" y="0.6" width="8.8" height="8.8" rx="1.4" /></svg
            >
        </button>
        <button
            class="wc-btn wc-close"
            on:click={closeWindow}
            aria-label="Close"
            title="Close"
        >
            <svg
                width="10"
                height="10"
                viewBox="0 0 10 10"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                ><line x1="1" y1="1" x2="9" y2="9" /><line
                    x1="9"
                    y1="1"
                    x2="1"
                    y2="9"
                /></svg
            >
        </button>
    </div>
</div>

<style>
    .titlebar {
        grid-area: titlebar;
        display: flex;
        align-items: center;
        height: var(--titlebar-h);
        background: var(--bg);
        border-bottom: 1px solid var(--border);
        padding: 0 8px;
        user-select: none;
        -webkit-user-select: none;
    }

    .wordmark {
        display: flex;
        align-items: center;
        flex-shrink: 0;
        color: var(--accent);
        padding: 0 8px 0 2px;
    }

    .window-title {
        flex: 1;
        text-align: center;
        font-size: 12px;
        font-weight: 500;
        color: var(--text-2);
        letter-spacing: 0.01em;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
        padding: 0 8px;
    }

    .window-controls {
        display: flex;
        align-items: center;
        gap: 6px;
        flex-shrink: 0;
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
        transition:
            background 0.12s,
            color 0.12s,
            border-color 0.12s;
        flex-shrink: 0;
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
