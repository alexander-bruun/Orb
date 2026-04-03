<script lang="ts">
  import {
    abSpeed,
    abBookmarks,
    sleepTimerMins,
    currentAudiobook,
    abCurrentChapter,
    setABSpeed,
    setSleepTimer,
    jumpToChapter,
    seekAudiobook,
    createBookmark,
    deleteBookmark,
    AB_SPEEDS,
    SLEEP_PRESETS,
  } from "$lib/stores/player/audiobookPlayer";
  import type { AudiobookChapter } from "$lib/types";

  export let showSpeed = false;
  export let showSleep = false;
  export let showChapters = false;
  export let showBookmarks = false;
  export let closeSheets: () => void;

  function fmtMs(ms: number): string {
    const s = Math.floor(ms / 1000);
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = s % 60;
    if (h > 0)
      return `${h}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
    return `${m}:${String(sec).padStart(2, "0")}`;
  }
</script>

{#if showSpeed}
  <button
    type="button"
    class="sheet-overlay"
    tabindex="-1"
    aria-label="Close sheets"
    on:click={closeSheets}
    on:touchstart|stopPropagation
    on:touchmove|stopPropagation
    on:touchend|stopPropagation
  ></button>

  <div
    class="bottom-sheet"
    role="dialog"
    tabindex="-1"
    on:click|stopPropagation
    on:touchstart|stopPropagation
    on:touchmove|stopPropagation
    on:touchend|stopPropagation
    on:keydown|stopPropagation
  >
    <div class="sheet-handle"></div>
    <p class="sheet-title">Playback Speed</p>
    <div class="speed-grid">
      {#each AB_SPEEDS as s}
        <button
          class="speed-chip"
          class:chip-active={$abSpeed === s}
          on:click={() => {
            setABSpeed(s);
            showSpeed = false;
          }}>{s}×</button
        >
      {/each}
    </div>
  </div>
{/if}

{#if showSleep}
  <button
    type="button"
    class="sheet-overlay"
    tabindex="-1"
    aria-label="Close sheets"
    on:click={closeSheets}
    on:touchstart|stopPropagation
    on:touchmove|stopPropagation
    on:touchend|stopPropagation
  ></button>

  <div
    class="bottom-sheet"
    role="dialog"
    tabindex="-1"
    on:click|stopPropagation
    on:touchstart|stopPropagation
    on:touchmove|stopPropagation
    on:touchend|stopPropagation
    on:keydown|stopPropagation
  >
    <div class="sheet-handle"></div>
    <p class="sheet-title">Sleep Timer</p>
    <div class="speed-grid">
      <button
        class="speed-chip"
        class:chip-active={$sleepTimerMins === 0}
        on:click={() => {
          setSleepTimer(0);
          showSleep = false;
        }}>Off</button
      >
      {#each SLEEP_PRESETS as mins}
        <button
          class="speed-chip"
          class:chip-active={$sleepTimerMins === mins}
          on:click={() => {
            setSleepTimer(mins);
            showSleep = false;
          }}>{mins}m</button
        >
      {/each}
    </div>
  </div>
{/if}

{#if showChapters && $currentAudiobook?.chapters?.length}
  <button
    type="button"
    class="sheet-overlay"
    tabindex="-1"
    aria-label="Close sheets"
    on:click={closeSheets}
    on:touchstart|stopPropagation
    on:touchmove|stopPropagation
    on:touchend|stopPropagation
  ></button>

  <div
    class="bottom-sheet chapter-sheet"
    role="dialog"
    tabindex="-1"
    on:click|stopPropagation
    on:touchstart|stopPropagation
    on:touchmove|stopPropagation
    on:touchend|stopPropagation
    on:keydown|stopPropagation
  >
    <div class="sheet-handle"></div>
    <p class="sheet-title">Chapters</p>
    <div class="chapter-scroll">
      {#each $currentAudiobook.chapters as ch (ch.id)}
        {@const active = $abCurrentChapter?.id === ch.id}
        <button
          class="chapter-row"
          class:ch-active={active}
          on:click={() => {
            jumpToChapter(ch);
            showChapters = false;
          }}
        >
          <span class="ch-num">{ch.chapter_num + 1}</span>
          <span class="ch-name">{ch.title}</span>
          <span class="ch-time">{fmtMs(ch.start_ms)}</span>
          {#if active}<span class="ch-dot"></span>{/if}
        </button>
      {/each}
    </div>
  </div>
{/if}

{#if showBookmarks}
  <button
    type="button"
    class="sheet-overlay"
    tabindex="-1"
    aria-label="Close sheets"
    on:click={closeSheets}
    on:touchstart|stopPropagation
    on:touchmove|stopPropagation
    on:touchend|stopPropagation
  ></button>

  <div
    class="bottom-sheet"
    role="dialog"
    tabindex="-1"
    on:click|stopPropagation
    on:touchstart|stopPropagation
    on:touchmove|stopPropagation
    on:touchend|stopPropagation
    on:keydown|stopPropagation
  >
    <div class="sheet-handle"></div>
    <p class="sheet-title">Bookmarks</p>
    <button
      class="bm-add-btn"
      on:click={() => {
        createBookmark();
        showBookmarks = false;
      }}
    >
      <svg
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2.5"
      >
        <line x1="12" y1="5" x2="12" y2="19" /><line
          x1="5"
          y1="12"
          x2="19"
          y2="12"
        />
      </svg>
      Bookmark current position
    </button>
    {#if $abBookmarks.length === 0}
      <p class="bm-empty">No bookmarks yet</p>
    {:else}
      <div class="bm-list">
        {#each $abBookmarks as bm (bm.id)}
          <div class="bm-row">
            <button
              class="bm-jump"
              on:click={() => {
                seekAudiobook(bm.position_ms / 1000);
                showBookmarks = false;
              }}
              aria-label="Jump to bookmark at {fmtMs(bm.position_ms)}"
            >
              <span class="bm-t">{fmtMs(bm.position_ms)}</span>
              {#if bm.note}<span class="bm-n">{bm.note}</span>{/if}
            </button>
            <button
              class="bm-del"
              on:click={() => deleteBookmark(bm.id)}
              aria-label="Delete"
            >
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2.5"
                stroke-linecap="round"
              >
                <line x1="18" y1="6" x2="6" y2="18" /><line
                  x1="6"
                  y1="6"
                  x2="18"
                  y2="18"
                />
              </svg>
            </button>
          </div>
        {/each}
      </div>
    {/if}
  </div>
{/if}

<style>
  @media (max-width: 640px) {
    .sheet-overlay {
      position: fixed;
      inset: 0;
      background: rgba(0, 0, 0, 0.45);
      z-index: 700;
      border: none;
      padding: 0;
      margin: 0;
      cursor: default;
      outline: none;
      display: block;
    }
    .bottom-sheet {
      position: fixed;
      bottom: 0;
      left: 0;
      right: 0;
      background: var(--bg-elevated);
      border-radius: 20px 20px 0 0;
      padding: 12px 24px calc(24px + env(safe-area-inset-bottom, 0px));
      z-index: 800;
      border-top: 1px solid var(--border);
      box-shadow: 0 -8px 32px rgba(0, 0, 0, 0.4);
    }
    .sheet-handle {
      width: 36px;
      height: 4px;
      background: var(--border);
      border-radius: 2px;
      margin: 0 auto 16px;
    }
    .sheet-title {
      font-size: 0.75rem;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--text-muted);
      margin: 0 0 18px;
      text-align: center;
    }

    .speed-grid {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      justify-content: center;
    }
    .speed-chip {
      padding: 12px 20px;
      background: var(--bg-hover);
      border: 1px solid var(--border);
      border-radius: 24px;
      font-size: 0.9rem;
      font-weight: 600;
      color: var(--text-muted);
      cursor: pointer;
    }
    .chip-active {
      background: var(--accent) !important;
      color: #fff !important;
      border-color: var(--accent) !important;
      font-weight: 700;
    }

    .chapter-sheet {
      max-height: 70vh;
      display: flex;
      flex-direction: column;
    }
    .chapter-scroll {
      overflow-y: auto;
      flex: 1;
    }
    .chapter-row {
      display: flex;
      align-items: center;
      gap: 12px;
      width: 100%;
      padding: 14px 8px;
      background: none;
      border: none;
      border-bottom: 1px solid var(--border);
      cursor: pointer;
      text-align: left;
    }
    .ch-active {
      background: rgba(var(--accent-rgb), 0.1) !important;
    }
    .ch-num {
      font-size: 0.75rem;
      color: var(--text-muted);
      width: 24px;
      text-align: right;
    }
    .ch-name {
      flex: 1;
      font-size: 0.9rem;
      color: var(--text);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .ch-time {
      font-size: 0.75rem;
      color: var(--text-muted);
      font-variant-numeric: tabular-nums;
    }
    .ch-dot {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: var(--accent);
    }

    .bm-add-btn {
      display: flex;
      align-items: center;
      gap: 10px;
      width: 100%;
      padding: 14px 8px;
      background: none;
      border: none;
      border-bottom: 1px solid var(--border);
      cursor: pointer;
      font-size: 0.9rem;
      font-weight: 600;
      color: var(--accent);
    }
    .bm-empty {
      font-size: 0.85rem;
      color: var(--text-muted);
      text-align: center;
      padding: 24px 0;
    }
    .bm-list {
      max-height: 300px;
      overflow-y: auto;
    }
    .bm-row {
      display: flex;
      align-items: center;
      border-bottom: 1px solid var(--border);
    }
    .bm-jump {
      flex: 1;
      display: flex;
      align-items: center;
      gap: 12px;
      padding: 14px 8px;
      background: none;
      border: none;
      cursor: pointer;
      text-align: left;
    }
    .bm-t {
      font-size: 0.85rem;
      color: var(--accent);
      font-weight: 600;
    }
    .bm-n {
      font-size: 0.85rem;
      color: var(--text-muted);
      flex: 1;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .bm-del {
      background: none;
      border: none;
      color: var(--text-muted);
      padding: 12px;
    }
  }
</style>
