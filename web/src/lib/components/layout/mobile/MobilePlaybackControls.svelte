<script lang="ts">
  import {
    playbackState,
    repeatMode,
    shuffle,
    smartShuffleEnabled,
    togglePlayPause,
    next,
    previous,
    toggleRepeat,
    toggleShuffle,
  } from "$lib/stores/player";

  /** Whether this is a fullscreen (fs) or mini layout */
  export let variant: "fullscreen" | "mini" = "fullscreen";
</script>

{#if variant === "fullscreen"}
  <div class="fs-controls">
    <button
      class="fs-btn fs-btn--icon"
      class:active={$shuffle}
      on:click={toggleShuffle}
      aria-label="Shuffle"
      aria-pressed={$shuffle}
      title={$shuffle && $smartShuffleEnabled ? "Smart Shuffle on" : "Shuffle"}
      style="position:relative"
    >
      <svg
        width="20"
        height="20"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <polyline points="16 3 21 3 21 8" /><line
          x1="4"
          y1="20"
          x2="21"
          y2="3"
        />
        <polyline points="21 16 21 21 16 21" /><line
          x1="15"
          y1="15"
          x2="21"
          y2="21"
        />
        <line x1="4" y1="4" x2="9" y2="9" />
      </svg>
      {#if $shuffle && $smartShuffleEnabled}
        <span class="smart-dot" aria-hidden="true"></span>
      {/if}
    </button>

    <button
      class="fs-btn fs-btn--prev"
      on:click={previous}
      aria-label="Previous"
    >
      <svg
        width="28"
        height="28"
        viewBox="0 0 24 24"
        fill="currentColor"
        aria-hidden="true"
      >
        <polygon points="19,4 9,12 19,20" />
        <rect x="5" y="4" width="2.5" height="16" rx="1" />
      </svg>
    </button>

    <button
      class="fs-btn fs-btn--play"
      on:click={togglePlayPause}
      aria-label={$playbackState === "playing" ? "Pause" : "Play"}
    >
      {#if $playbackState === "playing"}
        <svg
          width="32"
          height="32"
          viewBox="0 0 24 24"
          fill="currentColor"
          aria-hidden="true"
        >
          <rect x="6" y="4" width="4" height="16" rx="1.5" />
          <rect x="14" y="4" width="4" height="16" rx="1.5" />
        </svg>
      {:else}
        <svg
          width="32"
          height="32"
          viewBox="0 0 24 24"
          fill="currentColor"
          aria-hidden="true"
        >
          <polygon points="5,3 19,12 5,21" />
        </svg>
      {/if}
    </button>

    <button class="fs-btn fs-btn--next" on:click={next} aria-label="Next">
      <svg
        width="28"
        height="28"
        viewBox="0 0 24 24"
        fill="currentColor"
        aria-hidden="true"
      >
        <polygon points="5,4 15,12 5,20" />
        <rect x="16" y="4" width="2.5" height="16" rx="1" />
      </svg>
    </button>

    <button
      class="fs-btn fs-btn--icon"
      class:active={$repeatMode !== "off"}
      on:click={toggleRepeat}
      aria-label="Repeat"
      aria-pressed={$repeatMode !== "off"}
      style="position: relative;"
    >
      <svg
        width="20"
        height="20"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <polyline points="17 1 21 5 17 9" /><path
          d="M3 11V9a4 4 0 0 1 4-4h14"
        />
        <polyline points="7 23 3 19 7 15" /><path
          d="M21 13v2a4 4 0 0 1-4 4H3"
        />
      </svg>
      {#if $repeatMode === "one"}
        <span class="one-badge">1</span>
      {/if}
    </button>
  </div>
{/if}

<style>
  @media (max-width: 640px) {
    .fs-controls {
      flex-shrink: 0;
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 8px 0 16px;
    }

    .fs-btn {
      background: none;
      border: none;
      color: rgba(255, 255, 255, 0.8);
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 8px;
      border-radius: 50%;
      transition:
        color 0.15s,
        background 0.1s;
      -webkit-tap-highlight-color: transparent;
      position: relative;
    }

    .fs-btn:active {
      background: rgba(255, 255, 255, 0.1);
    }

    .fs-btn--icon {
      color: rgba(255, 255, 255, 0.5);
    }

    .smart-dot {
      position: absolute;
      top: 2px;
      right: 2px;
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: var(--accent);
      opacity: 0.9;
    }

    .fs-btn--icon.active {
      color: var(--accent);
    }

    .fs-btn--icon.active::after {
      content: "";
      position: absolute;
      bottom: 3px;
      left: 50%;
      transform: translateX(-50%);
      width: 4px;
      height: 4px;
      border-radius: 50%;
      background: var(--accent);
    }

    .fs-btn--prev,
    .fs-btn--next {
      color: #fff;
    }

    .fs-btn--play {
      width: 68px;
      height: 68px;
      background: #fff;
      color: #000;
      border-radius: 50%;
      box-shadow: 0 4px 20px rgba(0, 0, 0, 0.4);
    }

    .fs-btn--play:active {
      background: rgba(255, 255, 255, 0.85);
    }

    .one-badge {
      position: absolute;
      bottom: 3px;
      right: 2px;
      font-size: 9px;
      font-weight: 700;
      line-height: 1;
      color: var(--accent);
      pointer-events: none;
    }
  }
</style>
