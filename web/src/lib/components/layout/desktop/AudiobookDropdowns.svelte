<script context="module" lang="ts">
  export function formatBmTime(ms: number): string {
    const s = Math.floor(ms / 1000);
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = s % 60;
    if (h > 0)
      return `${h}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
    return `${m}:${String(sec).padStart(2, "0")}`;
  }
</script>

<script lang="ts">
  import {
    currentAudiobook,
    abSpeed,
    abVolume,
    abCurrentChapter,
    abBookmarks,
    sleepTimerMins,
    setABSpeed,
    setABVolume,
    setSleepTimer,
    jumpToChapter,
    createBookmark,
    deleteBookmark,
    seekAudiobook,
    AB_SPEEDS,
    SLEEP_PRESETS,
  } from "$lib/stores/player/audiobookPlayer";
  import {
    lpRole,
    lpPanelOpen,
    lpParticipants,
    createAndConnect,
  } from "$lib/stores/social/listenParty";
  import {
    activeDevices,
    activeDeviceId,
    deviceId,
    exclusiveMode,
  } from "$lib/stores/player/deviceSession";
  import { audiobookSleepTimerEnabled } from "$lib/stores/settings/theme";

  export let speedOpen = false;
  export let sleepOpen = false;
  export let chapterOpen = false;
  export let bookmarkOpen = false;
  export let devicePickerOpen = false;

  export function closeAll() {
    speedOpen = false;
    sleepOpen = false;
    chapterOpen = false;
    bookmarkOpen = false;
    devicePickerOpen = false;
  }

  function onVolume(e: Event) {
    setABVolume(parseFloat((e.target as HTMLInputElement).value));
  }

  async function transferToDevice(targetId: string) {
    devicePickerOpen = false;
    const { transferAudiobookPlayback } = await import(
      "$lib/stores/player/audiobookPlayer"
    );
    await transferAudiobookPlayback(targetId);
  }
</script>

<div
  class="ab-right"
  role="presentation"
  tabindex="-1"
  on:click|stopPropagation
  on:keydown|stopPropagation
>
  <!-- Speed picker -->
  <div class="picker-wrap">
    <button
      class="ctrl-btn speed-btn"
      class:active={$abSpeed !== 1.0}
      on:click|stopPropagation={() => {
        speedOpen = !speedOpen;
        sleepOpen = false;
        chapterOpen = false;
        bookmarkOpen = false;
      }}
      title="Playback speed"
      aria-label="Playback speed"
    >
      {$abSpeed === 1.0 ? "1×" : `${$abSpeed}×`}
    </button>
    {#if speedOpen}
      <div class="picker-popup speed-popup">
        <div class="picker-header">Speed</div>
        {#each AB_SPEEDS as s (s)}
          <button
            class="picker-item"
            class:is-active={$abSpeed === s}
            on:click={() => {
              setABSpeed(s);
              speedOpen = false;
            }}>{s}×</button
          >
        {/each}
      </div>
    {/if}
  </div>

  <!-- Sleep timer -->
  {#if $audiobookSleepTimerEnabled}
    <div class="picker-wrap">
      <button
        class="ctrl-btn icon-btn"
        class:active={$sleepTimerMins > 0}
        on:click|stopPropagation={() => {
          sleepOpen = !sleepOpen;
          speedOpen = false;
          chapterOpen = false;
          bookmarkOpen = false;
        }}
        title={$sleepTimerMins > 0
          ? `Sleep in ${$sleepTimerMins} min`
          : "Sleep timer"}
        aria-label="Sleep timer"
      >
        <svg
          width="15"
          height="15"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
        </svg>
        {#if $sleepTimerMins > 0}
          <span class="sleep-badge">{$sleepTimerMins}</span>
        {/if}
      </button>
      {#if sleepOpen}
        <div class="picker-popup">
          <div class="picker-header">Sleep timer</div>
          <button
            class="picker-item"
            class:is-active={$sleepTimerMins === 0}
            on:click={() => {
              setSleepTimer(0);
              sleepOpen = false;
            }}>Off</button
          >
          {#each SLEEP_PRESETS as mins (mins)}
            <button
              class="picker-item"
              class:is-active={$sleepTimerMins === mins}
              on:click={() => {
                setSleepTimer(mins);
                sleepOpen = false;
              }}>{mins} min</button
            >
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Listen Along button -->
  {#if $lpRole === "host"}
    <button
      class="ctrl-btn icon-btn party-btn"
      class:active={$lpPanelOpen}
      on:click={() => lpPanelOpen.update((v) => !v)}
      title="Listen Along"
      aria-label="Listen Along panel"
    >
      <svg
        width="15"
        height="15"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <circle cx="9" cy="7" r="3" /><path
          d="M3 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2"
        />
        <circle cx="18" cy="7" r="2.5" /><path
          d="M22 21v-1.5a3.5 3.5 0 0 0-3.5-3.5H17"
        />
      </svg>
      {#if $lpParticipants.length > 0}
        <span class="party-count">{$lpParticipants.length}</span>
      {/if}
    </button>
  {:else if $lpRole === null}
    <button
      class="ctrl-btn icon-btn party-btn"
      on:click={createAndConnect}
      title="Start Listen Along"
      aria-label="Start Listen Along"
    >
      <svg
        width="15"
        height="15"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <circle cx="9" cy="7" r="3" /><path
          d="M3 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2"
        />
        <circle cx="18" cy="7" r="2.5" /><path
          d="M22 21v-1.5a3.5 3.5 0 0 0-3.5-3.5H17"
        />
      </svg>
    </button>
  {/if}

  <!-- Sessions button — only when exclusive mode is on and multiple sessions exist -->
  {#if $exclusiveMode && $activeDevices.length > 1}
    <div class="picker-wrap">
      <button
        class="ctrl-btn icon-btn device-btn"
        class:active={devicePickerOpen}
        on:click|stopPropagation={() => {
          devicePickerOpen = !devicePickerOpen;
          speedOpen = false;
          sleepOpen = false;
          chapterOpen = false;
          bookmarkOpen = false;
        }}
        title="Sessions"
        aria-label="Sessions"
      >
        <svg
          width="15"
          height="15"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <rect x="2" y="3" width="20" height="14" rx="2" />
          <path d="M8 21h8" />
          <path d="M12 17v4" />
        </svg>
        <span class="device-count">{$activeDevices.length}</span>
      </button>
      {#if devicePickerOpen}
        <button
          type="button"
          class="device-picker-overlay"
          aria-label="Close session picker"
          tabindex="-1"
          on:click={() => (devicePickerOpen = false)}
        ></button>
        <div class="picker-popup device-picker-popup">
          <div class="picker-header">Sessions</div>
          {#each $activeDevices as device (device.id)}
            <button
              class="picker-item device-item"
              class:is-active={device.is_active}
              on:click={() => transferToDevice(device.id)}
            >
              <span
                class="device-dot"
                class:dot-active={device.is_active}
                class:dot-this={device.id === deviceId}
              ></span>
              <span class="device-item-name">
                {device.name}
                {#if device.id === deviceId}<span class="this-badge">this</span
                  >{/if}
              </span>
              <span class="device-item-sub">
                {device.state.audiobook_title ||
                  device.state.track_title ||
                  "Idle"}
              </span>
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

  <input
    type="range"
    min="0"
    max="1"
    step="0.01"
    value={$abVolume}
    on:input={onVolume}
    class="volume-bar"
    aria-label="Volume"
  />

  <!-- Chapter list -->
  {#if $currentAudiobook?.chapters?.length}
    <div class="picker-wrap">
      <button
        class="ctrl-btn icon-btn"
        class:active={chapterOpen}
        on:click|stopPropagation={() => {
          chapterOpen = !chapterOpen;
          speedOpen = false;
          sleepOpen = false;
          bookmarkOpen = false;
        }}
        title="Chapters"
        aria-label="Chapters"
      >
        <svg
          width="15"
          height="15"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <line x1="8" y1="6" x2="21" y2="6" /><line
            x1="8"
            y1="12"
            x2="21"
            y2="12"
          />
          <line x1="8" y1="18" x2="21" y2="18" /><line
            x1="3"
            y1="6"
            x2="3.01"
            y2="6"
          />
          <line x1="3" y1="12" x2="3.01" y2="12" /><line
            x1="3"
            y1="18"
            x2="3.01"
            y2="18"
          />
        </svg>
      </button>
      {#if chapterOpen}
        <div class="picker-popup chapter-popup">
          <div class="picker-header">Chapters</div>
          <div class="chapter-list">
            {#each $currentAudiobook.chapters as ch (ch.id)}
              {@const isActive = $abCurrentChapter?.id === ch.id}
              <button
                class="chapter-item"
                class:is-active={isActive}
                on:click={() => {
                  jumpToChapter(ch);
                  chapterOpen = false;
                }}
              >
                <span class="chapter-num">{ch.chapter_num + 1}</span>
                <span class="chapter-title">{ch.title}</span>
                {#if isActive}
                  <span class="chapter-dot"></span>
                {/if}
              </button>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}

  <!-- Bookmarks -->
  <div class="picker-wrap">
    <button
      class="ctrl-btn icon-btn"
      class:active={$abBookmarks.length > 0}
      on:click|stopPropagation={() => {
        bookmarkOpen = !bookmarkOpen;
        speedOpen = false;
        sleepOpen = false;
        chapterOpen = false;
      }}
      title="Bookmarks"
      aria-label="Bookmarks"
    >
      <svg
        width="15"
        height="15"
        viewBox="0 0 24 24"
        fill={$abBookmarks.length > 0 ? "currentColor" : "none"}
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z" />
      </svg>
    </button>
    {#if bookmarkOpen}
      <div class="picker-popup bookmark-popup">
        <div class="picker-header">Bookmarks</div>
        <button
          class="picker-item add-bm"
          on:click={() => {
            createBookmark();
            bookmarkOpen = false;
          }}
        >
          <svg
            width="12"
            height="12"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            ><line x1="12" y1="5" x2="12" y2="19" /><line
              x1="5"
              y1="12"
              x2="19"
              y2="12"
            /></svg
          >
          Bookmark here
        </button>
        {#if $abBookmarks.length === 0}
          <p class="picker-empty">No bookmarks yet</p>
        {:else}
          {#each $abBookmarks as bm (bm.id)}
            <div class="bm-item">
              <button
                class="bm-jump"
                on:click={() => {
                  seekAudiobook(bm.position_ms / 1000);
                  bookmarkOpen = false;
                }}
              >
                <span class="bm-time">{formatBmTime(bm.position_ms)}</span>
                {#if bm.note}<span class="bm-note">{bm.note}</span>{/if}
              </button>
              <button
                class="bm-del"
                on:click={() => deleteBookmark(bm.id)}
                aria-label="Delete bookmark"
              >
                <svg
                  width="11"
                  height="11"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.5"
                  ><line x1="18" y1="6" x2="6" y2="18" /><line
                    x1="6"
                    y1="6"
                    x2="18"
                    y2="18"
                  /></svg
                >
              </button>
            </div>
          {/each}
        {/if}
      </div>
    {/if}
  </div>
</div>

<style>
  .ab-right {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .ctrl-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 4px 6px;
    height: 30px;
    transition: color 0.15s;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 4px;
  }
  .ctrl-btn:hover {
    color: var(--text);
  }

  .icon-btn {
    font-size: 0;
    padding: 6px;
    position: relative;
  }
  .icon-btn.active {
    color: var(--accent);
  }
  .icon-btn.active::after {
    content: "";
    position: absolute;
    bottom: 2px;
    left: 50%;
    transform: translateX(-50%);
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: var(--accent);
  }

  .speed-btn {
    font-size: 0.78rem;
    font-weight: 600;
    letter-spacing: -0.02em;
    color: var(--text-muted);
    padding: 4px 6px;
    border-radius: 4px;
    height: 26px;
    min-width: 30px;
  }
  .speed-btn.active {
    color: var(--accent);
  }

  .sleep-badge {
    position: absolute;
    top: 1px;
    right: 0;
    font-size: 8px;
    font-weight: 700;
    line-height: 1;
    color: var(--accent);
    pointer-events: none;
  }

  /* Listen Along button */
  .party-btn {
    position: relative;
    padding: 6px;
  }
  .party-btn svg {
    overflow: hidden;
    display: block;
  }
  .party-count {
    position: absolute;
    top: 1px;
    right: 0;
    font-size: 9px;
    font-weight: 700;
    line-height: 1;
    color: var(--accent);
    pointer-events: none;
  }

  .volume-bar {
    width: 76px;
    height: 4px;
    accent-color: var(--accent);
    cursor: pointer;
  }

  /* ── Pickers (shared) ─────────────────────────────────── */
  .picker-wrap {
    position: relative;
  }

  .picker-popup {
    position: absolute;
    bottom: calc(100% + 10px);
    right: 0;
    min-width: 140px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.35);
    z-index: 1000;
    overflow: hidden;
  }
  .picker-header {
    font-size: 0.68rem;
    font-weight: 600;
    letter-spacing: 0.07em;
    text-transform: uppercase;
    color: var(--text-muted);
    padding: 8px 12px 4px;
  }
  .picker-item {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 7px 12px;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    font-size: 0.83rem;
    color: var(--text-muted);
    transition:
      background 0.1s,
      color 0.1s;
  }
  .picker-item:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .picker-item.is-active {
    color: var(--accent);
    font-weight: 600;
  }
  .picker-empty {
    font-size: 0.78rem;
    color: var(--text-muted);
    padding: 6px 12px 10px;
    margin: 0;
  }

  /* Speed popup: center-aligned numbers */
  .speed-popup {
    min-width: 90px;
  }
  .speed-popup .picker-item {
    justify-content: center;
  }

  /* Chapter popup: wider, scrollable */
  .chapter-popup {
    min-width: 220px;
    right: 0;
  }
  .chapter-list {
    max-height: 260px;
    overflow-y: auto;
  }
  .chapter-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 7px 12px;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
  }
  .chapter-item:hover {
    background: var(--bg-hover);
  }
  .chapter-item.is-active {
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .chapter-num {
    font-size: 0.7rem;
    color: var(--text-muted);
    width: 18px;
    flex-shrink: 0;
    text-align: right;
  }
  .chapter-title {
    font-size: 0.82rem;
    color: var(--text);
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .chapter-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
    flex-shrink: 0;
  }

  /* Bookmark popup */
  .bookmark-popup {
    min-width: 200px;
  }
  .add-bm {
    color: var(--accent);
    font-weight: 500;
    border-bottom: 1px solid var(--border);
  }
  .bm-item {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px 2px 0;
  }
  .bm-jump {
    flex: 1;
    display: flex;
    gap: 8px;
    padding: 6px 12px;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
  }
  .bm-jump:hover {
    background: var(--bg-hover);
  }
  .bm-time {
    font-size: 0.78rem;
    color: var(--accent);
    font-variant-numeric: tabular-nums;
  }
  .bm-note {
    font-size: 0.78rem;
    color: var(--text-muted);
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .bm-del {
    flex-shrink: 0;
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 4px 8px;
    opacity: 0;
    transition:
      opacity 0.15s,
      color 0.15s;
    display: inline-flex;
    align-items: center;
  }
  .bm-item:hover .bm-del {
    opacity: 1;
  }
  .bm-del:hover {
    color: var(--error, #e55);
  }

  /* ── Device picker ───────────────────────────────────── */
  .device-btn {
    position: relative;
  }
  .device-count {
    position: absolute;
    top: 1px;
    right: 0;
    font-size: 8px;
    font-weight: 700;
    line-height: 1;
    color: var(--accent);
    pointer-events: none;
  }
  .device-picker-overlay {
    position: fixed;
    inset: 0;
    z-index: 999;
    background: transparent;
    border: none;
    padding: 0;
    margin: 0;
    cursor: default;
  }
  .device-picker-popup {
    min-width: 210px;
    right: 0;
  }
  .device-item {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: nowrap;
  }
  .device-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex-shrink: 0;
    background: var(--text-muted);
    opacity: 0.3;
  }
  .device-dot.dot-active {
    background: var(--accent);
    opacity: 1;
  }
  .device-dot.dot-this {
    opacity: 0.6;
  }
  .device-item-name {
    flex: 1;
    font-size: 0.83rem;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .device-item-sub {
    font-size: 0.72rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 80px;
  }
  .this-badge {
    font-size: 0.65rem;
    color: var(--text-muted);
    border: 1px solid var(--border);
    border-radius: 3px;
    padding: 0 3px;
    line-height: 1.4;
  }
  .device-transfer-hint {
    font-size: 0.72rem;
    color: var(--accent);
    flex-shrink: 0;
  }
</style>
