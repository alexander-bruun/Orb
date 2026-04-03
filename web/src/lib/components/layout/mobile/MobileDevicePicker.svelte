<script lang="ts">
  import {
    castState,
    castDeviceName,
    sinkIdSupported,
    audioOutputDevices,
    selectedAudioOutputId,
    setAudioOutput,
    refreshAudioOutputDevices,
  } from "$lib/stores/player/casting";
  import {
    activeDevices,
    deviceId,
    exclusiveMode,
  } from "$lib/stores/player/deviceSession";

  export let open = false;
  export let onCastToggle: () => void;
  export let onTransfer: (targetId: string) => void;

  function toggle() {
    open = !open;
    if (open) refreshAudioOutputDevices();
  }
</script>

{#if ($exclusiveMode && $activeDevices.length > 0) || sinkIdSupported || $castState !== "unavailable"}
  <div class="fs-device-wrap">
    <button
      class="fs-extra-btn"
      class:active={open}
      on:click|stopPropagation={toggle}
      aria-label="Switch playback device or audio output"
      title="Switch device / audio output"
    >
      <svg
        width="18"
        height="18"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <rect x="2" y="3" width="20" height="14" rx="2" />
        <path d="M8 21h8" />
        <path d="M12 17v4" />
      </svg>
      <span
        >Devices{#if $activeDevices.length > 1}&nbsp;<span class="queue-count"
            >{$activeDevices.length}</span
          >{/if}</span
      >
    </button>

    {#if open}
      <button
        type="button"
        class="fs-device-overlay"
        tabindex="-1"
        aria-label="Close device picker"
        on:click|stopPropagation={() => (open = false)}
        on:touchstart|stopPropagation={() => {}}
        on:touchmove|stopPropagation={() => {}}
      ></button>

      <div
        class="fs-device-popup"
        role="dialog"
        tabindex="-1"
        on:touchstart|stopPropagation={() => {}}
        on:touchmove|stopPropagation={() => {}}
        on:keydown|stopPropagation
      >
        <!-- Chromecast section -->
        {#if $castState !== "unavailable"}
          <div class="fs-device-header">Cast</div>
          <button
            class="fs-device-item"
            class:is-active={$castState === "connected"}
            on:click={onCastToggle}
            disabled={$castState === "connecting"}
          >
            <div class="fs-device-left">
              <span
                class="fs-device-dot"
                class:fs-device-dot--active={$castState === "connected"}
              ></span>
              <div class="fs-device-info">
                <span class="fs-device-name">
                  {$castState === "connected"
                    ? $castDeviceName
                    : "Chromecast / Cast device"}
                </span>
                <span class="fs-device-track">
                  {#if $castState === "connecting"}Connecting…
                  {:else if $castState === "connected"}Casting now — tap to stop
                  {:else}Tap to cast to a nearby device{/if}
                </span>
              </div>
            </div>
            {#if $castState !== "connected"}
              <span class="fs-transfer-hint">Cast</span>
            {:else}
              <span class="fs-transfer-hint" style="color:var(--error,#e55)"
                >Stop</span
              >
            {/if}
          </button>
        {/if}

        <!-- Audio output section -->
        {#if sinkIdSupported && $audioOutputDevices.length > 0}
          <div
            class="fs-device-header"
            style="margin-top:{$castState !== 'unavailable' ? '8px' : '0'}"
          >
            Audio output
          </div>
          {#each $audioOutputDevices as out (out.deviceId)}
            <button
              class="fs-device-item"
              class:is-active={$selectedAudioOutputId === out.deviceId}
              on:click={() => {
                setAudioOutput(out.deviceId);
                open = false;
              }}
            >
              <div class="fs-device-left">
                <span
                  class="fs-device-dot"
                  class:fs-device-dot--active={$selectedAudioOutputId ===
                    out.deviceId}
                ></span>
                <div class="fs-device-info">
                  <span class="fs-device-name">{out.label}</span>
                  <span class="fs-device-track"
                    >{out.deviceId === "default"
                      ? "System default"
                      : "Audio output"}</span
                  >
                </div>
              </div>
              {#if $selectedAudioOutputId !== out.deviceId}
                <span class="fs-transfer-hint">Select</span>
              {/if}
            </button>
          {/each}
        {/if}

        <!-- Browser / app sessions (only in exclusive mode) -->
        {#if $exclusiveMode && $activeDevices.length > 0}
          <div
            class="fs-device-header"
            style="margin-top:{$castState !== 'unavailable' ||
            (sinkIdSupported && $audioOutputDevices.length > 0)
              ? '8px'
              : '0'}"
          >
            Sessions
          </div>
          {#each $activeDevices as device (device.id)}
            <button
              class="fs-device-item"
              class:is-active={device.is_active}
              class:is-this={device.id === deviceId}
              on:click={() => onTransfer(device.id)}
            >
              <div class="fs-device-left">
                <span
                  class="fs-device-dot"
                  class:fs-device-dot--active={device.is_active}
                ></span>
                <div class="fs-device-info">
                  <span class="fs-device-name">
                    {device.name}
                    {#if device.id === deviceId}<span class="fs-this-badge"
                        >this device</span
                      >{/if}
                  </span>
                  <span class="fs-device-track"
                    >{device.state.track_title || "Idle"}</span
                  >
                </div>
              </div>
              {#if device.id !== deviceId}
                <span class="fs-transfer-hint">Transfer</span>
              {:else if !device.is_active}
                <span class="fs-transfer-hint">Play here</span>
              {/if}
            </button>
          {/each}
        {/if}
      </div>
    {/if}
  </div>
{/if}

<style>
  .fs-extra-btn {
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.45);
    cursor: pointer;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    padding: 8px;
    border-radius: 8px;
    position: relative;
    -webkit-tap-highlight-color: transparent;
    transition: color 0.15s;
  }
  .fs-extra-btn.active {
    color: var(--accent);
  }
  .queue-count {
    font-weight: 700;
  }

  .fs-device-wrap {
    position: relative;
  }
  .fs-device-overlay {
    position: fixed;
    inset: 0;
    z-index: 10;
    border: none;
    padding: 0;
    margin: 0;
    background: transparent;
    cursor: default;
    outline: none;
  }
  .fs-device-popup {
    position: absolute;
    bottom: calc(100% + 8px);
    left: 50%;
    transform: translateX(-50%);
    width: 260px;
    background: var(--bg-elevated, #1e1e1e);
    border: 1px solid rgba(255, 255, 255, 0.12);
    border-radius: 12px;
    overflow: hidden;
    z-index: 11;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.55);
  }
  .fs-device-header {
    padding: 10px 14px 6px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: rgba(255, 255, 255, 0.4);
  }
  .fs-device-item {
    width: 100%;
    background: none;
    border: none;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 10px 14px;
    cursor: pointer;
    color: rgba(255, 255, 255, 0.85);
    font-size: 0.875rem;
    text-align: left;
    transition: background 0.1s;
    -webkit-tap-highlight-color: transparent;
  }
  .fs-device-item:active,
  .fs-device-item:hover {
    background: rgba(255, 255, 255, 0.07);
  }
  .fs-device-item.is-active {
    color: #fff;
  }
  .fs-device-left {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }
  .fs-device-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
    background: rgba(255, 255, 255, 0.25);
  }
  .fs-device-dot--active {
    background: var(--accent, #1db954);
    box-shadow: 0 0 6px var(--accent, #1db954);
  }
  .fs-device-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .fs-device-name {
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .fs-this-badge {
    font-size: 10px;
    font-weight: 400;
    color: rgba(255, 255, 255, 0.4);
  }
  .fs-device-track {
    font-size: 0.75rem;
    color: rgba(255, 255, 255, 0.4);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .fs-transfer-hint {
    font-size: 11px;
    color: var(--accent, #1db954);
    flex-shrink: 0;
    font-weight: 500;
  }
</style>
