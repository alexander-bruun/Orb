<script lang="ts">
  import {
    transferPlayback,
  } from '$lib/stores/player';
  import { activeDevices, deviceId, exclusiveMode } from '$lib/stores/player/deviceSession';
  import {
    audioOutputDevices,
    selectedAudioOutputId,
    sinkIdSupported,
    refreshAudioOutputDevices,
    setAudioOutput,
    castState,
    castDeviceName,
    initCastSdk,
    startCast,
    stopCast,
  } from '$lib/stores/player/casting';

  let castPickerOpen = false;
  let devicePickerOpen = false;

  async function handleCastToggle() {
    if ($castState === 'connected') {
      stopCast();
    } else if ($castState === 'idle') {
      try { await startCast(); } catch { /* user cancelled */ }
    }
  }

  async function transferToDevice(targetId: string) {
    devicePickerOpen = false;
    await transferPlayback(targetId);
  }
</script>

<!-- Cast / audio output — always visible, separate from session management -->
<div class="device-picker-wrap">
  <button
    class="ctrl-btn icon-btn cast-btn"
    class:active={castPickerOpen || $castState === 'connected'}
    on:click={() => { castPickerOpen = !castPickerOpen; devicePickerOpen = false; if (castPickerOpen) { initCastSdk(); refreshAudioOutputDevices(); } }}
    title="Cast / audio output"
    aria-label="Cast to a device or change audio output"
  >
    <!-- Cast / screen-share icon -->
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <path d="M2 8.5V6a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-6"/>
      <path d="M2 15a7 7 0 0 1 7 7"/>
      <path d="M2 15a3 3 0 0 1 3 3"/>
      <line x1="2" y1="22" x2="2.01" y2="22"/>
    </svg>
    {#if $castState === 'connected'}
      <span class="device-count cast-dot"></span>
    {/if}
  </button>

  {#if castPickerOpen}
    <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
    <div class="device-picker-overlay" on:click={() => castPickerOpen = false}></div>
    <div class="device-picker-popup">

      <!-- ── Chromecast section — always shown ─────────────── -->
      <div class="device-picker-header">Chromecast</div>
      <button
        class="device-item"
        class:is-active={$castState === 'connected'}
        on:click={handleCastToggle}
        disabled={$castState === 'connecting' || $castState === 'unavailable'}
      >
        <div class="device-item-left">
          {#if $castState === 'connected'}
            <span class="device-active-dot"></span>
          {:else}
            <span class="device-idle-dot" class:dim={$castState === 'unavailable'}></span>
          {/if}
          <div class="device-item-info">
            <span class="device-item-name">
              {$castState === 'connected' ? $castDeviceName : 'Chromecast / Google TV'}
            </span>
            <span class="device-item-track">
              {#if $castState === 'unavailable'}Requires Chrome — no Cast devices found
              {:else if $castState === 'connecting'}Connecting…
              {:else if $castState === 'connected'}Casting now — click to stop
              {:else}Click to cast to a nearby device{/if}
            </span>
          </div>
        </div>
        {#if $castState === 'connected'}
          <span class="device-transfer-hint" style="color:var(--error,#e55)">Stop</span>
        {:else if $castState !== 'unavailable'}
          <span class="device-transfer-hint">Cast</span>
        {/if}
      </button>

      <!-- ── Audio output section ──────────────────────────── -->
      {#if sinkIdSupported}
        <div class="device-picker-header" style="margin-top:8px">Audio output</div>
        {#if $audioOutputDevices.length === 0}
          <p class="device-picker-empty">No additional outputs found</p>
        {:else}
          {#each $audioOutputDevices as out (out.deviceId)}
            <button
              class="device-item"
              class:is-active={$selectedAudioOutputId === out.deviceId}
              on:click={() => { setAudioOutput(out.deviceId); castPickerOpen = false; }}
            >
              <div class="device-item-left">
                {#if $selectedAudioOutputId === out.deviceId}
                  <span class="device-active-dot"></span>
                {:else}
                  <span class="device-idle-dot"></span>
                {/if}
                <div class="device-item-info">
                  <span class="device-item-name">{out.label}</span>
                  <span class="device-item-track">{out.deviceId === 'default' ? 'System default' : 'Audio output'}</span>
                </div>
              </div>
              {#if $selectedAudioOutputId !== out.deviceId}
                <span class="device-transfer-hint">Select</span>
              {/if}
            </button>
          {/each}
        {/if}
      {/if}

    </div>
  {/if}
</div>

<!-- Sessions button — only when exclusive mode is on and multiple sessions exist -->
{#if $exclusiveMode && $activeDevices.length > 1}
  <div class="device-picker-wrap">
    <button
      class="ctrl-btn icon-btn device-btn"
      class:active={devicePickerOpen}
      on:click={() => { devicePickerOpen = !devicePickerOpen; castPickerOpen = false; }}
      title="Orb sessions"
      aria-label="Switch to another active session"
    >
      <!-- Devices icon -->
      <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <rect x="2" y="3" width="20" height="14" rx="2"/>
        <path d="M8 21h8"/>
        <path d="M12 17v4"/>
      </svg>
      <span class="device-count">{$activeDevices.length}</span>
    </button>

    {#if devicePickerOpen}
      <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
      <div class="device-picker-overlay" on:click={() => devicePickerOpen = false}></div>
      <div class="device-picker-popup">
        <div class="device-picker-header">Sessions</div>
        {#each $activeDevices as device (device.id)}
          <button
            class="device-item"
            class:is-active={device.is_active}
            class:is-this={device.id === deviceId}
            on:click={() => transferToDevice(device.id)}
          >
            <div class="device-item-left">
              {#if device.is_active}
                <span class="device-active-dot"></span>
              {:else}
                <span class="device-idle-dot"></span>
              {/if}
              <div class="device-item-info">
                <span class="device-item-name">
                  {device.name}
                  {#if device.id === deviceId}<span class="this-badge">this device</span>{/if}
                </span>
                <span class="device-item-track">
                  {device.state.track_title || 'Idle'}
                </span>
              </div>
            </div>
            {#if device.id !== deviceId}
              <span class="device-transfer-hint">Transfer</span>
            {:else if !device.is_active}
              <span class="device-transfer-hint">Play here</span>
            {/if}
          </button>
        {/each}
      </div>
    {/if}
  </div>
{/if}

<style>
  /* ── Device picker ── */
  .device-picker-wrap {
    position: relative;
  }
  .ctrl-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 1.2rem;
    line-height: 1;
    padding: 0 6px;
    height: 28px;
    transition: color 0.15s;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .ctrl-btn:hover { color: var(--text); }
  .icon-btn {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 0;
    padding: 6px;
  }
  .icon-btn.active { color: var(--accent); }
  .icon-btn.active::after {
    content: '';
    position: absolute;
    bottom: 3px;
    left: 50%;
    transform: translateX(-50%);
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: var(--accent);
  }
  .device-btn {
    position: relative;
  }
  .device-count {
    position: absolute;
    top: 1px;
    right: 0px;
    font-size: 9px;
    font-weight: 700;
    line-height: 1;
    color: var(--accent);
    pointer-events: none;
  }
  .cast-dot {
    position: absolute;
    top: 3px;
    right: 2px;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
    pointer-events: none;
  }
  .device-picker-overlay {
    position: fixed;
    inset: 0;
    z-index: 999;
  }
  .device-picker-popup {
    position: absolute;
    bottom: calc(100% + 10px);
    right: 0;
    width: 260px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 8px 32px rgba(0,0,0,0.35);
    z-index: 1000;
    overflow: hidden;
  }
  .device-picker-header {
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-muted);
    padding: 10px 14px 6px;
  }
  .device-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    padding: 8px 14px;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    gap: 8px;
    transition: background 0.12s;
  }
  .device-item:hover {
    background: var(--bg-hover);
  }
  .device-item.is-active .device-item-name {
    color: var(--accent);
  }
  .device-item-left {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }
  .device-active-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--accent);
    flex-shrink: 0;
    box-shadow: 0 0 6px var(--accent);
  }
  .device-idle-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-muted);
    opacity: 0.35;
    flex-shrink: 0;
  }
  .device-idle-dot.dim {
    opacity: 0.15;
  }
  .device-picker-empty {
    font-size: 0.8rem;
    color: var(--text-muted);
    padding: 6px 10px;
    margin: 0;
  }
  .device-item-info {
    display: flex;
    flex-direction: column;
    min-width: 0;
    gap: 2px;
  }
  .device-item-name {
    font-size: 0.85rem;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .this-badge {
    font-size: 0.65rem;
    color: var(--text-muted);
    font-weight: 400;
  }
  .device-item-track {
    font-size: 0.75rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .device-transfer-hint {
    font-size: 0.7rem;
    color: var(--text-muted);
    flex-shrink: 0;
    opacity: 0;
    transition: opacity 0.12s;
  }
  .device-item:hover .device-transfer-hint {
    opacity: 1;
  }
</style>
