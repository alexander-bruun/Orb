<script lang="ts">
  import { createEventDispatcher } from "svelte";

  const clamp01 = (v: number) => Math.min(1, Math.max(0, v));

  export let outValues: number[] = [1, 0.82, 0.58, 0.26, 0];
  export let inValues: number[] = [0, 0.26, 0.58, 0.82, 1];

  const dispatch = createEventDispatcher<{
    change: { outValues: number[]; inValues: number[] };
  }>();

  const viewSize = 100;
  const pad = 8;
  const graphSize = viewSize - pad * 2;
  const gridFractions = [0, 0.25, 0.5, 0.75, 1];

  let svgEl: SVGSVGElement | null = null;
  let activeHandle: { curve: "out" | "in"; index: number } | null = null;

  function normaliseCurve(vals: number[], lockStart: number, lockEnd: number): number[] {
    const next = vals.length >= 2 ? vals.map((v) => clamp01(Number(v) || 0)) : [lockStart, lockEnd];
    next[0] = clamp01(lockStart);
    next[next.length - 1] = clamp01(lockEnd);
    return next;
  }

  $: outCurve = normaliseCurve(outValues, 1, 0);
  $: inCurve = normaliseCurve(inValues, 0, 1);
  $: pointCount = Math.min(outCurve.length, inCurve.length);

  function xFor(index: number): number {
    if (pointCount <= 1) return pad;
    return pad + (index / (pointCount - 1)) * graphSize;
  }

  function yFor(gain: number): number {
    return pad + (1 - clamp01(gain)) * graphSize;
  }

  function isLocked(index: number): boolean {
    return index === 0 || index === pointCount - 1;
  }

  function emit(nextOut: number[], nextIn: number[]): void {
    outValues = nextOut;
    inValues = nextIn;
    dispatch("change", { outValues: nextOut, inValues: nextIn });
  }

  function updatePoint(curve: "out" | "in", index: number, gain: number): void {
    const nextOut = [...outCurve];
    const nextIn = [...inCurve];

    if (curve === "out") nextOut[index] = Math.round(clamp01(gain) * 100) / 100;
    else nextIn[index] = Math.round(clamp01(gain) * 100) / 100;

    nextOut[0] = 1;
    nextOut[nextOut.length - 1] = 0;
    nextIn[0] = 0;
    nextIn[nextIn.length - 1] = 1;

    emit(nextOut, nextIn);
  }

  function gainFromClientY(clientY: number): number {
    if (!svgEl) return 0;
    const rect = svgEl.getBoundingClientRect();
    if (rect.height <= 0) return 0;
    const ratio = (clientY - rect.top) / rect.height;
    return clamp01(1 - ratio);
  }

  function onPointerDown(curve: "out" | "in", index: number, e: PointerEvent): void {
    if (isLocked(index)) return;
    activeHandle = { curve, index };
    const target = e.currentTarget as SVGCircleElement;
    target.setPointerCapture(e.pointerId);
    updatePoint(curve, index, gainFromClientY(e.clientY));
  }

  function onPointerMove(e: PointerEvent): void {
    if (!activeHandle) return;
    updatePoint(activeHandle.curve, activeHandle.index, gainFromClientY(e.clientY));
  }

  function onPointerUp(): void {
    activeHandle = null;
  }

  function onHandleKeydown(curve: "out" | "in", index: number, e: KeyboardEvent): void {
    if (isLocked(index)) return;
    if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
    e.preventDefault();
    const base = curve === "out" ? outCurve[index] : inCurve[index];
    const delta = e.key === "ArrowUp" ? 0.02 : -0.02;
    updatePoint(curve, index, base + delta);
  }

  $: outPath = outCurve
    .slice(0, pointCount)
    .map((gain, idx) => `${idx === 0 ? "M" : "L"} ${xFor(idx)} ${yFor(gain)}`)
    .join(" ");

  $: inPath = inCurve
    .slice(0, pointCount)
    .map((gain, idx) => `${idx === 0 ? "M" : "L"} ${xFor(idx)} ${yFor(gain)}`)
    .join(" ");
</script>

<div class="overlap-editor">
  <div class="overlap-meta">
    <span class="overlap-title">Crossfade Shape</span>
    <span class="overlap-subtitle">
      Drag points on either line to shape how tracks overlap through the crossfade window.
    </span>
  </div>

  <svg
    bind:this={svgEl}
    class="overlap-graph"
    viewBox="0 0 100 100"
    role="application"
    aria-label="Crossfade overlap graph editor"
    on:pointermove={onPointerMove}
    on:pointerup={onPointerUp}
    on:pointerleave={onPointerUp}
  >
    <rect x={pad} y={pad} width={graphSize} height={graphSize} class="grid-bg" rx="3" />

    {#each gridFractions as t}
      <line
        x1={pad}
        y1={pad + t * graphSize}
        x2={pad + graphSize}
        y2={pad + t * graphSize}
        class="grid-line"
      />
      <line
        x1={pad + t * graphSize}
        y1={pad}
        x2={pad + t * graphSize}
        y2={pad + graphSize}
        class="grid-line"
      />
    {/each}

    <line x1={pad} y1={pad} x2={pad} y2={pad + graphSize} class="axis" />
    <line x1={pad} y1={pad + graphSize} x2={pad + graphSize} y2={pad + graphSize} class="axis" />

    <path d={outPath} class="curve-line curve-line--out" />
    <path d={inPath} class="curve-line curve-line--in" />

    {#each outCurve.slice(0, pointCount) as gain, idx}
      <circle
        cx={xFor(idx)}
        cy={yFor(gain)}
        r="2.2"
        class="curve-handle curve-handle--out"
        class:curve-handle--locked={isLocked(idx)}
        tabindex={isLocked(idx) ? -1 : 0}
        role="slider"
        aria-label={`Current track fade-out point ${idx + 1}`}
        aria-valuemin="0"
        aria-valuemax="1"
        aria-valuenow={gain}
        on:pointerdown={(e) => onPointerDown("out", idx, e)}
        on:keydown={(e) => onHandleKeydown("out", idx, e)}
      />
    {/each}

    {#each inCurve.slice(0, pointCount) as gain, idx}
      <circle
        cx={xFor(idx)}
        cy={yFor(gain)}
        r="2.2"
        class="curve-handle curve-handle--in"
        class:curve-handle--locked={isLocked(idx)}
        tabindex={isLocked(idx) ? -1 : 0}
        role="slider"
        aria-label={`Next track fade-in point ${idx + 1}`}
        aria-valuemin="0"
        aria-valuemax="1"
        aria-valuenow={gain}
        on:pointerdown={(e) => onPointerDown("in", idx, e)}
        on:keydown={(e) => onHandleKeydown("in", idx, e)}
      />
    {/each}
  </svg>

  <div class="overlap-legend" aria-label="Curve legend">
    <span class="legend-item"><i class="legend-dot legend-dot--out"></i>Current track fade-out</span>
    <span class="legend-item"><i class="legend-dot legend-dot--in"></i>Next track fade-in</span>
  </div>
</div>

<style>
  .overlap-editor {
    display: flex;
    flex-direction: column;
    gap: 10px;
    width: 100%;
    min-width: 0;
  }

  .overlap-meta {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .overlap-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--text);
  }

  .overlap-subtitle {
    font-size: 11px;
    color: var(--text-2);
  }

  .overlap-graph {
    width: 100%;
    height: auto;
    aspect-ratio: 2.2;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: color-mix(in srgb, var(--surface) 85%, transparent);
    touch-action: none;
  }

  .grid-bg {
    fill: color-mix(in srgb, var(--surface) 78%, transparent);
  }

  .grid-line {
    stroke: var(--border);
    stroke-width: 0.8;
  }

  .axis {
    stroke: var(--text-2);
    stroke-opacity: 0.65;
    stroke-width: 1;
  }

  .curve-line {
    fill: none;
    stroke-width: 2;
  }

  .curve-line--out {
    stroke: #ef4444;
  }

  .curve-line--in {
    stroke: #22c55e;
  }

  .curve-handle {
    stroke-width: 0.7;
    cursor: ns-resize;
  }

  .curve-handle--out {
    fill: #ef4444;
    stroke: #fff;
  }

  .curve-handle--in {
    fill: #22c55e;
    stroke: #fff;
  }

  .curve-handle:focus-visible {
    outline: none;
    stroke: var(--text);
    stroke-width: 1.2;
  }

  .curve-handle--locked {
    opacity: 0.4;
    cursor: default;
  }

  .overlap-legend {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    font-size: 10.5px;
    color: var(--text-2);
  }

  .legend-item {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .legend-dot {
    width: 8px;
    height: 8px;
    border-radius: 999px;
    display: inline-block;
  }

  .legend-dot--out {
    background: #ef4444;
  }

  .legend-dot--in {
    background: #22c55e;
  }
</style>
