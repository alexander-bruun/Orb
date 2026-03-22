<script lang="ts">
  /**
   * Spectrogram
   *
   * A Spek-like scrolling waterfall spectrogram rendered on an HTML canvas.
   *
   *   X-axis  = time (scrolls left; newest data on the right)
   *   Y-axis  = frequency, logarithmic scale (low freq at bottom, high at top)
   *   Colour  = amplitude / intensity (Spek/SOX palette by default)
   *
   * Left overlay  = kHz frequency axis labels
   * Right overlay = dB level axis labels
   *
   * Props:
   *   colorScheme  – 'accent' | 'rainbow' | 'mono'
   *   width        – logical canvas width  (default 300)
   *   height       – logical canvas height (default 140)
   */
  import { onMount, onDestroy } from 'svelte';
  import { audioEngine } from '$lib/audio/engine';

  export let colorScheme: 'accent' | 'rainbow' | 'mono' = 'accent';
  export let width  = 300;
  export let height = 140;

  let canvas: HTMLCanvasElement | null = null;
  let rafId: number | null = null;

  // Audible frequency range to display
  const MIN_FREQ = 20;
  const MAX_FREQ = 20000;

  // dB range — read from AnalyserNode; defaults match Web Audio API spec
  let minDb = -100;
  let maxDb = -30;
  let analyserSeeded = false;

  // ---- Axis label data -------------------------------------------------------

  const FREQ_TICKS_HZ = [20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 20000];

  function formatHz(hz: number): string {
    return hz >= 1000 ? `${hz / 1000}k` : `${hz}`;
  }

  const AXIS_PAD = 8; // px kept clear at top/bottom so edge labels are visible

  function freqToY(hz: number, h: number): number {
    const logMin  = Math.log10(MIN_FREQ);
    const logMax  = Math.log10(MAX_FREQ);
    const logFreq = Math.log10(Math.max(hz, MIN_FREQ));
    const t = (logFreq - logMin) / (logMax - logMin); // 0=low, 1=high
    return AXIS_PAD + (1 - t) * (h - 2 * AXIS_PAD);
  }

  /** Visible kHz tick labels, filtered to avoid crowding at edges. */
  $: freqLabels = FREQ_TICKS_HZ
    .map((hz) => ({ text: formatHz(hz), y: freqToY(hz, height) }))
    .filter((l) => l.y >= 7 && l.y <= height - 5);

  /** 5 evenly-spaced dB labels across the analyser range. */
  $: dbLabels = Array.from({ length: 5 }, (_, i) => {
    const db = maxDb - (i / 4) * (maxDb - minDb); // maxDb at top → minDb at bottom
    const t  = (db - minDb) / (maxDb - minDb);
    const y  = (1 - t) * height;
    return { text: `${Math.round(db)}`, y };
  }).filter((l) => l.y >= 7 && l.y <= height - 5);

  // ---- Color LUT (amplitude 0-255 → RGB) ------------------------------------

  function hslToRgb(h: number, s: number, l: number): [number, number, number] {
    h /= 360;
    const q = l < 0.5 ? l * (1 + s) : l + s - l * s;
    const p = 2 * l - q;
    function hue2rgb(t: number): number {
      if (t < 0) t += 1;
      if (t > 1) t -= 1;
      if (t < 1 / 6) return p + (q - p) * 6 * t;
      if (t < 1 / 2) return q;
      if (t < 2 / 3) return p + (q - p) * (2 / 3 - t) * 6;
      return p;
    }
    return [
      Math.round(hue2rgb(h + 1 / 3) * 255),
      Math.round(hue2rgb(h) * 255),
      Math.round(hue2rgb(h - 1 / 3) * 255),
    ];
  }

  function buildColorLUT(scheme: 'accent' | 'rainbow' | 'mono'): Uint8ClampedArray {
    const lut = new Uint8ClampedArray(256 * 3);
    for (let i = 0; i < 256; i++) {
      const t = i / 255;
      let r = 0, g = 0, b = 0;

      if (scheme === 'mono') {
        r = g = b = Math.round(t * 255);
      } else if (scheme === 'rainbow') {
        const hue = 240 - t * 240;
        [r, g, b] = hslToRgb(hue, 0.9, 0.05 + t * 0.50);
      } else {
        // Spek/SOX: black → navy → blue → cyan → green → yellow → orange → red/white
        if (t < 0.08) {
          const s = t / 0.08;
          r = 0; g = 0; b = Math.round(s * 90);
        } else if (t < 0.22) {
          const s = (t - 0.08) / 0.14;
          r = 0; g = Math.round(s * 20); b = Math.round(90 + s * 165);
        } else if (t < 0.42) {
          const s = (t - 0.22) / 0.20;
          r = 0; g = Math.round(20 + s * 235); b = 255;
        } else if (t < 0.62) {
          const s = (t - 0.42) / 0.20;
          r = 0; g = 255; b = Math.round(255 * (1 - s));
        } else if (t < 0.77) {
          const s = (t - 0.62) / 0.15;
          r = Math.round(s * 255); g = 255; b = 0;
        } else if (t < 0.92) {
          const s = (t - 0.77) / 0.15;
          r = 255; g = Math.round(255 - s * 175); b = 0;
        } else {
          const s = (t - 0.92) / 0.08;
          r = 255; g = Math.round(80 + s * 175); b = Math.round(s * 255);
        }
      }

      lut[i * 3]     = r;
      lut[i * 3 + 1] = g;
      lut[i * 3 + 2] = b;
    }
    return lut;
  }

  let colorLUT: Uint8ClampedArray = buildColorLUT('accent');
  $: colorLUT = buildColorLUT(colorScheme);

  // ---- Bin-to-row LUT (FFT bin → canvas pixel row) -------------------------

  let binLUT: Int32Array | null = null;
  let lutBinCount   = 0;
  let lutSampleRate = 0;
  let lutHeight     = 0;

  function buildBinLUT(binCount: number, sampleRate: number, ph: number): Int32Array {
    const lut     = new Int32Array(ph);
    const nyquist = sampleRate / 2;
    const logMin  = Math.log10(Math.max(1, MIN_FREQ));
    const logMax  = Math.log10(Math.min(MAX_FREQ, nyquist));
    for (let row = 0; row < ph; row++) {
      const t       = row / Math.max(1, ph - 1);
      const logFreq = logMax - t * (logMax - logMin);
      const freq    = Math.pow(10, logFreq);
      const bin     = Math.round((freq / nyquist) * (binCount - 1));
      lut[row]      = Math.max(0, Math.min(binCount - 1, bin));
    }
    return lut;
  }

  // ---- Draw loop ------------------------------------------------------------

  let freqBuf: Uint8Array | null = null;
  let colImgData: ImageData | null = null;

  function draw() {
    if (!canvas) { rafId = requestAnimationFrame(draw); return; }
    const ctx = canvas.getContext('2d', { willReadFrequently: true });
    if (!ctx) { rafId = requestAnimationFrame(draw); return; }

    const dpr = Math.max(1, Math.round(window.devicePixelRatio ?? 1));
    const pw  = width  * dpr;
    const ph  = height * dpr;

    if (canvas.width !== pw || canvas.height !== ph) {
      canvas.width  = pw;
      canvas.height = ph;
      colImgData    = null;
    }

    const analyser = audioEngine.getAnalyser();

    if (!analyser) {
      ctx.fillStyle = '#050508';
      ctx.fillRect(0, 0, pw, ph);
      rafId = requestAnimationFrame(draw);
      return;
    }

    // Seed dB range from analyser once (values are immutable after creation)
    if (!analyserSeeded) {
      minDb = analyser.minDecibels;
      maxDb = analyser.maxDecibels;
      analyserSeeded = true;
    }

    const sampleRate = analyser.context.sampleRate;
    const binCount   = analyser.frequencyBinCount;

    if (binCount !== lutBinCount || sampleRate !== lutSampleRate || ph !== lutHeight) {
      binLUT        = buildBinLUT(binCount, sampleRate, ph);
      lutBinCount   = binCount;
      lutSampleRate = sampleRate;
      lutHeight     = ph;
    }

    if (!freqBuf || freqBuf.length !== binCount) {
      freqBuf = new Uint8Array(binCount);
    }
    analyser.getByteFrequencyData(freqBuf);

    // Shift canvas left by 1 physical pixel (hardware-accelerated via drawImage)
    if (pw > 1) ctx.drawImage(canvas as unknown as CanvasImageSource, -1, 0);

    // Paint rightmost column with current FFT frame
    if (!colImgData) colImgData = ctx.createImageData(1, ph);
    const px = colImgData.data;
    for (let row = 0; row < ph; row++) {
      const amp  = freqBuf[binLUT![row]];
      const ci   = amp * 3;
      const pi   = row * 4;
      px[pi]     = colorLUT[ci];
      px[pi + 1] = colorLUT[ci + 1];
      px[pi + 2] = colorLUT[ci + 2];
      px[pi + 3] = 255;
    }
    ctx.putImageData(colImgData, pw - 1, 0);

    rafId = requestAnimationFrame(draw);
  }

  onMount(() => { rafId = requestAnimationFrame(draw); });
  onDestroy(() => { if (rafId !== null) cancelAnimationFrame(rafId); });
</script>

<figure
  class="spk-fig"
  aria-label="Spek-style scrolling spectrogram — frequency over time"
  style="width:{width}px;height:{height}px;margin:0;"
>
  <canvas bind:this={canvas} style="width:{width}px;height:{height}px;" />

  <!-- kHz frequency axis (left) -->
  <div class="spk-axis spk-axis--left" aria-hidden="true">
    {#each freqLabels as lbl}
      <span class="spk-tick" style="top:{lbl.y}px">{lbl.text}</span>
    {/each}
    <span class="spk-axis-unit" style="top:3px">Hz</span>
  </div>

  <!-- dB level axis (right) -->
  <div class="spk-axis spk-axis--right" aria-hidden="true">
    {#each dbLabels as lbl}
      <span class="spk-tick" style="top:{lbl.y}px">{lbl.text}</span>
    {/each}
    <span class="spk-axis-unit" style="top:3px">dB</span>
  </div>
</figure>

<style>
  .spk-fig {
    display: block;
    position: relative;
    background: #050508;
    border-radius: 4px;
    overflow: hidden;
    line-height: 0;
  }

  .spk-fig canvas {
    display: block;
  }

  /* ---- axis overlay panels ---- */
  .spk-axis {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 26px;
    pointer-events: none;
    /* subtle gradient so labels are readable against bright spectrogram */
  }

  .spk-axis--left {
    left: 0;
    background: linear-gradient(to right, rgba(5,5,8,0.72) 0%, rgba(5,5,8,0) 100%);
    border-right: 1px solid rgba(255,255,255,0.06);
  }

  .spk-axis--right {
    right: 0;
    background: linear-gradient(to left, rgba(5,5,8,0.72) 0%, rgba(5,5,8,0) 100%);
    border-left: 1px solid rgba(255,255,255,0.06);
    text-align: right;
  }

  /* ---- tick labels ---- */
  .spk-tick {
    position: absolute;
    left: 0;
    right: 0;
    display: block;
    transform: translateY(-50%);
    font-size: 8px;
    font-variant-numeric: tabular-nums;
    line-height: 1;
    color: rgba(255, 255, 255, 0.65);
    padding: 0 3px;
    white-space: nowrap;
  }

  .spk-axis--right .spk-tick {
    text-align: right;
  }

  /* axis unit header (Hz / dB) */
  .spk-axis-unit {
    position: absolute;
    left: 0;
    right: 0;
    display: block;
    font-size: 7px;
    line-height: 1;
    color: rgba(255, 255, 255, 0.30);
    padding: 0 3px;
    white-space: nowrap;
  }

  .spk-axis--right .spk-axis-unit {
    text-align: right;
  }
</style>
