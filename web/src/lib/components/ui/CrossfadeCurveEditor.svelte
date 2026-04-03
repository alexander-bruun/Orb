<script lang="ts">
  import { createEventDispatcher } from "svelte";

  const clamp01 = (v: number) => Math.min(1, Math.max(0, v));

  export let title = "";
  export let subtitle = "";
  export let values: number[] = [0, 0.25, 0.6, 0.85, 1];
  export let lockStart: number | null = null;
  export let lockEnd: number | null = null;

  const dispatch = createEventDispatcher<{ change: { values: number[] } }>();

  const viewSize = 100;
  const pad = 8;
  const graphSize = viewSize - pad * 2;
  const gridFractions = [0, 0.25, 0.5, 0.75, 1];

  let svgEl: SVGSVGElement | null = null;
  let activeIndex: number | null = null;

  function normalise(vals: number[]): number[] {
    const next = vals.length >= 2 ? vals.map((v) => clamp01(Number(v) || 0)) : [0, 1];
    if (lockStart !== null) next[0] = clamp01(lockStart);
    if (lockEnd !== null) next[next.length - 1] = clamp01(lockEnd);
    return next;
  }

  $: curve = normalise(values);

  function xFor(index: number): number {
    if (curve.length <= 1) return pad;
    return pad + (index / (curve.length - 1)) * graphSize;
  }

  function yFor(gain: number): number {
    return pad + (1 - clamp01(gain)) * graphSize;
  }

  function isLocked(index: number): boolean {
    if (index === 0 && lockStart !== null) return true;
    if (index === curve.length - 1 && lockEnd !== null) return true;
    return false;
  }

  function updateIndex(index: number, gain: number): void {
    const next = [...curve];
    next[index] = Math.round(clamp01(gain) * 100) / 100;
    if (lockStart !== null) next[0] = clamp01(lockStart);
    if (lockEnd !== null) next[next.length - 1] = clamp01(lockEnd);
    values = next;
    dispatch("change", { values: next });
  }

  function gainFromClientY(clientY: number): number {
    if (!svgEl) return 0;
    const rect = svgEl.getBoundingClientRect();
    if (rect.height <= 0) return 0;
    const ratio = (clientY - rect.top) / rect.height;
    return clamp01(1 - ratio);
  }

  function onPointerDown(index: number, e: PointerEvent): void {
    if (isLocked(index)) return;
    activeIndex = index;
    const target = e.currentTarget as SVGCircleElement;
    target.setPointerCapture(e.pointerId);
    updateIndex(index, gainFromClientY(e.clientY));
  }

  function onPointerMove(e: PointerEvent): void {
    if (activeIndex === null) return;
    updateIndex(activeIndex, gainFromClientY(e.clientY));
  }

  function onPointerUp(): void {
    activeIndex = null;
  }

  function onHandleKeydown(index: number, e: KeyboardEvent): void {
    if (isLocked(index)) return;
    if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
    e.preventDefault();
    const delta = e.key === "ArrowUp" ? 0.02 : -0.02;
    updateIndex(index, curve[index] + delta);
  }

  $: pathData = curve
    .map((gain, idx) => `${idx === 0 ? "M" : "L"} ${xFor(idx)} ${yFor(gain)}`)
    .join(" ");
</script>

<div class="curve-editor">
  <div class="curve-meta">
    <span class="curve-title">{title}</span>
    <span class="curve-subtitle">{subtitle}</span>
  </div>

  <svg
    bind:this={svgEl}
    class="curve-graph"
    viewBox="0 0 100 100"
    role="application"
    aria-label={`${title} crossfade curve editor`}
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

    <path d={pathData} class="curve-line" />

    {#each curve as gain, idx}
      <circle
        cx={xFor(idx)}
        cy={yFor(gain)}
        r="2.2"
        class="curve-handle"
        class:curve-handle--locked={isLocked(idx)}
        tabindex={isLocked(idx) ? -1 : 0}
        role="slider"
        aria-label={`${title} point ${idx + 1}`}
        aria-valuemin="0"
        aria-valuemax="1"
        aria-valuenow={gain}
        on:pointerdown={(e) => onPointerDown(idx, e)}
        on:keydown={(e) => onHandleKeydown(idx, e)}
      />
    {/each}
  </svg>

  <div class="curve-scale">
    <span>0%</span>
    <span>Gain</span>
    <span>100%</span>
  </div>
</div>

<style>
  .curve-editor {
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: 100%;
    min-width: 0;
  }

  .curve-meta {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .curve-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--text);
  }

  .curve-subtitle {
    font-size: 11px;
    color: var(--text-2);
  }

  .curve-graph {
    width: 100%;
    height: auto;
    aspect-ratio: 1.9;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: color-mix(in srgb, var(--surface) 85%, transparent);
    touch-action: none;
  }

  .axis {
    stroke: var(--text-2);
    stroke-opacity: 0.65;
    stroke-width: 1;
  }

  .grid-bg {
    fill: color-mix(in srgb, var(--surface) 78%, transparent);
  }

  .grid-line {
    stroke: var(--border);
    stroke-width: 0.8;
  }

  .curve-line {
    fill: none;
    stroke: var(--accent);
    stroke-width: 2;
  }

  .curve-handle {
    fill: var(--accent);
    stroke: #fff;
    stroke-width: 0.7;
    cursor: ns-resize;
  }

  .curve-handle:focus-visible {
    outline: none;
    stroke: var(--text);
    stroke-width: 1.2;
  }

  .curve-handle--locked {
    opacity: 0.35;
    cursor: default;
  }

  .curve-scale {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 10px;
    color: var(--text-2);
    letter-spacing: 0.02em;
  }
</style>
