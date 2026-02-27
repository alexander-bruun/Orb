<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { lyricsOpen, lyricsLines, lyricsLoading, activeLyricIndex } from '$lib/stores/lyrics';
  import { currentTrack, positionMs } from '$lib/stores/player';

  // Drag state
  let posX = 0;
  let posY = 0;
  let dragOffsetX = 0;
  let dragOffsetY = 0;
  let dragging = false;

  // DOM refs
  let container: HTMLElement;
  let listEl: HTMLElement;

  // ── Smooth scroll via rAF lerp (foo_openlyrics approach) ──
  // Instead of calling scrollIntoView (which the browser manages choppily
  // when called repeatedly), we run our own animation loop that lerps
  // the container's scrollTop toward a target offset every frame.
  let targetScrollTop = 0;
  let currentScrollTop = 0;
  let rafId: number | null = null;
  let userScrolling = false;
  let userScrollTimer: ReturnType<typeof setTimeout> | null = null;

  const LERP_SPEED = 0.08; // lower = smoother but slower to catch up

  function scrollTick() {
    if (!listEl || !$lyricsOpen) {
      rafId = null;
      return;
    }

    if (!userScrolling) {
      // Lerp toward target
      const diff = targetScrollTop - currentScrollTop;
      if (Math.abs(diff) > 0.5) {
        currentScrollTop += diff * LERP_SPEED;
        listEl.scrollTop = Math.round(currentScrollTop);
      } else {
        currentScrollTop = targetScrollTop;
        listEl.scrollTop = targetScrollTop;
      }
    } else {
      // While user is manually scrolling, track their position
      currentScrollTop = listEl.scrollTop;
    }

    rafId = requestAnimationFrame(scrollTick);
  }

  function startScrollLoop() {
    if (rafId === null) {
      currentScrollTop = listEl?.scrollTop ?? 0;
      rafId = requestAnimationFrame(scrollTick);
    }
  }

  function stopScrollLoop() {
    if (rafId !== null) {
      cancelAnimationFrame(rafId);
      rafId = null;
    }
  }

  // Detect user manually scrolling (pause auto-scroll briefly)
  function onWheel() {
    userScrolling = true;
    if (userScrollTimer) clearTimeout(userScrollTimer);
    userScrollTimer = setTimeout(() => {
      userScrolling = false;
      // Re-sync currentScrollTop so lerp resumes from here
      currentScrollTop = listEl?.scrollTop ?? 0;
    }, 3000);
  }

  // Compute target scroll: center the active line in the container
  function updateTargetScroll(idx: number) {
    if (!listEl || idx < 0) return;
    const el = listEl.querySelector<HTMLElement>(`[data-idx="${idx}"]`);
    if (!el) return;

    const containerH = listEl.clientHeight;
    // Target: put the active line's vertical center at the container's center
    const elTop = el.offsetTop;
    const elH = el.offsetHeight;
    targetScrollTop = Math.max(0, elTop - containerH / 2 + elH / 2);
  }

  // Re-target whenever the active line changes
  let prevIdx = -1;
  $: if ($activeLyricIndex !== prevIdx && $lyricsOpen && listEl) {
    prevIdx = $activeLyricIndex;
    userScrolling = false; // snap back to auto on line change
    if (userScrollTimer) clearTimeout(userScrollTimer);
    updateTargetScroll($activeLyricIndex);
  }

  // Start/stop loop when modal opens/closes
  $: if ($lyricsOpen && listEl) {
    currentScrollTop = listEl.scrollTop;
    updateTargetScroll($activeLyricIndex);
    startScrollLoop();
  } else {
    stopScrollLoop();
  }

  // Reset on new track
  let prevTrackId: string | null = null;
  $: if ($currentTrack?.id !== prevTrackId) {
    prevTrackId = $currentTrack?.id ?? null;
    prevIdx = -1;
    targetScrollTop = 0;
    currentScrollTop = 0;
    if (listEl) listEl.scrollTop = 0;
  }

  // ── Next-line fade (foo_openlyrics highlight interpolation) ──
  let nextLineFade = 0;
  $: {
    const lines = $lyricsLines;
    const pos = $positionMs;
    const idx = $activeLyricIndex;
    if (idx >= 0 && idx < lines.length - 1) {
      const nextTime = lines[idx + 1].time_ms;
      const fadeStart = Math.max(lines[idx].time_ms, nextTime - 600);
      if (pos >= fadeStart) {
        nextLineFade = Math.min(1, (pos - fadeStart) / (nextTime - fadeStart));
      } else {
        nextLineFade = 0;
      }
    } else {
      nextLineFade = 0;
    }
  }

  // ── Drag ──
  onMount(() => {
    posX = window.innerWidth - 360;
    posY = Math.max(60, window.innerHeight - 560);
  });

  onDestroy(() => {
    stopScrollLoop();
    if (userScrollTimer) clearTimeout(userScrollTimer);
  });

  function startDrag(e: MouseEvent) {
    if ((e.target as HTMLElement).closest('button')) return;
    dragging = true;
    dragOffsetX = e.clientX - posX;
    dragOffsetY = e.clientY - posY;
    window.addEventListener('mousemove', onDragMove);
    window.addEventListener('mouseup', stopDrag);
    e.preventDefault();
  }

  function onDragMove(e: MouseEvent) {
    if (!dragging) return;
    posX = Math.max(0, Math.min(window.innerWidth - 320, e.clientX - dragOffsetX));
    posY = Math.max(0, Math.min(window.innerHeight - 80, e.clientY - dragOffsetY));
  }

  function stopDrag() {
    dragging = false;
    window.removeEventListener('mousemove', onDragMove);
    window.removeEventListener('mouseup', stopDrag);
  }
</script>

{#if $lyricsOpen}
  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div
    class="lyrics-modal"
    class:dragging
    bind:this={container}
    style="left: {posX}px; top: {posY}px;"
  >
    <!-- svelte-ignore a11y-no-static-element-interactions -->
    <div class="modal-header" on:mousedown={startDrag}>
      <span class="modal-title">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M9 18V5l12-2v13"/>
          <circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>
        </svg>
        Lyrics
      </span>
      {#if $currentTrack}
        <span class="track-name">{$currentTrack.title}</span>
      {/if}
      <button class="close-btn" on:click={() => lyricsOpen.set(false)} aria-label="Close lyrics">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
          <line x1="18" y1="6" x2="6" y2="18"/>
          <line x1="6" y1="6" x2="18" y2="18"/>
        </svg>
      </button>
    </div>

    <!-- svelte-ignore a11y-no-static-element-interactions -->
    <div class="modal-body" bind:this={listEl} on:wheel={onWheel}>
      {#if $lyricsLoading}
        <div class="state-msg">Loading…</div>
      {:else if $lyricsLines.length === 0}
        <div class="state-msg">No lyrics available</div>
      {:else}
        {#each $lyricsLines as line, i (line.time_ms + '-' + i)}
          <div
            class="lyric-line"
            class:active={i === $activeLyricIndex}
            class:past={i < $activeLyricIndex}
            class:next={i === $activeLyricIndex + 1}
            style={i === $activeLyricIndex + 1 ? `opacity: ${0.55 + 0.45 * nextLineFade}` : ''}
            data-idx={i}
          >
            {line.text}
          </div>
        {/each}
      {/if}
    </div>
  </div>
{/if}

<style>
  .lyrics-modal {
    position: fixed;
    z-index: 600;
    width: 320px;
    max-height: 440px;
    display: flex;
    flex-direction: column;
    background: var(--bg-elevated);
    border: 1px solid var(--border-2);
    border-radius: 10px;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.45), 0 2px 8px rgba(0, 0, 0, 0.2);
    overflow: hidden;
    animation: popIn 0.18s cubic-bezier(0.34, 1.56, 0.64, 1);
    user-select: none;
  }

  .lyrics-modal.dragging {
    box-shadow: 0 16px 48px rgba(0, 0, 0, 0.55);
    cursor: grabbing;
  }

  @keyframes popIn {
    from { opacity: 0; transform: scale(0.94); }
    to   { opacity: 1; transform: scale(1); }
  }

  .modal-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    border-bottom: 1px solid var(--border);
    cursor: grab;
    flex-shrink: 0;
  }
  .lyrics-modal.dragging .modal-header {
    cursor: grabbing;
  }

  .modal-title {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .track-name {
    flex: 1;
    font-size: 0.78rem;
    color: var(--text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .close-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    border-radius: 5px;
    flex-shrink: 0;
    transition: background 0.12s, color 0.12s;
  }
  .close-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .modal-body {
    flex: 1;
    overflow-y: auto;
    padding: 16px 12px 20px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .modal-body::-webkit-scrollbar { width: 4px; }
  .modal-body::-webkit-scrollbar-track { background: transparent; }
  .modal-body::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }

  .state-msg {
    text-align: center;
    padding: 32px 0;
    font-size: 0.82rem;
    color: var(--text-muted);
  }

  .lyric-line {
    padding: 6px 10px;
    font-size: 0.9rem;
    line-height: 1.5;
    color: var(--text-muted);
    border-radius: 6px;
    transition: color 0.3s ease, background 0.3s ease, opacity 0.3s ease, transform 0.3s ease;
    cursor: default;
    opacity: 0.55;
  }

  .lyric-line.past {
    color: var(--text-2);
    opacity: 0.35;
  }

  .lyric-line.active {
    font-size: 1rem;
    font-weight: 600;
    background: var(--accent-dim);
    color: var(--accent);
    opacity: 1;
    transform: scale(1.01);
  }

  .lyric-line.next {
    color: var(--text);
  }
</style>
