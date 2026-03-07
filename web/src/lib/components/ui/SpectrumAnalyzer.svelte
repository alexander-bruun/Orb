<script lang="ts">
  /**
   * SpectrumAnalyzer
   *
   * A real-time FFT spectrum analyzer rendered on an HTML canvas.
   * Reads frequency-domain data from the Web Audio AnalyserNode exposed by
   * AudioEngine.getAnalyser(). When no analyser is available (e.g. native
   * path not yet wired) the canvas shows a silent flat baseline.
   *
   * Props:
   *   colorScheme  – 'accent' | 'rainbow' | 'mono'
   *   width        – canvas logical width  (default 300)
   *   height       – canvas logical height (default 80)
   */
  import { onMount, onDestroy } from 'svelte';
  import { audioEngine } from '$lib/audio/engine';

  export let colorScheme: 'accent' | 'rainbow' | 'mono' = 'accent';
  export let width = 300;
  export let height = 80;

  let canvas: HTMLCanvasElement | null = null;
  let rafId: number | null = null;

  // Bar count — 64 gives a nice dense look without being too heavy.
  const BAR_COUNT = 64;
  const BAR_GAP = 1;

  function getBarColor(i: number, value: number, ctx2d: CanvasRenderingContext2D): string | CanvasGradient {
    if (colorScheme === 'rainbow') {
      return `hsl(${(i / BAR_COUNT) * 270}, 80%, 60%)`;
    }
    if (colorScheme === 'mono') {
      const alpha = 0.4 + 0.6 * (value / 255);
      return `rgba(200, 200, 200, ${alpha.toFixed(2)})`;
    }
    // 'accent' — read the CSS variable so it always matches the theme.
    const accentHex = typeof document !== 'undefined'
      ? getComputedStyle(document.documentElement).getPropertyValue('--accent').trim() || '#5b8dee'
      : '#5b8dee';
    const grad = ctx2d.createLinearGradient(0, height, 0, 0);
    grad.addColorStop(0, accentHex);
    grad.addColorStop(1, accentHex + '66'); // 40% alpha at top
    return grad;
  }

  function draw() {
    if (!canvas) { rafId = requestAnimationFrame(draw); return; }
    const ctx2d = canvas.getContext('2d');
    if (!ctx2d) { rafId = requestAnimationFrame(draw); return; }

    const analyser = audioEngine.getAnalyser();

    // Scale the canvas to the actual device pixel ratio once.
    const dpr = window.devicePixelRatio ?? 1;
    if (canvas.width !== width * dpr) {
      canvas.width = width * dpr;
      canvas.height = height * dpr;
      ctx2d.scale(dpr, dpr);
    }

    ctx2d.clearRect(0, 0, width, height);

    if (!analyser) {
      // Draw flat baseline when no audio data is available.
      ctx2d.fillStyle = 'rgba(120,120,120,0.15)';
      ctx2d.fillRect(0, height - 2, width, 2);
      rafId = requestAnimationFrame(draw);
      return;
    }

    const binCount = analyser.frequencyBinCount; // fftSize / 2 = 1024
    const dataArray = new Uint8Array(binCount);
    analyser.getByteFrequencyData(dataArray);

    // Map BAR_COUNT logarithmic-ish bins across [0, binCount/2] so low-freq
    // detail is not crushed. Limiting to binCount/2 skips the very high
    // frequencies that are rarely interesting for music.
    const maxBin = Math.floor(binCount * 0.5);
    const barW = (width - BAR_GAP * (BAR_COUNT - 1)) / BAR_COUNT;

    for (let i = 0; i < BAR_COUNT; i++) {
      // Map bar index to a bin range using a logarithmic scale.
      const t0 = Math.pow(i / BAR_COUNT, 1.6);
      const t1 = Math.pow((i + 1) / BAR_COUNT, 1.6);
      const binStart = Math.floor(t0 * maxBin);
      const binEnd   = Math.max(binStart + 1, Math.floor(t1 * maxBin));

      // Average the bins in this range.
      let sum = 0;
      for (let b = binStart; b < binEnd; b++) sum += dataArray[b];
      const avg = sum / (binEnd - binStart);

      const barH = Math.max(2, (avg / 255) * height);
      const x = i * (barW + BAR_GAP);
      const y = height - barH;

      ctx2d.fillStyle = getBarColor(i, avg, ctx2d);
      // Rounded top cap: draw a rect then a small arc.
      const radius = Math.min(barW / 2, 2);
      ctx2d.beginPath();
      ctx2d.moveTo(x + radius, y);
      ctx2d.lineTo(x + barW - radius, y);
      ctx2d.arcTo(x + barW, y, x + barW, y + radius, radius);
      ctx2d.lineTo(x + barW, y + barH);
      ctx2d.lineTo(x, y + barH);
      ctx2d.lineTo(x, y + radius);
      ctx2d.arcTo(x, y, x + radius, y, radius);
      ctx2d.closePath();
      ctx2d.fill();
    }

    rafId = requestAnimationFrame(draw);
  }

  onMount(() => {
    rafId = requestAnimationFrame(draw);
  });

  onDestroy(() => {
    if (rafId !== null) cancelAnimationFrame(rafId);
  });
</script>

<figure
  class="viz-figure"
  aria-label="Real-time frequency spectrum analyzer"
  style="width:{width}px;height:{height}px;margin:0;"
>
  <canvas
    bind:this={canvas}
    style="width:{width}px;height:{height}px;"
  ></canvas>
</figure>

<style>
  .viz-figure canvas {
    display: block;
    border-radius: 4px;
  }
  .viz-figure {
    display: block;
  }
</style>
