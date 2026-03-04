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
  import { onMount, onDestroy } from 'svelte';
  import { waveformPeaks, waveformLoading } from '$lib/stores/waveformPeaks';
  import { positionMs, durationMs, seek } from '$lib/stores/player';

  export let colorScheme: 'accent' | 'rainbow' | 'mono' = 'accent';
  export let width = 300;
  export let height = 80;

  let canvas: HTMLCanvasElement | null = null;
  let focused = false;
  const unsubs: (() => void)[] = [];

  // ── colour helpers ──────────────────────────────────────────────────────────

  function accentColor(): string {
    return typeof document !== 'undefined'
      ? getComputedStyle(document.documentElement)
          .getPropertyValue('--accent').trim() || '#5b8dee'
      : '#5b8dee';
  }

  function barColor(t: number, played: boolean, peak: number): string {
    if (colorScheme === 'rainbow') {
      const l = played ? 60 : 28;
      return `hsl(${t * 270}, 70%, ${l}%)`;
    }
    if (colorScheme === 'mono') {
      return played
        ? `rgba(200,200,200,${(0.45 + 0.55 * peak).toFixed(2)})`
        : `rgba(80,80,80,0.45)`;
    }
    // accent
    return played ? accentColor() : 'rgba(120,120,120,0.28)';
  }

  // ── drawing ─────────────────────────────────────────────────────────────────

  function draw() {
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const dpr   = window.devicePixelRatio ?? 1;
    const physW = Math.round(width  * dpr);
    const physH = Math.round(height * dpr);

    // Resize backing store when needed.
    if (canvas.width !== physW || canvas.height !== physH) {
      canvas.width  = physW;
      canvas.height = physH;
    }

    ctx.clearRect(0, 0, physW, physH);

    const peaks      = $waveformPeaks?.peaks ?? null;
    const loading    = $waveformLoading;
    const dur        = $durationMs;
    const pos        = $positionMs;
    const pct        = dur > 0 ? Math.min(pos / dur, 1) : 0;
    const cursorPx   = Math.round(pct * physW);
    const cy         = physH / 2;
    const maxBarH    = Math.max(1, (physH - 4 * dpr) / 2);

    if (loading && !peaks) {
      drawPlaceholder(ctx, physW, physH);
      return;
    }

    if (!peaks) {
      // Silent / no data: flat centre line.
      ctx.fillStyle = 'rgba(120,120,120,0.2)';
      ctx.fillRect(0, cy - 1, physW, 2);
      return;
    }

    // ── draw bars (1 physical pixel per column) ───────────────────────────
    for (let px = 0; px < physW; px++) {
      const t       = px / physW;                                          // 0..1
      const idx     = Math.min(Math.floor(t * peaks.length), peaks.length - 1);
      const peak    = peaks[idx];
      const barH    = Math.max(1 * dpr, peak * maxBarH);
      const played  = px < cursorPx;

      ctx.fillStyle = barColor(t, played, peak);
      ctx.fillRect(px, cy - barH, 1, barH * 2);
    }

    // ── cursor line ────────────────────────────────────────────────────────
    const accent = accentColor();
    // Glow pass (wider, low opacity)
    ctx.fillStyle = accent;
    ctx.globalAlpha = 0.25;
    ctx.fillRect(Math.max(0, cursorPx - dpr), 0, dpr * 3, physH);
    // Sharp line
    ctx.globalAlpha = 1;
    ctx.fillStyle = colorScheme === 'mono' ? 'rgba(255,255,255,0.9)' : accent;
    ctx.fillRect(cursorPx, 0, Math.max(1, dpr), physH);

    // ── loading shimmer overlay while peaks are still computing ───────────
    if (loading) {
      ctx.fillStyle = 'rgba(0,0,0,0.35)';
      ctx.fillRect(0, 0, physW, physH);
    }
  }

  /**
   * Placeholder bars drawn while the waveform is being computed.
   * A simple soft pattern so the widget doesn't look empty.
   */
  function drawPlaceholder(ctx: CanvasRenderingContext2D, w: number, h: number) {
    const cy   = h / 2;
    const step = Math.ceil(w / 150);

    for (let i = 0, px = 0; px < w; i++, px += step) {
      const t    = i / 150;
      // Two overlapping sine waves for a vaguely musical silhouette.
      const barH = (Math.abs(Math.sin(t * Math.PI * 6)) * 0.4 +
                    Math.abs(Math.sin(t * Math.PI * 13)) * 0.25 + 0.05)
                   * (h / 2 - 4);
      ctx.fillStyle = `rgba(120,120,120,${(0.10 + 0.05 * Math.sin(t * Math.PI)).toFixed(2)})`;
      ctx.fillRect(px, cy - barH, step - 1, barH * 2);
    }
  }

  // ── seeking ─────────────────────────────────────────────────────────────────

  function onSeekInput(e: Event) {
    const val = parseFloat((e.target as HTMLInputElement).value); // 0..100
    seek(($durationMs / 1000) * (val / 100));
  }

  // ── lifecycle ───────────────────────────────────────────────────────────────

  onMount(() => {
    unsubs.push(
      positionMs.subscribe(draw),
      waveformPeaks.subscribe(draw),
      waveformLoading.subscribe(draw),
    );
    draw(); // initial render
  });

  onDestroy(() => {
    unsubs.forEach((u) => u());
  });

  $: progress = $durationMs > 0 ? ($positionMs / $durationMs) * 100 : 0;
</script>

<div
  class="wf-root"
  class:wf-focused={focused}
  style="width:{width}px;height:{height}px;"
>
  <figure
    class="wf-figure"
    aria-label="Track waveform. Use the seek slider below to navigate."
    style="width:{width}px;height:{height}px;margin:0;"
  >
    <canvas bind:this={canvas} style="width:{width}px;height:{height}px;"></canvas>
  </figure>

  <!--
    Transparent range input layered on top for click-to-seek and keyboard nav.
    Screen readers announce it as a seek slider; arrow keys work out of the box.
  -->
  <input
    type="range"
    class="wf-seek"
    min="0"
    max="100"
    step="0.01"
    value={progress}
    on:input={onSeekInput}
    on:focus={() => (focused = true)}
    on:blur={() => (focused = false)}
    aria-label="Seek"
    title="Click or drag to seek"
  />
</div>

<style>
  .wf-root {
    position: relative;
    border-radius: 4px;
    overflow: hidden;
    /* Focus ring on the wrapper when the hidden input is focused */
    transition: box-shadow 0.12s;
  }
  .wf-root.wf-focused {
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

  /* Invisible range input sits exactly on top of the canvas */
  .wf-seek {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    margin: 0;
    opacity: 0;
    cursor: pointer;
    -webkit-appearance: none;
    appearance: none;
  }
  /* Keep the input itself focusable via keyboard but invisible — the wrapper
     .wf-focused class renders the visible focus ring instead. */
  .wf-seek:focus { outline: none; }
</style>
