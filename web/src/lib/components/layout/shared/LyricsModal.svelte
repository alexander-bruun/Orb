<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { fly } from "svelte/transition";
  import { cubicOut, cubicIn } from "svelte/easing";
  import {
    lyricsOpen,
    lyricsLines,
    lyricsLoading,
    activeLyricIndex,
    lyricsMode,
  } from "$lib/stores/player/lyrics";
  import { currentTrack, positionMs, seek } from "$lib/stores/player";
  import Spinner from "$lib/components/ui/Spinner.svelte";

  // Drag state (modal mode only)
  let posX = 0;
  let posY = 0;
  let dragOffsetX = 0;
  let dragOffsetY = 0;
  let dragging = false;

  // DOM refs
  let container: HTMLElement;
  let listEl: HTMLElement;

  // ── Smooth scroll via rAF lerp ──
  let targetScrollTop = 0;
  let currentScrollTop = 0;
  let rafId: number | null = null;
  let userScrolling = false;
  let userScrollTimer: ReturnType<typeof setTimeout> | null = null;

  const LERP_SPEED = 0.08;

  function scrollTick() {
    if (!listEl || !$lyricsOpen) {
      rafId = null;
      return;
    }
    if (!userScrolling) {
      const diff = targetScrollTop - currentScrollTop;
      if (Math.abs(diff) > 0.5) {
        currentScrollTop += diff * LERP_SPEED;
        listEl.scrollTop = Math.round(currentScrollTop);
      } else {
        currentScrollTop = targetScrollTop;
        listEl.scrollTop = targetScrollTop;
      }
    } else {
      currentScrollTop = listEl.scrollTop;
    }
    rafId = requestAnimationFrame(scrollTick);
  }

  function startScrollLoop() {
    if (rafId === null) {
      currentScrollTop = listEl?.scrollTop ?? 0;
      rafId = requestAnimationFrame(scrollTick);
    }
  }

  function stopScrollLoop() {
    if (rafId !== null) {
      cancelAnimationFrame(rafId);
      rafId = null;
    }
  }

  function onWheel() {
    userScrolling = true;
    if (userScrollTimer) clearTimeout(userScrollTimer);
    userScrollTimer = setTimeout(() => {
      userScrolling = false;
      currentScrollTop = listEl?.scrollTop ?? 0;
    }, 3000);
  }

  function updateTargetScroll(idx: number) {
    if (!listEl || idx < 0) return;
    const el = listEl.querySelector<HTMLElement>(`[data-idx="${idx}"]`);
    if (!el) return;
    const containerH = listEl.clientHeight;
    const elTop = el.offsetTop;
    const elH = el.offsetHeight;
    targetScrollTop = Math.max(0, elTop - containerH / 2 + elH / 2);
  }

  let prevIdx = -1;
  $: if ($activeLyricIndex !== prevIdx && $lyricsOpen && listEl) {
    prevIdx = $activeLyricIndex;
    userScrolling = false;
    if (userScrollTimer) clearTimeout(userScrollTimer);
    updateTargetScroll($activeLyricIndex);
  }

  $: if ($lyricsOpen && listEl) {
    updateTargetScroll($activeLyricIndex);
    currentScrollTop = targetScrollTop;
    listEl.scrollTop = targetScrollTop;
    startScrollLoop();
  } else {
    stopScrollLoop();
  }

  let prevTrackId: string | null = null;
  $: if ($currentTrack?.id !== prevTrackId) {
    prevTrackId = $currentTrack?.id ?? null;
    prevIdx = -1;
    targetScrollTop = 0;
    currentScrollTop = 0;
    if (listEl) listEl.scrollTop = 0;
  }

  // ── Next-line fade ──
  let nextLineFade = 0;
  $: {
    const lines = $lyricsLines;
    const pos = $positionMs;
    const idx = $activeLyricIndex;
    if (idx >= 0 && idx < lines.length - 1) {
      const nextTime = lines[idx + 1].time_ms;
      const fadeStart = Math.max(lines[idx].time_ms, nextTime - 600);
      if (pos >= fadeStart) {
        nextLineFade = Math.min(1, (pos - fadeStart) / (nextTime - fadeStart));
      } else {
        nextLineFade = 0;
      }
    } else {
      nextLineFade = 0;
    }
  }

  function seekToLine(time_ms: number) {
    seek(time_ms / 1000);
    userScrolling = false;
    if (userScrollTimer) clearTimeout(userScrollTimer);
  }

  // ── Drag (modal mode only) ──
  onMount(() => {
    posX = window.innerWidth - 360;
    posY = Math.max(60, window.innerHeight - 560);
  });

  onDestroy(() => {
    stopScrollLoop();
    if (userScrollTimer) clearTimeout(userScrollTimer);
  });

  function startDrag(e: MouseEvent) {
    if ($lyricsMode !== "modal") return;
    if ((e.target as HTMLElement).closest("button")) return;
    dragging = true;
    dragOffsetX = e.clientX - posX;
    dragOffsetY = e.clientY - posY;
    window.addEventListener("mousemove", onDragMove);
    window.addEventListener("mouseup", stopDrag);
    e.preventDefault();
  }

  function onDragMove(e: MouseEvent) {
    if (!dragging) return;
    posX = Math.max(
      0,
      Math.min(window.innerWidth - 320, e.clientX - dragOffsetX),
    );
    posY = Math.max(
      0,
      Math.min(window.innerHeight - 80, e.clientY - dragOffsetY),
    );
  }

  function stopDrag() {
    dragging = false;
    window.removeEventListener("mousemove", onDragMove);
    window.removeEventListener("mouseup", stopDrag);
  }

  // Direction for teleprompter transitions — lines scroll upward as they advance
  let tpDir = 1; // 1 = going forward, -1 = going back
  let prevTpIdx = -1;
  $: {
    const idx = $activeLyricIndex;
    tpDir = idx >= prevTpIdx ? 1 : -1;
    prevTpIdx = idx;
  }

  // Non-narrowing helper so mode comparisons work inside any {#if} branch
  let mode: string = "modal";
  $: mode = $lyricsMode;
  function isMode(m: string): boolean {
    return mode === m;
  }

  let tpAtTop = false;
</script>

{#if $lyricsOpen}
  {#if mode === "teleprompter"}
    <!-- ── Teleprompter strip ── -->
    <div class="tp-strip" class:at-top={tpAtTop}>
      <div class="tp-text-area">
        {#if $lyricsLoading}
          <span class="tp-state"><Spinner size={16} /></span>
        {:else if $lyricsLines.length === 0}
          <span class="tp-state">No lyrics available</span>
        {:else if $activeLyricIndex < 0}
          <span class="tp-state">♪</span>
        {:else}
          {#key $activeLyricIndex}
            <button
              class="tp-line"
              in:fly={{ y: tpDir * 10, duration: 300, easing: cubicOut }}
              out:fly={{ y: tpDir * -10, duration: 200, easing: cubicIn }}
              on:click={() =>
                seekToLine($lyricsLines[$activeLyricIndex].time_ms)}
            >
              <span class="tp-line-inner"
                >{$lyricsLines[$activeLyricIndex].text}</span
              >
            </button>
          {/key}
        {/if}
      </div>

      <!-- Controls revealed on hover -->
      <div class="tp-controls">
        <button
          class="tp-btn"
          on:click={() => (tpAtTop = !tpAtTop)}
          title={tpAtTop ? "Move to bottom" : "Move to top"}
          aria-label={tpAtTop ? "Move to bottom" : "Move to top"}
        >
          {#if tpAtTop}
            <svg
              width="11"
              height="11"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <line x1="3" y1="21" x2="21" y2="21" />
              <polyline points="6 15 12 9 18 15" />
            </svg>
          {:else}
            <svg
              width="11"
              height="11"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <line x1="3" y1="3" x2="21" y2="3" />
              <polyline points="6 9 12 15 18 9" />
            </svg>
          {/if}
        </button>
        <div class="mode-switcher">
          <button
            class="mode-btn"
            class:active={isMode("modal")}
            on:click={() => lyricsMode.set("modal")}
            title="Popup mode"
            aria-label="Popup mode"
          >
            <svg
              width="11"
              height="11"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <rect x="3" y="3" width="18" height="18" rx="3" />
              <line x1="9" y1="3" x2="9" y2="21" />
            </svg>
          </button>
          <button
            class="mode-btn"
            class:active={isMode("overlay")}
            on:click={() => lyricsMode.set("overlay")}
            title="Bar overlay mode"
            aria-label="Bar overlay mode"
          >
            <svg
              width="11"
              height="11"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <rect x="2" y="14" width="20" height="8" rx="2" />
              <rect x="2" y="2" width="20" height="10" rx="2" />
            </svg>
          </button>
          <button
            class="mode-btn"
            class:active={isMode("teleprompter")}
            title="Teleprompter mode"
            aria-label="Teleprompter mode"
          >
            <svg
              width="11"
              height="11"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <line x1="4" y1="7" x2="20" y2="7" stroke-opacity="0.4" />
              <line x1="2" y1="12" x2="22" y2="12" stroke-width="3" />
              <line x1="4" y1="17" x2="20" y2="17" stroke-opacity="0.4" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  {:else}
    <!-- ── Modal / Overlay panel ── -->
    <div
      class="lyrics-modal"
      class:dragging
      class:overlay-mode={mode === "overlay"}
      bind:this={container}
      style={mode === "modal" ? `left: ${posX}px; top: ${posY}px;` : ""}
    >
      <div class="modal-header" role="presentation" on:mousedown={startDrag}>
        <span class="modal-title">
          <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M9 18V5l12-2v13" />
            <circle cx="6" cy="18" r="3" /><circle cx="18" cy="16" r="3" />
          </svg>
          Lyrics
        </span>
        {#if $currentTrack}
          <span class="track-name">{$currentTrack.title}</span>
        {/if}
        <div class="mode-switcher">
          <button
            class="mode-btn"
            class:active={mode === "modal"}
            on:click|stopPropagation={() => lyricsMode.set("modal")}
            title="Popup mode"
            aria-label="Popup mode"
          >
            <svg
              width="11"
              height="11"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <rect x="3" y="3" width="18" height="18" rx="3" />
              <line x1="9" y1="3" x2="9" y2="21" />
            </svg>
          </button>
          <button
            class="mode-btn"
            class:active={mode === "overlay"}
            on:click|stopPropagation={() => lyricsMode.set("overlay")}
            title="Bar overlay mode"
            aria-label="Bar overlay mode"
          >
            <svg
              width="11"
              height="11"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <rect x="2" y="14" width="20" height="8" rx="2" />
              <rect x="2" y="2" width="20" height="10" rx="2" />
            </svg>
          </button>
          <button
            class="mode-btn"
            class:active={mode === "teleprompter"}
            on:click|stopPropagation={() => lyricsMode.set("teleprompter")}
            title="Teleprompter mode"
            aria-label="Teleprompter mode"
          >
            <svg
              width="11"
              height="11"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <line x1="4" y1="7" x2="20" y2="7" stroke-opacity="0.4" />
              <line x1="2" y1="12" x2="22" y2="12" stroke-width="3" />
              <line x1="4" y1="17" x2="20" y2="17" stroke-opacity="0.4" />
            </svg>
          </button>
        </div>
        <button
          class="close-btn"
          on:click={() => lyricsOpen.set(false)}
          aria-label="Close lyrics"
        >
          <svg
            width="13"
            height="13"
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

      <div class="modal-body" bind:this={listEl} on:wheel={onWheel}>
        {#if $lyricsLoading}
          <div class="state-msg"><Spinner size={22} /></div>
        {:else if $lyricsLines.length === 0}
          <div class="state-msg">No lyrics available</div>
        {:else}
          {#each $lyricsLines as line, i (line.time_ms + "-" + i)}
            <button
              type="button"
              class="lyric-line"
              class:active={i === $activeLyricIndex}
              class:past={i < $activeLyricIndex}
              class:next={i === $activeLyricIndex + 1}
              style={i === $activeLyricIndex + 1
                ? `opacity: ${0.55 + 0.45 * nextLineFade}`
                : ""}
              data-idx={i}
              on:click={() => seekToLine(line.time_ms)}
            >
              {line.text}
            </button>
          {/each}
        {/if}
      </div>
    </div>
  {/if}
{/if}

<style>
  /* ── Modal / Overlay shared ── */
  .lyrics-modal {
    position: fixed;
    z-index: 600;
    width: 320px;
    max-height: 440px;
    display: flex;
    flex-direction: column;
    background: var(--bg-elevated);
    border: 1px solid var(--border-2);
    border-radius: 10px;
    box-shadow:
      0 8px 32px rgba(0, 0, 0, 0.45),
      0 2px 8px rgba(0, 0, 0, 0.2);
    overflow: hidden;
    animation: popIn 0.18s cubic-bezier(0.34, 1.56, 0.64, 1);
    user-select: none;
  }

  .lyrics-modal.dragging {
    box-shadow: 0 16px 48px rgba(0, 0, 0, 0.55);
    cursor: grabbing;
  }

  .lyrics-modal.overlay-mode {
    bottom: var(--bottom-h);
    right: 20px;
    top: auto;
    left: auto;
    max-height: 420px;
    border-bottom-left-radius: 0;
    border-bottom-right-radius: 0;
    border-bottom: none;
    box-shadow:
      0 -4px 24px rgba(0, 0, 0, 0.35),
      0 -1px 6px rgba(0, 0, 0, 0.15);
    animation: slideUp 0.18s cubic-bezier(0.34, 1.56, 0.64, 1);
  }

  @keyframes popIn {
    from {
      opacity: 0;
      transform: scale(0.94);
    }
    to {
      opacity: 1;
      transform: scale(1);
    }
  }

  @keyframes slideUp {
    from {
      opacity: 0;
      transform: translateY(12px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .modal-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    border-bottom: 1px solid var(--border);
    cursor: grab;
    flex-shrink: 0;
  }
  .overlay-mode .modal-header {
    cursor: default;
  }
  .lyrics-modal.dragging .modal-header {
    cursor: grabbing;
  }

  .modal-title {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .track-name {
    flex: 1;
    font-size: 0.78rem;
    color: var(--text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  /* Mode switcher (shared by panel header and tp-controls) */
  .mode-switcher {
    display: flex;
    align-items: center;
    gap: 2px;
    flex-shrink: 0;
    background: var(--bg-hover);
    border: 1px solid var(--border);
    border-radius: 5px;
    padding: 2px;
  }

  .mode-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 18px;
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    border-radius: 3px;
    transition:
      background 0.12s,
      color 0.12s;
    padding: 0;
  }
  .mode-btn:hover {
    color: var(--text);
  }
  .mode-btn.active {
    background: var(--bg-elevated);
    color: var(--accent);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
  }

  .close-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    border-radius: 5px;
    flex-shrink: 0;
    transition:
      background 0.12s,
      color 0.12s;
  }
  .close-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .modal-body {
    flex: 1;
    overflow-y: auto;
    padding: 16px 12px 20px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .modal-body::-webkit-scrollbar {
    width: 4px;
  }
  .modal-body::-webkit-scrollbar-track {
    background: transparent;
  }
  .modal-body::-webkit-scrollbar-thumb {
    background: var(--border);
    border-radius: 2px;
  }

  .state-msg {
    text-align: center;
    padding: 32px 0;
    font-size: 0.82rem;
    color: var(--text-muted);
  }

  .lyric-line {
    padding: 6px 10px;
    font-size: 0.9rem;
    line-height: 1.5;
    color: var(--text-muted);
    border-radius: 6px;
    transition:
      color 0.3s ease,
      background 0.3s ease,
      opacity 0.3s ease,
      transform 0.3s ease;
    cursor: pointer;
    opacity: 0.55;
    border: none;
    background: none;
    width: 100%;
    text-align: left;
    font: inherit;
  }
  .lyric-line:hover {
    background: var(--bg-hover);
    opacity: 1 !important;
  }
  .lyric-line.past {
    color: var(--text-2);
    opacity: 0.35;
  }
  .lyric-line.active {
    font-size: 1rem;
    font-weight: 600;
    background: var(--accent-dim);
    color: var(--accent);
    opacity: 1;
    transform: scale(1.01);
  }
  .lyric-line.next {
    color: var(--text);
  }

  /* ── Teleprompter strip ── */
  .tp-strip {
    position: fixed;
    z-index: 600;
    /* Span the playback section: from sidebar edge to right */
    left: var(--sidebar-w);
    right: 0;
    bottom: var(--bottom-h);
    height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    user-select: none;
    animation: tpIn 0.2s ease;
    pointer-events: none;
  }

  @keyframes tpIn {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  /* Text area — fixed height, clip overflow so transitions don't bleed */
  .tp-text-area {
    position: relative;
    flex: 1;
    height: 100%;
    overflow: hidden;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  /* Each lyric line absolutely fills the text area */
  .tp-line {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    font: inherit;
    font-size: 0.92rem;
    font-weight: 600;
    color: var(--accent);
    cursor: pointer;
    pointer-events: auto;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    padding: 0 16px;
    transition: color 0.15s;
    /* Background on the inner text span, not the full-width button */
    background: none;
  }
  .tp-line:hover {
    color: var(--text);
  }
  /* Elliptical backdrop centered behind the lyric text */
  .tp-text-area::before {
    content: "";
    position: absolute;
    inset: -12px -20px;
    background: radial-gradient(
      ellipse 55% 100% at center,
      var(--bg-elevated) 20%,
      transparent 100%
    );
    pointer-events: none;
    z-index: 0;
  }

  .tp-line-inner,
  .tp-state {
    position: relative;
    z-index: 1;
    white-space: nowrap;
    max-width: 100%;
    background: none;
  }

  .tp-state {
    font-size: 0.82rem;
    color: var(--text-muted);
  }

  /* Controls shown on strip hover */
  .tp-controls {
    position: absolute;
    right: 12px;
    top: 50%;
    transform: translateY(-50%);
    display: flex;
    align-items: center;
    gap: 6px;
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.15s;
  }
  /* Re-enable pointer events on the interactive children */
  .tp-text-area,
  .tp-strip:hover .tp-controls {
    pointer-events: auto;
  }
  .tp-strip:hover .tp-controls {
    opacity: 1;
  }

  /* Top position variant */
  .tp-strip.at-top {
    bottom: auto;
    top: var(--top-h);
  }

  /* Position toggle button */
  .tp-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    background: var(--bg-hover);
    border: 1px solid var(--border);
    border-radius: 5px;
    color: var(--text-muted);
    cursor: pointer;
    flex-shrink: 0;
    transition:
      background 0.12s,
      color 0.12s;
    padding: 0;
  }
  .tp-btn:hover {
    background: var(--bg-elevated);
    color: var(--text);
  }

  /* Hide on mobile (no bottom bar) */
  @media (max-width: 640px) {
    .tp-strip {
      display: none;
    }
  }
</style>
