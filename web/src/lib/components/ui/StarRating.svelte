<script lang="ts">
  import { ratings } from '$lib/stores/library/ratings';

  export let trackId: string;
  export let size: number = 16;

  let open = false;
  let hoverRating = 0;
  let closeTimer: ReturnType<typeof setTimeout>;

  $: currentRating = $ratings.get(trackId) ?? 0;

  async function pick(n: number) {
    open = false;
    hoverRating = 0;
    await ratings.toggle(trackId, n);
  }

  function scheduleClose() {
    closeTimer = setTimeout(() => {
      open = false;
      hoverRating = 0;
    }, 150);
  }

  function cancelClose() {
    clearTimeout(closeTimer);
  }
</script>


<div
  class="star-root"
  role="presentation"
  on:mouseleave={scheduleClose}
  on:mouseenter={cancelClose}
>
  <!-- Trigger: single star showing current rating -->
  <button
    class="star-trigger"
    class:rated={currentRating > 0}
    on:click|stopPropagation={() => { open = !open; hoverRating = currentRating; }}
    aria-label={currentRating > 0 ? `Rated ${currentRating} stars` : 'Rate this track'}
    title={currentRating > 0 ? `${currentRating}/5 stars` : 'Rate'}
  >
    <svg width={size} height={size} viewBox="0 0 24 24" fill={currentRating > 0 ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/>
    </svg>
    {#if currentRating > 0}
      <span class="rating-badge">{currentRating}</span>
    {/if}
  </button>

  <!-- Popover: 5-star picker -->
  {#if open}
    <div class="star-popover">
      {#each [1, 2, 3, 4, 5] as n}
        <button
          class="star-pick"
          class:lit={n <= (hoverRating || currentRating)}
          on:mouseenter={() => hoverRating = n}
          on:mouseleave={() => hoverRating = 0}
          on:click|stopPropagation={() => pick(n)}
          aria-label="{n} star{n > 1 ? 's' : ''}"
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill={n <= (hoverRating || currentRating) ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/>
          </svg>
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .star-root {
    position: relative;
    display: inline-flex;
    align-items: center;
  }

  .star-trigger {
    background: none;
    border: none;
    padding: 2px;
    cursor: pointer;
    color: var(--text-muted);
    display: inline-flex;
    align-items: center;
    gap: 2px;
    transition: color 0.15s;
    position: relative;
  }
  .star-trigger:hover { color: var(--text); }
  .star-trigger.rated { color: #f5c518; }

  .rating-badge {
    font-size: 9px;
    font-weight: 700;
    line-height: 1;
    color: #f5c518;
    pointer-events: none;
  }

  .star-popover {
    position: absolute;
    bottom: calc(100% + 6px);
    right: 0;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 6px 8px;
    display: flex;
    gap: 2px;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.3);
    z-index: 1001;
    animation: pop-in 0.12s cubic-bezier(0.22, 1, 0.36, 1);
    white-space: nowrap;
  }

  @keyframes pop-in {
    from { opacity: 0; transform: scale(0.85); transform-origin: bottom right; }
    to   { opacity: 1; transform: scale(1); transform-origin: bottom right; }
  }

  .star-pick {
    background: none;
    border: none;
    padding: 2px;
    cursor: pointer;
    color: var(--text-muted);
    display: flex;
    align-items: center;
    transition: color 0.1s, transform 0.1s;
  }
  .star-pick:hover { transform: scale(1.2); }
  .star-pick.lit { color: #f5c518; }
</style>
