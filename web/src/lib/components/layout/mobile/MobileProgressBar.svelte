<script lang="ts">
  import {
    positionMs,
    durationMs,
    formattedPosition,
    formattedDuration,
    bufferedPct,
    seek,
  } from '$lib/stores/player';
  import { waveformEnabled } from '$lib/stores/settings/theme';
  import TrackWaveform from '$lib/components/ui/TrackWaveform.svelte';
  import { waveformFailed } from '$lib/stores/player/waveformPeaks';

  $: progress = $durationMs > 0 ? ($positionMs / $durationMs) * 100 : 0;

  let seekDragValue: number | null = null;
  let waveformWidth = 0;

  function onSeekInput(e: Event) {
    seekDragValue = parseFloat((e.target as HTMLInputElement).value);
  }

  function onSeek(e: Event) {
    const pct = parseFloat((e.target as HTMLInputElement).value);
    seekDragValue = null;
    seek(($durationMs / 1000) * (pct / 100));
  }
</script>

<div class="fs-seek">
  {#if $waveformEnabled && !$waveformFailed}
    
    <div
      class="waveform-wrap"
      role="presentation"
      bind:clientWidth={waveformWidth}
      on:touchstart|stopPropagation={() => {}}
      on:touchmove|stopPropagation={() => {}}
    >
      {#if waveformWidth > 0}
        <TrackWaveform width={waveformWidth} height={48} />
      {/if}
    </div>
  {:else}
    <div class="seek-bar-wrap">
      <div class="seek-track">
        <div class="seek-buffered" style="width: {$bufferedPct}%"></div>
        <div class="seek-fill" style="width: {seekDragValue !== null ? seekDragValue : progress}%"></div>
      </div>
      <input
        type="range"
        min="0"
        max="100"
        step="0.1"
        value={seekDragValue !== null ? seekDragValue : progress}
        on:input={onSeekInput}
        on:change={onSeek}
        on:touchstart|stopPropagation={() => {}}
        on:touchmove|stopPropagation={() => {}}
        class="seek-input"
        aria-label="Seek"
      />
    </div>
  {/if}
  <div class="seek-times">
    <span>{$formattedPosition}</span>
    <span>{$formattedDuration}</span>
  </div>
</div>

<style>
  @media (max-width: 640px) {
    .fs-seek {
      flex-shrink: 0;
      padding: 4px 0 12px;
    }

    .waveform-wrap {
      width: 100%;
      margin-bottom: 8px;
    }

    .seek-bar-wrap {
      position: relative;
      height: 4px;
      display: flex;
      align-items: center;
      margin-bottom: 8px;
    }

    .seek-track {
      position: absolute;
      left: 0; right: 0;
      height: 4px;
      background: rgba(255, 255, 255, 0.2);
      border-radius: 2px;
      overflow: hidden;
    }

    .seek-buffered {
      position: absolute;
      height: 100%;
      background: rgba(255, 255, 255, 0.3);
      pointer-events: none;
    }

    .seek-fill {
      position: absolute;
      height: 100%;
      background: #fff;
      pointer-events: none;
    }

    .seek-input {
      position: absolute;
      left: -8px; right: -8px;
      width: calc(100% + 16px);
      height: 28px;
      margin: 0;
      cursor: pointer;
      -webkit-appearance: none;
      appearance: none;
      background: transparent;
      touch-action: none;
    }

    .seek-input::-webkit-slider-runnable-track {
      background: transparent;
      height: 4px;
    }

    .seek-input::-moz-range-track {
      background: transparent;
      height: 4px;
      border: none;
    }

    .seek-input::-webkit-slider-thumb {
      -webkit-appearance: none;
      width: 16px;
      height: 16px;
      border-radius: 50%;
      background: #fff;
      margin-top: -6px;
      cursor: pointer;
    }

    .seek-input::-moz-range-thumb {
      width: 16px;
      height: 16px;
      border-radius: 50%;
      background: #fff;
      border: none;
      cursor: pointer;
    }

    .seek-times {
      display: flex;
      justify-content: space-between;
      font-size: 0.72rem;
      color: rgba(255, 255, 255, 0.55);
      font-variant-numeric: tabular-nums;
    }
  }
</style>
