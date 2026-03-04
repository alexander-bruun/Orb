<script lang="ts">
  /**
   * WaveformWidget
   *
   * A real-time oscilloscope-style waveform renderer (time-domain PCM data).
   * Uses the Web Audio AnalyserNode exposed by AudioEngine.getAnalyser().
   * Falls back gracefully to a silent centre-line when no analyser is available.
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

  function getStrokeColor(x: number, totalWidth: number): string {
    if (colorScheme === 'rainbow') {
      return `hsl(${(x / totalWidth) * 270}, 75%, 60%)`;
    }
    if (colorScheme === 'mono') {
      return 'rgba(200,200,200,0.85)';
    }
    // accent — CSS variable lookup
    return typeof document !== 'undefined'
      ? getComputedStyle(document.documentElement).getPropertyValue('--accent').trim() || '#5b8dee'
      : '#5b8dee';
  }

  // Smoothed data for a nicer visualisation — lerp between frames.
  let smoothed: Float32Array | null = null;
  const SMOOTH = 0.55; // mixing weight for new data vs. previous frame

  function draw() {
    if (!canvas) { rafId = requestAnimationFrame(draw); return; }
    const ctx2d = canvas.getContext('2d');
    if (!ctx2d) { rafId = requestAnimationFrame(draw); return; }

    const dpr = window.devicePixelRatio ?? 1;
    if (canvas.width !== width * dpr) {
      canvas.width = width * dpr;
      canvas.height = height * dpr;
      ctx2d.scale(dpr, dpr);
    }

    ctx2d.clearRect(0, 0, width, height);

    const analyser = audioEngine.getAnalyser();

    if (!analyser) {
      // Draw silent centre-line.
      ctx2d.strokeStyle = 'rgba(120,120,120,0.3)';
      ctx2d.lineWidth = 1.5;
      ctx2d.beginPath();
      ctx2d.moveTo(0, height / 2);
      ctx2d.lineTo(width, height / 2);
      ctx2d.stroke();
      rafId = requestAnimationFrame(draw);
      return;
    }

    const bufLen = analyser.fftSize; // time-domain buffer = fftSize (2048)
    const raw = new Uint8Array(bufLen);
    analyser.getByteTimeDomainData(raw);

    // Initialise or resize smoothed buffer.
    if (!smoothed || smoothed.length !== bufLen) {
      smoothed = new Float32Array(bufLen);
      for (let i = 0; i < bufLen; i++) smoothed[i] = raw[i];
    } else {
      for (let i = 0; i < bufLen; i++) {
        smoothed[i] = smoothed[i] * SMOOTH + raw[i] * (1 - SMOOTH);
      }
    }

    // Downsample to canvas pixel grid for performance.
    const step = Math.ceil(bufLen / width);

    if (colorScheme === 'rainbow') {
      // Segment-by-segment colour sweep.
      for (let i = 0; i < width - 1; i++) {
        const s0 = Math.floor(i * step);
        const s1 = Math.floor((i + 1) * step);
        const v0 = (smoothed[s0] / 128.0 - 1) * (height / 2);
        const v1 = (smoothed[s1] / 128.0 - 1) * (height / 2);
        ctx2d.strokeStyle = getStrokeColor(i, width);
        ctx2d.lineWidth = 1.5;
        ctx2d.beginPath();
        ctx2d.moveTo(i, height / 2 + v0);
        ctx2d.lineTo(i + 1, height / 2 + v1);
        ctx2d.stroke();
      }
    } else {
      // Single-pass path for non-rainbow modes (much cheaper).
      const accent = getStrokeColor(0, width);
      ctx2d.strokeStyle = accent;
      ctx2d.lineWidth = 1.5;
      ctx2d.lineJoin = 'round';
      ctx2d.lineCap = 'round';
      ctx2d.beginPath();

      for (let i = 0; i < width; i++) {
        const s = Math.floor(i * step);
        const v = (smoothed[s] / 128.0 - 1) * (height / 2);
        if (i === 0) {
          ctx2d.moveTo(i, height / 2 + v);
        } else {
          ctx2d.lineTo(i, height / 2 + v);
        }
      }
      ctx2d.stroke();

      // Soft glow: repeat the stroke with reduced opacity and wider line.
      ctx2d.globalAlpha = 0.15;
      ctx2d.lineWidth = 4;
      ctx2d.stroke();
      ctx2d.globalAlpha = 1;
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
  aria-label="Real-time audio waveform oscilloscope"
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
