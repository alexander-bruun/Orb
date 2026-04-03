<script lang="ts">
  /**
   * TrackWaveform — full-song Audacity-style waveform overview.
   *
   * Reads pre-computed peak data from the waveformPeaks store (computed once
   * per track from the decoded PCM) and renders a symmetric bar chart where:
   *   • The portion LEFT of the playhead is filled with the accent colour.
   *   • The portion RIGHT of the playhead is muted (dim colour).
   *   • A thin bright cursor marks the exact playhead position.
   *
   * The transparent <input type="range"> overlay handles seeking and
   * provides keyboard accessibility (← → keys and screen-reader labelling)
   * at no visual cost.
   */
  import { onMount, onDestroy } from "svelte";
  import {
    waveformPeaks,
    waveformLoading,
  } from "$lib/stores/player/waveformPeaks";
  import { positionMs, durationMs, seek } from "$lib/stores/player";

  export let colorScheme: "accent" | "rainbow" | "mono" = "accent";
  export let width = 300;
  export let height = 80;

  let canvas: HTMLCanvasElement | null = null;
  const unsubs: (() => void)[] = [];

  // ── animation state ──────────────────────────────────────────────────────────
  let animRaf = 0;
  let shimmerPhase = 0; // 0..1, advances during the placeholder phase
  let transitionT = 1; // 0 → 1: blends placeholder silhouette → real peaks
  let transitionStart = 0;
  let hadPeaks = false;
  const TRANSITION_MS = 700;

  // ── cursor smoothing state ───────────────────────────────────────────────────
  let smoothPct = 0; // visual cursor position (0..1), animated on seeks
  let prevPct = 0; // last known raw position — used to detect seek jumps
  let cursorAnimFrom = 0;
  let cursorAnimTarget = 0;
  let cursorAnimStart = 0;
  let cursorAnimRaf = 0;
  const CURSOR_ANIM_MS = 350;
  const SEEK_THRESHOLD = 0.008; // ~0.8% of track — larger = normal playback advance

  // ── colour helpers ──────────────────────────────────────────────────────────

  function accentColor(): string {
    return typeof document !== "undefined"
      ? getComputedStyle(document.documentElement)
          .getPropertyValue("--accent")
          .trim() || "#5b8dee"
      : "#5b8dee";
  }

  function barColor(t: number, played: boolean, peak: number): string {
    if (colorScheme === "rainbow") {
      const l = played ? 60 : 28;
      return `hsl(${t * 270}, 70%, ${l}%)`;
    }
    if (colorScheme === "mono") {
      return played
        ? `rgba(200,200,200,${(0.45 + 0.55 * peak).toFixed(2)})`
        : `rgba(80,80,80,0.45)`;
    }
    // accent
    return played ? accentColor() : "rgba(120,120,120,0.28)";
  }

  // ── animation helpers ────────────────────────────────────────────────────────

  function easeOutCubic(t: number): number {
    return 1 - Math.pow(1 - Math.min(t, 1), 3);
  }

  /** Normalized placeholder bar height at position t ∈ [0,1] — same twin-sine
   *  formula used in drawPlaceholder, extracted so draw() can blend against it. */
  function placeholderPeakAt(t: number): number {
    return (
      Math.abs(Math.sin(t * Math.PI * 6)) * 0.4 +
      Math.abs(Math.sin(t * Math.PI * 13)) * 0.25 +
      0.05
    );
  }

  function stopAnim() {
    if (animRaf) {
      cancelAnimationFrame(animRaf);
      animRaf = 0;
    }
  }

  function stopCursorAnim() {
    if (cursorAnimRaf) {
      cancelAnimationFrame(cursorAnimRaf);
      cursorAnimRaf = 0;
    }
  }

  function startCursorAnim(from: number, to: number) {
    stopCursorAnim();
    cursorAnimFrom = from;
    cursorAnimTarget = to;
    cursorAnimStart = 0;
    const loop = (now: number) => {
      if (!cursorAnimStart) cursorAnimStart = now;
      const t = Math.min((now - cursorAnimStart) / CURSOR_ANIM_MS, 1);
      smoothPct =
        cursorAnimFrom + (cursorAnimTarget - cursorAnimFrom) * easeOutCubic(t);
      // Only draw directly if the main waveform animation isn't already looping.
      if (!animRaf) draw();
      if (t < 1) {
        cursorAnimRaf = requestAnimationFrame(loop);
      } else {
        smoothPct = cursorAnimTarget;
        cursorAnimRaf = 0;
        if (!animRaf) draw();
      }
    };
    cursorAnimRaf = requestAnimationFrame(loop);
  }

  /** Called when positionMs changes. Animates the cursor on large jumps (seeks). */
  function handlePositionChange() {
    const dur = $durationMs;
    const pos = $positionMs;
    const newPct = dur > 0 ? Math.min(pos / dur, 1) : 0;

    const delta = Math.abs(newPct - prevPct);
    prevPct = newPct;

    if (delta > SEEK_THRESHOLD) {
      // Big jump — user or programmatic seek: animate the cursor.
      startCursorAnim(smoothPct, newPct);
    } else {
      // Normal playback tick — follow directly without animation.
      stopCursorAnim();
      smoothPct = newPct;
      if (!animRaf) draw();
    }
  }

  function startShimmer() {
    if (animRaf) return; // already looping
    const loop = (now: number) => {
      shimmerPhase = (now / 1800) % 1;
      draw();
      animRaf = requestAnimationFrame(loop);
    };
    animRaf = requestAnimationFrame(loop);
  }

  function startTransition() {
    stopAnim();
    transitionT = 0;
    transitionStart = 0;
    const loop = (now: number) => {
      if (!transitionStart) transitionStart = now;
      transitionT = Math.min((now - transitionStart) / TRANSITION_MS, 1);
      draw();
      if (transitionT < 1) {
        animRaf = requestAnimationFrame(loop);
      } else {
        animRaf = 0;
      }
    };
    animRaf = requestAnimationFrame(loop);
  }

  /** Called whenever any relevant store changes. Routes to the right animation. */
  function handleStoreChange() {
    const loading = $waveformLoading;
    const peaks = $waveformPeaks?.peaks ?? null;

    if (!peaks) {
      if (hadPeaks) hadPeaks = false;
      if (loading) {
        startShimmer();
      } else {
        stopAnim();
        draw();
      }
    } else {
      if (!hadPeaks) {
        hadPeaks = true;
        startTransition();
      } else if (!animRaf) {
        draw();
      }
    }
  }

  // ── drawing ─────────────────────────────────────────────────────────────────

  function draw() {
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const dpr = window.devicePixelRatio ?? 1;
    const physW = Math.round(width * dpr);
    const physH = Math.round(height * dpr);

    // Resize backing store when needed.
    if (canvas.width !== physW || canvas.height !== physH) {
      canvas.width = physW;
      canvas.height = physH;
    }

    ctx.clearRect(0, 0, physW, physH);

    const peaks = $waveformPeaks?.peaks ?? null;
    const loading = $waveformLoading;
    const pct = smoothPct;
    const cursorPx = Math.round(pct * physW);
    const cy = physH / 2;
    const maxBarH = Math.max(1, (physH - 4 * dpr) / 2);

    // Pure placeholder while loading with no peaks yet.
    if (loading && !peaks) {
      drawPlaceholder(ctx, physW, physH, shimmerPhase);
      return;
    }

    if (!peaks) {
      // Silent / no data: flat centre line.
      ctx.fillStyle = "rgba(120,120,120,0.2)";
      ctx.fillRect(0, cy - 1, physW, 2);
      return;
    }

    // ── draw bars — blends placeholder silhouette → real peaks ────────────
    const blend = easeOutCubic(transitionT);

    for (let px = 0; px < physW; px++) {
      const t = px / physW;
      const idx = Math.min(Math.floor(t * peaks.length), peaks.length - 1);
      const realPk = peaks[idx];
      // Morphs shape from placeholder → real while simultaneously fading in.
      const peak =
        blend < 1
          ? placeholderPeakAt(t) + (realPk - placeholderPeakAt(t)) * blend
          : realPk;
      const barH = Math.max(1 * dpr, peak * maxBarH);
      const played = px < cursorPx;

      ctx.globalAlpha = blend < 1 ? 0.18 + 0.82 * blend : 1;
      ctx.fillStyle = barColor(t, played, realPk);
      ctx.fillRect(px, cy - barH, 1, barH * 2);
    }
    ctx.globalAlpha = 1;

    // ── cursor line ────────────────────────────────────────────────────────
    const accent = accentColor();
    ctx.fillStyle = accent;
    ctx.globalAlpha = 0.25;
    ctx.fillRect(Math.max(0, cursorPx - dpr), 0, dpr * 3, physH);
    ctx.globalAlpha = 1;
    ctx.fillStyle = colorScheme === "mono" ? "rgba(255,255,255,0.9)" : accent;
    ctx.fillRect(cursorPx, 0, Math.max(1, dpr), physH);

    // Dim overlay while peaks are still refining.
    if (loading) {
      ctx.fillStyle = "rgba(0,0,0,0.35)";
      ctx.fillRect(0, 0, physW, physH);
    }
  }

  /**
   * Placeholder bars drawn while the waveform is computing.
   * Uses the same twin-sine silhouette as placeholderPeakAt() with a
   * traveling shimmer sweep animated via the shimmer parameter (0..1).
   */
  function drawPlaceholder(
    ctx: CanvasRenderingContext2D,
    w: number,
    h: number,
    shimmer: number,
  ) {
    const cy = h / 2;
    const step = Math.ceil(w / 150);

    for (let i = 0, px = 0; px < w; i++, px += step) {
      const t = i / 150;
      const barH = placeholderPeakAt(t) * (h / 2 - 4);
      ctx.fillStyle = `rgba(120,120,120,${(0.1 + 0.05 * Math.sin(t * Math.PI)).toFixed(2)})`;
      ctx.fillRect(px, cy - barH, step - 1, barH * 2);
    }

    // Traveling shimmer sweep — a soft highlight that scrolls left→right.
    const sw = w * 0.28;
    const sx = shimmer * (w + sw) - sw;
    const grad = ctx.createLinearGradient(sx, 0, sx + sw, 0);
    grad.addColorStop(0, "rgba(255,255,255,0)");
    grad.addColorStop(0.35, "rgba(255,255,255,0.045)");
    grad.addColorStop(0.5, "rgba(255,255,255,0.085)");
    grad.addColorStop(0.65, "rgba(255,255,255,0.045)");
    grad.addColorStop(1, "rgba(255,255,255,0)");
    ctx.fillStyle = grad;
    ctx.fillRect(0, 0, w, h);
  }

  // ── seeking ─────────────────────────────────────────────────────────────────

  let rangeInput: HTMLInputElement | null = null;
  let pointerSeeking = false;

  function pctFromPointer(e: PointerEvent): number {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    return Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
  }

  function onPointerDown(e: PointerEvent) {
    pointerSeeking = true;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    const pct = pctFromPointer(e);
    seek(($durationMs / 1000) * pct);
    // Update cursor immediately — no animation while dragging.
    stopCursorAnim();
    smoothPct = pct;
    prevPct = pct;
    if (!animRaf) draw();
    if (rangeInput) rangeInput.value = String(pct * 100);
  }

  function onPointerMove(e: PointerEvent) {
    if (!pointerSeeking) return;
    const pct = pctFromPointer(e);
    seek(($durationMs / 1000) * pct);
    // Keep cursor locked to pointer during drag.
    smoothPct = pct;
    prevPct = pct;
    if (!animRaf) draw();
    if (rangeInput) rangeInput.value = String(pct * 100);
  }

  function onPointerUp() {
    pointerSeeking = false;
  }

  // Keyboard navigation via the hidden range input (arrow keys, a11y).
  function onSeekInput(e: Event) {
    const val = parseFloat((e.target as HTMLInputElement).value); // 0..100
    seek(($durationMs / 1000) * (val / 100));
  }

  // ── lifecycle ───────────────────────────────────────────────────────────────

  onMount(() => {
    unsubs.push(
      positionMs.subscribe(handlePositionChange),
      waveformPeaks.subscribe(handleStoreChange),
      waveformLoading.subscribe(handleStoreChange),
    );
    handleStoreChange(); // initial render / kick off shimmer if needed
  });

  onDestroy(() => {
    unsubs.forEach((u) => u());
    stopAnim();
    stopCursorAnim();
  });

  $: progress = $durationMs > 0 ? ($positionMs / $durationMs) * 100 : 0;
</script>

<div class="wf-root" style="width:{width}px;height:{height}px;">
  <figure
    class="wf-figure"
    aria-label="Track waveform. Use the seek slider below to navigate."
    style="width:{width}px;height:{height}px;margin:0;"
  >
    <canvas bind:this={canvas} style="width:{width}px;height:{height}px;"
    ></canvas>
  </figure>

  <!--
    Transparent range input layered on top for click-to-seek and keyboard nav.
    Screen readers announce it as a seek slider; arrow keys work out of the box.
  -->
  <input
    bind:this={rangeInput}
    type="range"
    class="wf-seek"
    min="0"
    max="100"
    step="0.01"
    value={progress}
    on:input={onSeekInput}
    on:pointerdown={onPointerDown}
    on:pointermove={onPointerMove}
    on:pointerup={onPointerUp}
    aria-label="Seek"
    title="Click or drag to seek"
  />
</div>

<style>
  .wf-root {
    position: relative;
    border-radius: 4px;
    overflow: hidden;
    transition: box-shadow 0.12s;
  }
  .wf-root:has(.wf-seek:focus-visible) {
    box-shadow: 0 0 0 2px var(--accent, #5b8dee);
  }

  .wf-figure {
    display: block;
    border-radius: 4px;
    overflow: hidden;
    line-height: 0;
  }

  canvas {
    display: block;
  }

  .wf-root {
    cursor: pointer;
  }

  /* Range input is invisible and ignores pointer events — used only for
     keyboard navigation (arrow keys) and screen-reader accessibility.
     All mouse/touch seeking goes through the pointerdown/move/up handlers
     on the container, which gives pixel-perfect position mapping. */
  .wf-seek {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    margin: 0;
    opacity: 0;
    pointer-events: auto;
    cursor: pointer;
    -webkit-appearance: none;
    appearance: none;
  }
  /* Keep the input itself focusable via keyboard but invisible — the wrapper
     .wf-focused class renders the visible focus ring instead. */
  .wf-seek:focus {
    outline: none;
  }
  .wf-seek:focus-visible {
    outline: none;
  }
</style>
