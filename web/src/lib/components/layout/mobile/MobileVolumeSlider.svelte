<script lang="ts">
  import { volume, setVolume } from '$lib/stores/player';

  function onVolumeChange(e: Event) {
    setVolume(parseFloat((e.target as HTMLInputElement).value));
  }
</script>

<div class="fs-volume">
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
    <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>
  </svg>
  <input
    type="range"
    min="0"
    max="1"
    step="0.01"
    value={$volume}
    on:input={onVolumeChange}
    on:touchstart|stopPropagation={() => {}}
    on:touchmove|stopPropagation={() => {}}
    class="volume-slider"
    aria-label="Volume"
    style="background: linear-gradient(to right, rgba(255,255,255,0.85) {$volume * 100}%, rgba(255,255,255,0.25) {$volume * 100}%)"
  />
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
    <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>
    <path d="M15.54 8.46a5 5 0 0 1 0 7.07"/>
    <path d="M19.07 4.93a10 10 0 0 1 0 14.14"/>
  </svg>
</div>

<style>
  @media (max-width: 640px) {
    .fs-volume {
      flex-shrink: 0;
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 0 0 16px;
      color: rgba(255, 255, 255, 0.45);
    }

    .volume-slider {
      flex: 1;
      height: 4px;
      accent-color: #fff;
      cursor: pointer;
      -webkit-appearance: none;
      appearance: none;
      background: rgba(255, 255, 255, 0.25);
      border-radius: 2px;
      touch-action: none;
      transition: background 0.15s ease;
    }

    .volume-slider::-webkit-slider-thumb {
      -webkit-appearance: none;
      width: 14px;
      height: 14px;
      border-radius: 50%;
      background: #fff;
      cursor: pointer;
    }

    .volume-slider::-moz-range-thumb {
      width: 14px;
      height: 14px;
      border-radius: 50%;
      background: #fff;
      border: none;
      cursor: pointer;
    }
  }
</style>
