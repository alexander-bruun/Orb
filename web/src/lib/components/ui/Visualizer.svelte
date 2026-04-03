<script lang="ts">
  /**
   * Visualizer — floating draggable widget that hosts either the
   * SpectrumAnalyzer or the WaveformWidget.
   *
   * Visibility and position preferences are managed by visualizerStore and
   * persisted to localStorage so they survive page reloads.
   *
   * Accessibility:
   *   - The drag handle is keyboard-focusable; arrow keys nudge the widget.
   *   - Escape closes the widget (same as the toggle button in BottomBar).
   *   - All interactive controls have aria-label attributes.
   *   - The canvas has role="img" with a descriptive aria-label.
   */
  import { onMount, onDestroy } from "svelte";
  import SpectrumAnalyzer from "./SpectrumAnalyzer.svelte";
  import WaveformWidget from "./WaveformWidget.svelte";
  import TrackWaveform from "./TrackWaveform.svelte";
  import Spectrogram from "./Spectrogram.svelte";
  import { visualizerStore } from "$lib/stores/player/visualizer";
  import type {
    VisualizerPosition,
    VisualizerColorScheme,
    VisualizerType,
  } from "$lib/stores/player/visualizer";

  // ---- dimensions -----------------------------------------------------------
  const WIDGET_W = 300;
  // Canvas heights per type; total widget height = canvas + drag handle (20) + controls (28)
  const CANVAS_HEIGHTS: Record<VisualizerType, number> = {
    spectrum: 72,
    waveform: 72,
    "track-waveform": 80,
    spectrogram: 140,
  };
  $: canvasH = CANVAS_HEIGHTS[state.type] ?? 72;
  $: WIDGET_H = canvasH + 20 + 28;

  // ---- position helpers -----------------------------------------------------
  /**
   * Compute pixel left / top for the widget given a position preset +
   * the current window size.
   */
  function presetOrigin(
    preset: VisualizerPosition,
    ww: number,
    wh: number,
    margin: number,
    widgetH: number,
  ): { left: number; top: number } {
    // Reserve bottom-bar height (CSS var fallback to 64px) so the widget
    // doesn't overlap the playback bar.
    const barH =
      typeof document !== "undefined"
        ? parseInt(
            getComputedStyle(document.documentElement).getPropertyValue(
              "--bottom-h",
            ),
            10,
          ) || 64
        : 64;

    switch (preset) {
      case "top-left":
        return { left: margin, top: margin };
      case "top-center":
        return { left: (ww - WIDGET_W) / 2, top: margin };
      case "top-right":
        return { left: ww - WIDGET_W - margin, top: margin };
      case "bottom-left":
        return { left: margin, top: wh - barH - widgetH - margin };
      case "bottom-center":
        return { left: (ww - WIDGET_W) / 2, top: wh - barH - widgetH - margin };
      case "bottom-right":
      default:
        return {
          left: ww - WIDGET_W - margin,
          top: wh - barH - widgetH - margin,
        };
    }
  }

  function clamp(val: number, lo: number, hi: number) {
    return Math.max(lo, Math.min(hi, val));
  }

  // ---- component state ------------------------------------------------------
  $: state = $visualizerStore;

  let ww = typeof window !== "undefined" ? window.innerWidth : 1280;
  let wh = typeof window !== "undefined" ? window.innerHeight : 800;

  // Current pixel position (origin + drag offset, clamped to viewport).
  $: rawOrigin = presetOrigin(state.position, ww, wh, 12, WIDGET_H);
  $: left = clamp(rawOrigin.left + state.dragOffset.x, 0, ww - WIDGET_W);
  $: top = clamp(rawOrigin.top + state.dragOffset.y, 0, wh - WIDGET_H);

  // ---- drag logic -----------------------------------------------------------
  let dragging = false;
  let dragStart = { x: 0, y: 0 };
  let originAtDragStart = { x: 0, y: 0 };

  function onPointerDown(e: PointerEvent) {
    if (e.button !== 0) return;
    dragging = true;
    dragStart = { x: e.clientX, y: e.clientY };
    originAtDragStart = { x: left, y: top };
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    e.preventDefault();
  }

  function onPointerMove(e: PointerEvent) {
    if (!dragging) return;
    const dx = e.clientX - dragStart.x;
    const dy = e.clientY - dragStart.y;
    const newLeft = clamp(originAtDragStart.x + dx, 0, ww - WIDGET_W);
    const newTop = clamp(originAtDragStart.y + dy, 0, wh - WIDGET_H);
    // Store new drag offset relative to the current preset origin.
    const origin = presetOrigin(state.position, ww, wh, 12, WIDGET_H);
    visualizerStore.setDragOffset(newLeft - origin.left, newTop - origin.top);
  }

  function onPointerUp() {
    dragging = false;
  }

  // Keyboard nudge on the drag handle.
  function onHandleKeydown(e: KeyboardEvent) {
    const STEP = 16;
    let dx = 0;
    let dy = 0;
    if (e.key === "ArrowLeft") {
      dx = -STEP;
      e.preventDefault();
    }
    if (e.key === "ArrowRight") {
      dx = STEP;
      e.preventDefault();
    }
    if (e.key === "ArrowUp") {
      dy = -STEP;
      e.preventDefault();
    }
    if (e.key === "ArrowDown") {
      dy = STEP;
      e.preventDefault();
    }
    if (e.key === "Escape") {
      visualizerStore.setVisible(false);
      return;
    }
    if (dx !== 0 || dy !== 0) {
      const newLeft = clamp(left + dx, 0, ww - WIDGET_W);
      const newTop = clamp(top + dy, 0, wh - WIDGET_H);
      const origin = presetOrigin(state.position, ww, wh, 12, WIDGET_H);
      visualizerStore.setDragOffset(newLeft - origin.left, newTop - origin.top);
    }
  }

  // ---- global ESC handler ---------------------------------------------------
  function onWindowKeydown(e: KeyboardEvent) {
    if (e.key === "Escape" && state.visible) {
      visualizerStore.setVisible(false);
    }
  }

  // ---- resize handling ------------------------------------------------------
  function onResize() {
    ww = window.innerWidth;
    wh = window.innerHeight;
  }

  onMount(() => {
    window.addEventListener("resize", onResize);
    window.addEventListener("keydown", onWindowKeydown);
  });

  onDestroy(() => {
    window.removeEventListener("resize", onResize);
    window.removeEventListener("keydown", onWindowKeydown);
  });

  // ---- option cycling -------------------------------------------------------
  const TYPES: VisualizerType[] = [
    "spectrum",
    "waveform",
    "track-waveform",
    "spectrogram",
  ];
  const SCHEMES: VisualizerColorScheme[] = ["accent", "rainbow", "mono"];
  const POSITIONS: VisualizerPosition[] = [
    "bottom-right",
    "bottom-center",
    "bottom-left",
    "top-right",
    "top-center",
    "top-left",
  ];

  function cycleType() {
    const idx = TYPES.indexOf(state.type);
    visualizerStore.setType(TYPES[(idx + 1) % TYPES.length]);
  }

  function cycleColor() {
    const idx = SCHEMES.indexOf(state.colorScheme);
    visualizerStore.setColorScheme(SCHEMES[(idx + 1) % SCHEMES.length]);
  }

  function cyclePosition() {
    const idx = POSITIONS.indexOf(state.position);
    visualizerStore.setPosition(POSITIONS[(idx + 1) % POSITIONS.length]);
  }

  const TYPE_LABELS: Record<VisualizerType, string> = {
    spectrum: "Spectrum",
    waveform: "Waveform",
    "track-waveform": "Song Wave",
    spectrogram: "Spek",
  };
  const SCHEME_LABELS: Record<VisualizerColorScheme, string> = {
    accent: "Accent",
    rainbow: "Rainbow",
    mono: "Mono",
  };
</script>

{#if state.visible}
  <div
    class="viz-widget"
    style="left:{left}px;top:{top}px;"
    role="region"
    aria-label="Sound visualizer"
    aria-live="off"
  >
    <!-- Drag handle --------------------------------------------------------->
    <div
      class="viz-handle"
      tabindex="0"
      role="slider"
      aria-label="Drag to move visualizer. Use arrow keys to nudge."
      aria-valuenow={0}
      on:pointerdown={onPointerDown}
      on:pointermove={onPointerMove}
      on:pointerup={onPointerUp}
      on:pointercancel={onPointerUp}
      on:keydown={onHandleKeydown}
    >
      <svg width="12" height="8" viewBox="0 0 12 8" aria-hidden="true">
        <rect
          x="0"
          y="0"
          width="12"
          height="1.5"
          rx="0.75"
          fill="currentColor"
        />
        <rect
          x="0"
          y="3.25"
          width="12"
          height="1.5"
          rx="0.75"
          fill="currentColor"
        />
        <rect
          x="0"
          y="6.5"
          width="12"
          height="1.5"
          rx="0.75"
          fill="currentColor"
        />
      </svg>
    </div>

    <!-- Close button --------------------------------------------------------->
    <button
      class="viz-close"
      on:click={() => visualizerStore.setVisible(false)}
      aria-label="Close visualizer"
      title="Close">✕</button
    >

    <!-- Canvas area ---------------------------------------------------------->
    <div class="viz-canvas-wrap">
      {#if state.type === "spectrum"}
        <SpectrumAnalyzer
          width={WIDGET_W}
          height={canvasH}
          colorScheme={state.colorScheme}
        />
      {:else if state.type === "track-waveform"}
        <TrackWaveform
          width={WIDGET_W}
          height={canvasH}
          colorScheme={state.colorScheme}
        />
      {:else if state.type === "spectrogram"}
        <Spectrogram
          width={WIDGET_W}
          height={canvasH}
          colorScheme={state.colorScheme}
        />
      {:else}
        <WaveformWidget
          width={WIDGET_W}
          height={canvasH}
          colorScheme={state.colorScheme}
        />
      {/if}
    </div>

    <!-- Controls row --------------------------------------------------------->
    <div class="viz-controls" role="toolbar" aria-label="Visualizer controls">
      <button
        class="viz-ctrl-btn"
        on:click={cycleType}
        title="Switch visualizer type"
        aria-label="Visualizer type: {TYPE_LABELS[state.type]}. Click to cycle."
      >
        {TYPE_LABELS[state.type]}
      </button>

      <button
        class="viz-ctrl-btn"
        on:click={cycleColor}
        title="Switch colour scheme"
        aria-label="Colour: {SCHEME_LABELS[state.colorScheme]}. Click to cycle."
      >
        <span class="color-dot" data-scheme={state.colorScheme}></span>
        {SCHEME_LABELS[state.colorScheme]}
      </button>

      <button
        class="viz-ctrl-btn"
        on:click={cyclePosition}
        title="Snap to next position preset"
        aria-label="Position: {state.position}. Click to cycle."
      >
        <svg
          width="12"
          height="12"
          viewBox="0 0 16 16"
          fill="none"
          stroke="currentColor"
          stroke-width="1.8"
          aria-hidden="true"
        >
          <rect x="1" y="1" width="14" height="14" rx="2" />
          <circle
            cx={state.position.includes("left")
              ? 4
              : state.position.includes("right")
                ? 12
                : 8}
            cy={state.position.includes("top") ? 4 : 12}
            r="2"
            fill="currentColor"
            stroke="none"
          />
        </svg>
      </button>
    </div>
  </div>
{/if}

<style>
  .viz-widget {
    position: fixed;
    z-index: 9000;
    width: 300px;
    background: var(--bg-elevated, #1c1c1e);
    border: 1px solid var(--border, rgba(255, 255, 255, 0.08));
    border-radius: 8px;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
    overflow: hidden;
    display: flex;
    flex-direction: column;
    /* Smooth drag / position changes */
    will-change: transform;
    animation: viz-appear 0.18s cubic-bezier(0.4, 0, 0.2, 1);
  }

  @keyframes viz-appear {
    from {
      opacity: 0;
      transform: scale(0.93);
    }
    to {
      opacity: 1;
      transform: scale(1);
    }
  }

  /* ---- drag handle ---- */
  .viz-handle {
    width: 100%;
    height: 20px;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: grab;
    color: var(--text-muted, rgba(255, 255, 255, 0.35));
    background: var(--bg-elevated, #1c1c1e);
    user-select: none;
    touch-action: none;
    outline: none;
    transition: background 0.1s;
  }
  .viz-handle:active {
    cursor: grabbing;
  }
  .viz-handle:focus-visible {
    box-shadow: inset 0 0 0 2px var(--accent, #5b8dee);
    background: var(--bg-hover, rgba(255, 255, 255, 0.06));
  }
  .viz-handle:hover {
    background: var(--bg-hover, rgba(255, 255, 255, 0.06));
  }

  /* ---- close button ---- */
  .viz-close {
    position: absolute;
    top: 2px;
    right: 4px;
    background: none;
    border: none;
    color: var(--text-muted, rgba(255, 255, 255, 0.35));
    cursor: pointer;
    font-size: 0.7rem;
    line-height: 1;
    padding: 2px 4px;
    border-radius: 3px;
    transition:
      color 0.12s,
      background 0.12s;
    z-index: 1;
  }
  .viz-close:hover {
    color: var(--text, #fff);
    background: rgba(255, 255, 255, 0.06);
  }
  .viz-close:focus-visible {
    outline: 2px solid var(--accent, #5b8dee);
  }

  /* ---- canvas wrapper ---- */
  .viz-canvas-wrap {
    background: var(--bg, #111);
    line-height: 0; /* remove inline-block gap */
  }

  /* ---- controls row ---- */
  .viz-controls {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 4px 8px;
    background: var(--bg-elevated, #1c1c1e);
    border-top: 1px solid var(--border, rgba(255, 255, 255, 0.06));
  }

  .viz-ctrl-btn {
    flex: 1;
    background: none;
    border: 1px solid transparent;
    border-radius: 4px;
    color: var(--text-muted, rgba(255, 255, 255, 0.5));
    font-size: 0.68rem;
    cursor: pointer;
    padding: 3px 4px;
    transition:
      color 0.12s,
      border-color 0.12s,
      background 0.12s;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    white-space: nowrap;
    min-width: 0;
  }
  .viz-ctrl-btn:hover {
    color: var(--text, #fff);
    border-color: var(--border, rgba(255, 255, 255, 0.15));
    background: var(--bg-hover, rgba(255, 255, 255, 0.06));
  }
  .viz-ctrl-btn:focus-visible {
    outline: 2px solid var(--accent, #5b8dee);
  }

  /* colour dot */
  .color-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex-shrink: 0;
  }
  .color-dot[data-scheme="accent"] {
    background: var(--accent, #5b8dee);
  }
  .color-dot[data-scheme="rainbow"] {
    background: linear-gradient(90deg, #f33, #fa0, #3f3, #39f, #93f);
  }
  .color-dot[data-scheme="mono"] {
    background: #aaa;
  }

  /* ---- responsive: hide on very narrow viewports ---- */
  @media (max-width: 480px) {
    .viz-widget {
      width: calc(100vw - 24px);
    }
  }
</style>
