<script lang="ts">
  import { userQueue, removeFromUserQueue, playTrack, queueModalOpen } from '$lib/stores/player';

  let maximized = true;

  // Auto-close when queue drops to 1 or fewer items
  $: if ($userQueue.length <= 1) {
    queueModalOpen.set(false);
  }

  function close() {
    queueModalOpen.set(false);
  }

  function toggleSize() {
    maximized = !maximized;
  }

  function playFromQueue(index: number) {
    const track = $userQueue[index];
    // Remove all items up to and including this one
    removeFromUserQueue(index);
    playTrack(track);
  }
</script>

{#if $queueModalOpen && $userQueue.length > 1}
  <div class="queue-panel" class:maximized>
    <div class="panel-head">
      <span class="panel-title">
        Up Next
        <span class="count">{$userQueue.length}</span>
      </span>
      <div class="head-actions">
        <button class="head-btn" on:click={toggleSize} title={maximized ? 'Collapse' : 'Expand'}>
          {#if maximized}
            <!-- Chevron down = collapse -->
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
              <polyline points="6,9 12,15 18,9"/>
            </svg>
          {:else}
            <!-- Chevron up = expand -->
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
              <polyline points="18,15 12,9 6,15"/>
            </svg>
          {/if}
        </button>
        <button class="head-btn" on:click={close} title="Close">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
            <line x1="18" y1="6" x2="6" y2="18"/>
            <line x1="6" y1="6" x2="18" y2="18"/>
          </svg>
        </button>
      </div>
    </div>

    {#if maximized}
      <div class="queue-list">
        {#each $userQueue as track, i (track.id + '-' + i)}
          <div class="qi">
            <span class="qi-num">{i + 1}</span>
            <div class="qi-info">
              <span class="qi-title">{track.title}</span>
              {#if track.artist_name}
                <span class="qi-artist">{track.artist_name}</span>
              {/if}
            </div>
            <button
              class="qi-act"
              on:click={() => playFromQueue(i)}
              title="Play now"
            >
              <svg width="11" height="11" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <polygon points="5,3 19,12 5,21"/>
              </svg>
            </button>
            <button
              class="qi-act"
              on:click={() => removeFromUserQueue(i)}
              title="Remove"
            >
              <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
                <line x1="18" y1="6" x2="6" y2="18"/>
                <line x1="6" y1="6" x2="18" y2="18"/>
              </svg>
            </button>
          </div>
        {/each}
      </div>
    {/if}
  </div>
{/if}

<style>
  .queue-panel {
    position: fixed;
    bottom: var(--bottom-h);
    right: 20px;
    width: 300px;
    background: var(--surface);
    border: 1px solid var(--border-2);
    border-radius: 10px 10px 0 0;
    box-shadow: 0 -4px 24px rgba(0,0,0,0.4), 0 -1px 4px rgba(0,0,0,0.2);
    z-index: 500;
    overflow: hidden;
    animation: slideUp 0.18s ease-out;
  }

  @keyframes slideUp {
    from { transform: translateY(8px); opacity: 0; }
    to   { transform: translateY(0); opacity: 1; }
  }

  .panel-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 12px;
    border-bottom: 1px solid var(--border);
    user-select: none;
  }

  .panel-title {
    font-size: 0.78rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--text-2);
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .count {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: var(--accent-dim);
    color: var(--accent);
    border-radius: 10px;
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    font-size: 0.72rem;
    font-weight: 700;
  }

  .head-actions {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .head-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    background: none;
    border: none;
    color: var(--text-2);
    cursor: pointer;
    border-radius: 5px;
    transition: background 0.1s, color 0.1s;
  }
  .head-btn:hover { background: var(--surface-2); color: var(--text); }

  .queue-list {
    max-height: 340px;
    overflow-y: auto;
    padding: 4px;
  }
  .queue-list::-webkit-scrollbar { width: 4px; }
  .queue-list::-webkit-scrollbar-track { background: transparent; }
  .queue-list::-webkit-scrollbar-thumb { background: var(--border-2); border-radius: 2px; }

  .qi {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 8px;
    border-radius: 6px;
    transition: background 0.1s;
  }
  .qi:hover { background: var(--surface-2); }
  .qi:hover .qi-act { opacity: 1; }

  .qi-num {
    width: 18px;
    text-align: center;
    font-size: 0.72rem;
    color: var(--muted);
    flex-shrink: 0;
  }

  .qi-info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .qi-title {
    font-size: 0.82rem;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .qi-artist {
    font-size: 0.72rem;
    color: var(--text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .qi-act {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    flex-shrink: 0;
    background: none;
    border: none;
    color: var(--text-2);
    cursor: pointer;
    border-radius: 4px;
    opacity: 0;
    transition: opacity 0.1s, background 0.1s, color 0.1s;
  }
  .qi-act:hover { background: var(--border-2); color: var(--text); opacity: 1; }
</style>
